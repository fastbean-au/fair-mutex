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

/*
	Note: because of the presence of the Lock() method, the go vet copylocks
	check will be applied without anything else required in this code.

	A note on the documentation:

	The README file uses read/write semantics for the mutex keeping in line with
	the names of the methods. Internally, the code and comments will typically
	use shared and exclusive semantics, better(?) reflecting the nature of the
	mutex and the implementation.

	To prevent any doubt at all, a read lock is a shared lock, and a write lock
	is an exclusive lock.

	A note on performance:

	This implementation of a mutex performs much worse than the built-in RWMutex
	from the sync package.

	I have explored using sync.Pool to facilitate the re-use of the response
	channels. It performed worse.

	I have also explored the use of a channel to provide a pool of pre-created
	response channels. This also performed slightly worse.
*/

type RWMutex struct {
	// True when initialisation of the mutex is complete. used to prevent un-
	// initialised use of the mutex (i.e. when the mutex will not function as
	// a mutex).
	initialised atomic.Bool

	config *config

	// The OpenTelemetry metric which we record lock wait times.
	histogram metric.Float64Histogram

	// Pre-created attribute sets for the lock wait histogram
	rLockSetAttrs attribute.Set
	rLockAttrs    attribute.Set
	lockAttrs     attribute.Set

	// Channel to hold exclusive lock requests
	exclusiveQueue chan *lockRequest

	// Channel to hold shared lock requests
	sharedQueue chan *lockRequest

	// Channel used to communicate releasing write/exclusive locks
	releaseExclusive chan chan struct{}

	releaseShared chan chan struct{}

	// Channel used to signal to the processing go func that it should stop
	// processing locks and exit.
	stopProcessing chan struct{}

	// When true, we are waiting on exclusive locks to be released (exclusive
	// locks are being held).
	waitingOnExclusive atomic.Bool

	// When true, we are waiting on shared locks to be released (shared locks
	// are being held).
	waitingOnShared atomic.Bool

	// HasQueueBeenExceeded - true if the capacity of the write lock queue has
	// been filled to the point of probable overflow.
	//
	// While this is not an absolute indicator that request ordering has not
	// been able to be maintained due to the number of lock requests being
	// queued, it is a very strong indicator that the queue size might need to
	// be increased in order to maintain lock request ordering.
	HasQueueBeenExceeded bool

	// HasRQueueBeenExceeded - true if the capacity of the read lock queue has
	// been filled to the point of probable overflow.
	//
	// While this is not an absolute indicator that request ordering has not
	// been able to be maintained due to the number of lock requests being
	// queued, it is a very strong indicator that the queue size might need to
	// be increased in order to maintain lock request ordering.
	HasRQueueBeenExceeded bool
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
		exclusiveQueue:   make(chan *lockRequest, cfg.exclusiveMaxQueueSize),
		sharedQueue:      make(chan *lockRequest, cfg.sharedMaxQueueSize),
		releaseExclusive: make(chan chan struct{}),
		releaseShared:    make(chan chan struct{}, cfg.sharedMaxBatchSize),
		stopProcessing:   make(chan struct{}),
		rLockSetAttrs:    attribute.NewSet(append(cfg.metricAttributes, attribute.String("operation", "RLockSet"))...),
		rLockAttrs:       attribute.NewSet(append(cfg.metricAttributes, attribute.String("operation", "RLock"))...),
		lockAttrs:        attribute.NewSet(append(cfg.metricAttributes, attribute.String("operation", "Lock"))...),
	}

	go m.process()

	m.initialised.Store(true)

	return m
}

// Lock processing states
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

	var loopInitLock *lockRequest
	var lockItems int
	lockTypeTaken := noLockTaken

	for {
		switch lockTypeTaken {

		// If the last batch processed was shared, give exclusive locks, if any are waiting
		case sharedLockTaken:
			select {

			case <-m.stopProcessing:
				return

			case loopInitLock = <-m.exclusiveQueue:
				m.waitingOnExclusive.Store(true)
				lockItems = min(len(m.exclusiveQueue), m.config.exclusiveMaxBatchSize-1)
				lockTypeTaken = exclusiveLockTaken

			default:
				lockTypeTaken = noLockTaken

			}

		// If the last batch processed was exclusive, give shared locks, if any are waiting
		case exclusiveLockTaken:
			select {

			case <-m.stopProcessing:
				return

			case loopInitLock = <-m.sharedQueue:
				m.waitingOnShared.Store(true)
				lockItems = min(len(m.sharedQueue), m.config.sharedMaxBatchSize-1)
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

			case loopInitLock = <-m.exclusiveQueue:
				m.waitingOnExclusive.Store(true)
				lockItems = min(len(m.exclusiveQueue), m.config.exclusiveMaxBatchSize-1)
				lockTypeTaken = exclusiveLockTaken

			case loopInitLock = <-m.sharedQueue:
				m.waitingOnShared.Store(true)
				lockItems = min(len(m.sharedQueue), m.config.sharedMaxBatchSize-1)
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
				l := <-m.sharedQueue

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
					u <- struct{}{}
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
				u <- struct{}{}

				// Remaining exclusive lock requests
				for n := 1; n <= lockItems; n++ {
					// Grab the lock request
					l := <-m.exclusiveQueue

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
						u <- struct{}{}
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

	close(m.exclusiveQueue)
	close(m.sharedQueue)
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

	r := make(chan struct{})
	defer close(r)

	// Record if the queue has exceeded capacity (or is likely to exceed capacity).
	if !m.HasRQueueBeenExceeded && len(m.sharedQueue) == m.config.sharedMaxQueueSize {
		m.HasRQueueBeenExceeded = true
	}

	// Request the lock
	m.sharedQueue <- &lockRequest{c: r, n: 1}

	// Wait for the lock to be granted
	<-r

	m.histogram.Record(context.Background(), time.Since(start).Seconds(), metric.WithAttributeSet(m.rLockAttrs))
}

// TryRLock - tries to lock rw for reading and reports whether it succeeded.
//
// Note that while correct uses of TryRLock do exist, they are rare, and use of
// TryRLock is often a sign of a deeper problem in a particular use of mutexes.
func (m *RWMutex) TryRLock() bool {
	if !m.initialised.Load() {
		panic("attempt to use fair-mutex uninitialised")
	}

	r := make(chan struct{})
	defer close(r)

	if m.waitingOnExclusive.Load() || m.waitingOnShared.Load() || len(m.exclusiveQueue) > 0 || len(m.sharedQueue) > 0 {
		return false
	}

	// Record if the queue has exceeded capacity (or is likely to exceed capacity).
	if !m.HasRQueueBeenExceeded && len(m.sharedQueue) == m.config.sharedMaxQueueSize {
		m.HasRQueueBeenExceeded = true
	}

	// Request the lock
	m.sharedQueue <- &lockRequest{c: r, n: 1}

	// Wait for the lock to be granted
	<-r

	return true
}

// RUnlock - undoes a single RLock or TryRLock call that succeeded; it does not
// affect other simultaneous readers.
//
// It is a run-time error if rw is not locked for reading on entry to RUnlock.
func (m *RWMutex) RUnlock() {
	if !m.initialised.Load() {
		panic("attempt to use fair-mutex uninitialised")
	}

	if !m.waitingOnShared.Load() {
		panic("fair-mutex: RUnlock of unlocked RWMutex")
	}

	r := make(chan struct{})
	defer close(r)

	m.releaseShared <- r

	// Wait for the lock to be released
	<-r
}

// Lock - locks the mutex for writing. If the mutex is already locked for
// reading or writing, Lock blocks until the lock is available.
func (m *RWMutex) Lock() {
	if !m.initialised.Load() {
		panic("attempt to use fair-mutex uninitialised")
	}

	start := time.Now()

	// Record if the queue has exceeded capacity (or is likely to exceed capacity).
	if !m.HasQueueBeenExceeded && len(m.exclusiveQueue) == m.config.exclusiveMaxQueueSize {
		m.HasQueueBeenExceeded = true
	}

	r := make(chan struct{})
	defer close(r)

	// Request the lock
	m.exclusiveQueue <- &lockRequest{c: r, n: 1}

	// Wait for the lock to be granted
	<-r

	m.histogram.Record(context.Background(), time.Since(start).Seconds(), metric.WithAttributeSet(m.lockAttrs))
}

// TryLock - tries to lock rw for writing and reports whether it succeeded.
//
// Note that while correct uses of TryLock do exist, they are rare, and use of
// TryLock is often a sign of a deeper problem in a particular use of mutexes.
func (m *RWMutex) TryLock() bool {
	if !m.initialised.Load() {
		panic("attempt to use fair-mutex uninitialised")
	}

	// See if there are any lock requests or locks active
	if m.waitingOnExclusive.Load() || m.waitingOnShared.Load() || len(m.exclusiveQueue) > 0 || len(m.sharedQueue) > 0 {
		return false
	}

	// Record if the queue has exceeded capacity (or is likely to exceed capacity).
	if !m.HasQueueBeenExceeded && len(m.sharedQueue) == m.config.exclusiveMaxQueueSize {
		m.HasQueueBeenExceeded = true
	}

	r := make(chan struct{})
	defer close(r)

	// Request the lock
	m.exclusiveQueue <- &lockRequest{c: r, n: 1}

	// Wait on the lock to be granted
	<-r

	return true
}

// Unlock - undoes a single Lock or TryLock call that succeeded.
//
// It is a run-time error if the mutex is not locked for writing on entry to
// Unlock.
//
// A locked mutex is not associated with a particular goroutine or lock request.
// One goroutine may Lock a mutex and then arrange for another goroutine to
// Unlock it.
func (m *RWMutex) Unlock() {
	if !m.initialised.Load() {
		panic("attempt to use fair-mutex uninitialised")
	}

	if !m.waitingOnExclusive.Load() {
		panic("fair-mutex: Unlock of unlocked RWMutex")
	}

	r := make(chan struct{})
	defer close(r)

	m.releaseExclusive <- r

	// Wait for the lock to be released
	<-r
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

	r := make(chan struct{})
	defer close(r)

	// Request the lock
	m.sharedQueue <- &lockRequest{c: r, n: number}

	// Wait for the lock to be granted
	<-r

	m.histogram.Record(context.Background(), time.Since(start).Seconds(), metric.WithAttributeSet(m.rLockSetAttrs))
}
