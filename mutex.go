package fairmutex

import (
	"context"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type Mutex struct {
	initialised        bool
	config             *config
	histogram          metric.Float64Histogram
	exclusive          chan chan struct{}
	shared             chan chan struct{}
	releaseExclusive   chan struct{}
	releaseShared      chan struct{}
	waitingOnExclusive atomic.Bool
	waitingOnShared    atomic.Bool
}

func New(ctx context.Context, options ...Option) *Mutex {
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

	m := &Mutex{
		initialised:      true,
		config:           cfg,
		histogram:        histogram,
		exclusive:        make(chan chan struct{}, cfg.exclusiveMaxQueueSize),
		shared:           make(chan chan struct{}, cfg.sharedMaxQueueSize),
		releaseExclusive: make(chan struct{}),
		releaseShared:    make(chan struct{}, cfg.sharedMaxBatchSize),
	}

	go m.process(ctx)

	return m
}

const (
	NoLockTaken = iota
	SharedLockTaken
	ExclusiveLockTaken
)

// process - handles the batching and granting of the lock requests
func (m *Mutex) process(ctx context.Context) {
	defer m.cleanup()

	var loopInitLock chan struct{}
	var lockItems int
	lockTypeTaken := NoLockTaken

	for {
		switch lockTypeTaken {

		// If the last batch processed was shared, give exclusive locks, if any are waiting
		case SharedLockTaken:
			select {

			case <-ctx.Done():
				return

			case loopInitLock = <-m.exclusive:
				m.waitingOnExclusive.Store(true)
				lockItems = min(len(m.exclusive), m.config.exclusiveMaxBatchSize)
				lockTypeTaken = ExclusiveLockTaken

			default:
				lockTypeTaken = NoLockTaken

			}

		// If the last batch processed was exclusive, give shared locks, if any are waiting
		case ExclusiveLockTaken:
			select {

			case <-ctx.Done():
				return

			case loopInitLock = <-m.shared:
				m.waitingOnShared.Store(true)
				lockItems = min(len(m.shared), m.config.sharedMaxBatchSize)
				lockTypeTaken = SharedLockTaken

			default:
				lockTypeTaken = NoLockTaken

			}

		}

		// If this is the initial loop or there were no locks waiting of the opposite type to the last batch, then
		// we'll randomly select which type of lock to grant (given one of each type is requested simultaneously).
		if lockTypeTaken == NoLockTaken {
			select {

			case <-ctx.Done():
				return

			case loopInitLock = <-m.exclusive:
				m.waitingOnExclusive.Store(true)
				lockItems = min(len(m.exclusive), m.config.exclusiveMaxBatchSize)
				lockTypeTaken = ExclusiveLockTaken

			case loopInitLock = <-m.shared:
				m.waitingOnShared.Store(true)
				lockItems = min(len(m.shared), m.config.sharedMaxBatchSize)
				lockTypeTaken = SharedLockTaken

			}
		}

		// Action the locks - grant and wait until they are released
		switch lockTypeTaken {

		case SharedLockTaken:
			// Process shared locks - grant the locks
			loopInitLock <- struct{}{} // Initial lock

			for range lockItems {
				// Grab the lock request
				l := <-m.shared

				// Signal to the requester that they now have the lock
				l <- struct{}{}
			}

			// Wait for the shared locks to be returned
			for range lockItems + 1 {
				select {
				case <-ctx.Done():
					return
				case <-m.releaseShared:
				}
			}

			m.waitingOnShared.Store(false)

		case ExclusiveLockTaken:
			// Process exclusive locks - grant and wait for each lock to be returned

			// Initial loop lock
			// Signal to the requester that they now have the lock
			loopInitLock <- struct{}{}

			// Wait for the lock to be returned
			select {
			case <-ctx.Done():
				return
			case <-m.releaseExclusive:
			}

			// Remaining exclusive lock requests
			for range lockItems {
				// Grab the lock request
				l := <-m.exclusive

				// Signal to the requester that they now have the lock
				l <- struct{}{}

				// Wait for the lock to be returned
				select {
				case <-ctx.Done():
					return
				case <-m.releaseExclusive:
				}
			}

			m.waitingOnExclusive.Store(false)

		}
	}
}

// cleanup - ensures that we don't have any resource leakage, provided the
// context used to create the mutex has been cancelled.
func (m *Mutex) cleanup() {
	m.initialised = false

	close(m.exclusive)
	close(m.shared)
	close(m.releaseExclusive)
	close(m.releaseShared)
}

// -----------------------------------------------------------------------------
// sync.RWMutex methods
// -----------------------------------------------------------------------------

func (m *Mutex) RLock() {
	if !m.initialised {
		panic("attempt to use fair-mutex uninitialised")
	}

	start := time.Now()

	l := make(chan struct{})
	defer close(l)

	// Request the lock
	m.shared <- l

	// Wait for the lock to be granted
	<-l

	m.histogram.Record(context.Background(), time.Since(start).Seconds(), metric.WithAttributes(
		append(m.config.metricAttributes, attribute.String("operation", "RLock"))...,
	))
}

func (m *Mutex) TryRLock() bool {
	if !m.initialised {
		panic("attempt to use fair-mutex uninitialised")
	}

	l := make(chan struct{})
	defer close(l)

	if m.waitingOnExclusive.Load() || m.waitingOnShared.Load() || len(m.exclusive) > 0 || len(m.shared) > 0 {
		return false
	}

	// Request the lock
	m.shared <- l

	// Wait for the lock to be granted
	<-l

	return true
}

func (m *Mutex) RUnlock() {
	if !m.initialised {
		panic("attempt to use fair-mutex uninitialised")
	}

	if !m.waitingOnShared.Load() {
		panic("fair-mutex: RUnlock of unlocked RWMutex")
	}

	m.releaseShared <- struct{}{}
}

func (m *Mutex) Lock() {
	if !m.initialised {
		panic("attempt to use fair-mutex uninitialised")
	}

	start := time.Now()

	l := make(chan struct{})
	defer close(l)

	// Request the lock
	m.exclusive <- l

	// Wait for the lock to be granted
	<-l

	m.histogram.Record(context.Background(), time.Since(start).Seconds(), metric.WithAttributes(
		append(m.config.metricAttributes, attribute.String("operation", "Lock"))...,
	))
}

func (m *Mutex) TryLock() bool {
	if !m.initialised {
		panic("attempt to use fair-mutex uninitialised")
	}

	l := make(chan struct{})
	defer close(l)

	// See if there are any lock requests or locks active
	if m.waitingOnExclusive.Load() || m.waitingOnShared.Load() || len(m.exclusive) > 0 || len(m.shared) > 0 {
		return false
	}

	// Request the lock
	m.exclusive <- l

	// Wait on the lock to be granted
	<-l

	return true
}

func (m *Mutex) Unlock() {
	if !m.initialised {
		panic("attempt to use fair-mutex uninitialised")
	}

	if !m.waitingOnExclusive.Load() {
		panic("fair-mutex: Unlock of unlocked RWMutex")
	}

	m.releaseExclusive <- struct{}{}
}
