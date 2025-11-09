package fairmutex

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type RWMutex struct {
	initialised        atomic.Bool
	config             *config
	histogram          metric.Float64Histogram
	exclusive          chan lockRequest
	shared             chan lockRequest
	releaseExclusive   chan chan struct{}
	releaseShared      chan chan struct{}
	stopProcessing     chan struct{}
	waitingOnExclusive atomic.Bool
	waitingOnShared    atomic.Bool
}

type lockRequest struct {
	c chan struct{}
	n int // The number of (shared) locks requested
}

// New - returns a pointer to a new Mutex with the processing process already
// running.
func New(options ...Option) *RWMutex {
	cfg := getConfig(options...)

	meter := otel.Meter("fair-mutex")

	histogram, err := meter.Float64Histogram(
		cfg.metricName,
		metric.WithDescription("Duration waiting for a mutex lock in seconds"),
		metric.WithUnit("s"),
		// Buckets: 1ns, 10ns, 100ns, 1µs, 10µs, 100µs, 1ms, 10ms, 100ms, 1s, 10s, 100s
		metric.WithExplicitBucketBoundaries(0.000000001, 0.00000001, 0.0000001, 0.000001, 0.00001, 0.0001, 0.001, 0.01, 0.1, 1, 10, 100),
	)

	if err != nil {
		panic(err)
	}

	m := &RWMutex{
		config:           cfg,
		histogram:        histogram,
		exclusive:        make(chan lockRequest, cfg.exclusiveMaxQueueSize),
		shared:           make(chan lockRequest, cfg.sharedMaxQueueSize),
		releaseExclusive: make(chan chan struct{}),
		releaseShared:    make(chan chan struct{}, cfg.sharedMaxBatchSize),
		stopProcessing:   make(chan struct{}),
	}

	go m.process()

	m.initialised.Store(true)

	return m
}

const (
	noLockTaken = iota
	sharedLockTaken
	exclusiveLockTaken
)

// Stop - causes the go func processing mutex requests to stop running and the
// cleanup method to be called after that. Calling this function is required so
// that resources are not leaked (e.g. zombie go processes).
func (m *RWMutex) Stop() {
	if m.initialised.Load() {
		m.stopProcessing <- struct{}{}
	}
}

// process - handles the batching and granting of the lock requests
func (m *RWMutex) process() {
	defer m.cleanup()

	var loopInitLock lockRequest
	var lockItems int
	lockTypeTaken := noLockTaken

	for {
		switch lockTypeTaken {

		// If the last batch processed was shared, give exclusive locks, if any are waiting
		case sharedLockTaken:
			select {

			case <-m.stopProcessing:
				return

			case loopInitLock = <-m.exclusive:
				m.waitingOnExclusive.Store(true)
				lockItems = min(len(m.exclusive), m.config.exclusiveMaxBatchSize)
				lockTypeTaken = exclusiveLockTaken

			default:
				lockTypeTaken = noLockTaken

			}

		// If the last batch processed was exclusive, give shared locks, if any are waiting
		case exclusiveLockTaken:
			select {

			case <-m.stopProcessing:
				return

			case loopInitLock = <-m.shared:
				m.waitingOnShared.Store(true)
				lockItems = min(len(m.shared), m.config.sharedMaxBatchSize)
				lockTypeTaken = sharedLockTaken

			default:
				lockTypeTaken = noLockTaken

			}

		}

		// If this is the initial loop or there were no locks waiting of the opposite type to the last batch, then
		// we'll randomly select which type of lock to grant (given one of each type is requested simultaneously).
		if lockTypeTaken == noLockTaken {
			select {

			case <-m.stopProcessing:
				return

			case loopInitLock = <-m.exclusive:
				m.waitingOnExclusive.Store(true)
				lockItems = min(len(m.exclusive), m.config.exclusiveMaxBatchSize)
				lockTypeTaken = exclusiveLockTaken

			case loopInitLock = <-m.shared:
				m.waitingOnShared.Store(true)
				lockItems = min(len(m.shared), m.config.sharedMaxBatchSize)
				lockTypeTaken = sharedLockTaken

			}
		}

		// Action the locks - grant and wait until they are released
		switch lockTypeTaken {

		case sharedLockTaken:
			// Process shared locks - grant the locks
			loopInitLock.c <- struct{}{} // Initial lock

			lockCnt := loopInitLock.n

			for range lockItems {
				// Grab the lock request
				l := <-m.shared

				// Signal to the requester that they now have the lock
				l.c <- struct{}{}

				lockCnt += l.n
			}

			// Wait for the shared locks to be returned
			for n := 1; n <= lockCnt; n++ {
				select {
				case <-m.stopProcessing:
					return
				case u := <-m.releaseShared:
					if n == lockCnt {
						m.waitingOnShared.Store(false)
					}

					// Signal back that the lock has been released
					u<-struct{}{}
				}
			}


		case exclusiveLockTaken:
			// Process exclusive locks - grant and wait for each lock to be returned

			// Initial loop lock
			// Signal to the requester that they now have the lock
			loopInitLock.c <- struct{}{}

			// Wait for the lock to be returned
			select {
			case <-m.stopProcessing:
				return
			case u := <-m.releaseExclusive:
					if lockItems == 0 {
						m.waitingOnExclusive.Store(false)
					}

					// Signal back that the lock has been released
					u<-struct{}{}

				// Remaining exclusive lock requests
				for n := 1; n <= lockItems; n++ {
					// Grab the lock request
					l := <-m.exclusive

					// Signal to the requester that they now have the lock
					l.c <- struct{}{}

					// Wait for the lock to be returned
					select {
					case <-m.stopProcessing:
						return
					case u := <-m.releaseExclusive:
						if n == lockItems {
							m.waitingOnExclusive.Store(false)
						}

						// Signal back that the lock has been released
						u<-struct{}{}
					}
				}

			}
		}
	}
}

// cleanup - ensures that we don't have any resource leakage; however, this is
// only run after the process method has finished, and that is triggered be
// calling the Stop method.
func (m *RWMutex) cleanup() {
	m.initialised.Store(false)

	close(m.exclusive)
	close(m.shared)
	close(m.releaseExclusive)
	close(m.releaseShared)
}

// -----------------------------------------------------------------------------
// sync.RWMutex methods
// -----------------------------------------------------------------------------

// RLock - locks the mutex for reading.
//
// It should not be used for recursive read locking; a blocked Lock call
// excludes new readers from acquiring the lock. See the documentation on the
// RWMutex type (https://pkg.go.dev/sync#RWMutex).
func (m *RWMutex) RLock() {
	if !m.initialised.Load() {
		panic("attempt to use fair-mutex uninitialised")
	}

	start := time.Now()

	l := make(chan struct{})
	defer close(l)

	// Request the lock
	m.shared <- lockRequest{c: l, n: 1}

	// Wait for the lock to be granted
	<-l

	m.histogram.Record(context.Background(), time.Since(start).Seconds(), metric.WithAttributes(
		append(m.config.metricAttributes, attribute.String("operation", "RLock"))...,
	))
}

// TryRLock - tries to lock rw for reading and reports whether it succeeded.
//
// Note that while correct uses of TryRLock do exist, they are rare, and use of
// TryRLock is often a sign of a deeper problem in a particular use of mutexes.
func (m *RWMutex) TryRLock() bool {
	if !m.initialised.Load() {
		panic("attempt to use fair-mutex uninitialised")
	}

	l := make(chan struct{})
	defer close(l)

	if m.waitingOnExclusive.Load() || m.waitingOnShared.Load() || len(m.exclusive) > 0 || len(m.shared) > 0 {
		return false
	}

	// Request the lock
	m.shared <- lockRequest{c: l, n: 1}

	// Wait for the lock to be granted
	<-l

	return true
}

// RUnlock - undoes a single fairmutex.RLock call; it does not affect other
// simultaneous readers. It is a run-time error if rw is not locked for reading
// on entry to RUnlock.
func (m *RWMutex) RUnlock() {
	if !m.initialised.Load() {
		panic("attempt to use fair-mutex uninitialised")
	}

	if !m.waitingOnShared.Load() {
		panic("fair-mutex: RUnlock of unlocked RWMutex")
	}

	u := make(chan struct{})

	m.releaseShared <- u

	// Wait for the lock to be released
	<-u

	close(u)
}

// Lock - locks the mutex for writing. If the mutex is already locked for
// reading or writing, Lock blocks until the lock is available.
func (m *RWMutex) Lock() {
	if !m.initialised.Load() {
		panic("attempt to use fair-mutex uninitialised")
	}

	start := time.Now()

	l := make(chan struct{})
	defer close(l)

	// Request the lock
	m.exclusive <- lockRequest{c: l, n: 1}

	// Wait for the lock to be granted
	<-l

	m.histogram.Record(context.Background(), time.Since(start).Seconds(), metric.WithAttributes(
		append(m.config.metricAttributes, attribute.String("operation", "Lock"))...,
	))
}

// TryLock - tries to lock rw for writing and reports whether it succeeded.
//
// Note that while correct uses of TryLock do exist, they are rare, and use of
// TryLock is often a sign of a deeper problem in a particular use of mutexes.
func (m *RWMutex) TryLock() bool {
	if !m.initialised.Load() {
		panic("attempt to use fair-mutex uninitialised")
	}

	l := make(chan struct{})
	defer close(l)

	// See if there are any lock requests or locks active
	if m.waitingOnExclusive.Load() || m.waitingOnShared.Load() || len(m.exclusive) > 0 || len(m.shared) > 0 {
		return false
	}

	// Request the lock
	m.exclusive <- lockRequest{c: l, n: 1}

	// Wait on the lock to be granted
	<-l

	return true
}

// Unlock - unlocks rw for writing. It is a run-time error if rw is not locked
// for writing on entry to Unlock.
//
// As with Mutexes, a locked FairMutex is not associated with a particular
// goroutine. One goroutine may fairmutex.RLock (fairmutex.Lock) a FairMutex and
// then arrange for another goroutine to fairmutex.RUnlock (fairmutex.Unlock)
// it.
func (m *RWMutex) Unlock() {
	if !m.initialised.Load() {
		panic("attempt to use fair-mutex uninitialised")
	}

	if !m.waitingOnExclusive.Load() {
		panic("fair-mutex: Unlock of unlocked RWMutex")
	}

	u := make(chan struct{})

	m.releaseExclusive <- u

	// Wait for the lock to be released
	<-u

	close(u)
}

// RLocker - returns a [Locker] interface that implements the [Locker.Lock] and
// [Locker.Unlock] methods by calling m.RLock and m.RUnlock.
func (m *RWMutex) RLocker() sync.Locker {
	return (*rlocker)(m)
}

type rlocker RWMutex

func (r *rlocker) Lock()   { (*RWMutex)(r).RLock() }
func (r *rlocker) Unlock() { (*RWMutex)(r).RUnlock() }

// Extension methods

// RLockSet - locks the mutex for reading, granting the requested number of 
// locks. RUnlock must be called one for each of the requested number of locks.
//
// Use RLockSet when a set of read locks is required to be granted at the same
// time (that is, within the same batch). This might be done when read locks
// were granted in a loop before processing was done, and the locks unlocked.
//
// It should not be used for recursive read locking; a blocked Lock call
// excludes new readers from acquiring the lock. See the documentation on the
// RWMutex type (https://pkg.go.dev/sync#RWMutex).
func (m *RWMutex) RLockSet(number int) {
	if !m.initialised.Load() {
		panic("attempt to use fair-mutex uninitialised")
	}

	start := time.Now()

	l := make(chan struct{})
	defer close(l)

	// Request the lock
	m.shared <- lockRequest{c: l, n: number}

	// Wait for the lock to be granted
	<-l

	m.histogram.Record(context.Background(), time.Since(start).Seconds(), metric.WithAttributes(
		append(m.config.metricAttributes, attribute.String("operation", "RLockSet"))...,
	))
}
