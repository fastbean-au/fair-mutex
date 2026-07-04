package fairmutex

import (
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// === Basic Test Suite ===

func TestFairMutexBasicOperations(t *testing.T) {
	t.Run("TestNew", func(t *testing.T) {
		m := New()
		defer m.Stop()

		if m == nil || !m.initialised.Load() {
			t.Fatal("New() did not return an initialized mutex")
		}
	})

	t.Run("TestUninitializedFairMutex", func(t *testing.T) {
		m := &RWMutex{} // Not initialized via New()

		// Expect panics for standard methods
		assertPanic(t, "Lock", func() { m.Lock() })
		assertPanic(t, "Unlock", func() { m.Unlock() })
		assertPanic(t, "RLock", func() { m.RLock() })
		assertPanic(t, "RUnlock", func() { m.RUnlock() })
		assertPanic(t, "TryLock", func() { m.TryLock() })
		assertPanic(t, "TryRLock", func() { m.TryRLock() })
		assertPanic(t, "RLockSet", func() { m.RLockSet(1) })
	})

	t.Run("TestWriteLock", func(t *testing.T) {
		m := New()
		defer m.Stop()

		var counter int32
		var wg sync.WaitGroup

		wg.Add(2)

		// Two goroutines trying to increment counter with write lock
		for range 2 {
			go func() {
				defer wg.Done()

				m.Lock()
				defer m.Unlock()

				current := atomic.LoadInt32(&counter)
				time.Sleep(10 * time.Millisecond) // Simulate work
				atomic.StoreInt32(&counter, current+1)
			}()
		}

		wg.Wait()

		if counter != 2 {
			t.Errorf("Expected counter to be 2, got %d", counter)
		}
	})

	t.Run("TestReadLock", func(t *testing.T) {
		m := New()
		defer m.Stop()

		var wg sync.WaitGroup

		wg.Add(5)

		// Multiple readers accessing shared resource
		for range 5 {
			go func() {
				defer wg.Done()

				m.RLock()
				defer m.RUnlock()

				time.Sleep(10 * time.Millisecond) // Simulate read
			}()
		}

		wg.Wait()
	})

	t.Run("TestReadLockSet", func(t *testing.T) {
		m := New()
		defer m.Stop()

		var wg sync.WaitGroup

		wg.Add(5)

		// Multiple readers accessing shared resource
		for range 5 {
			go func() {
				defer wg.Done()

				m.RLockSet(10)
				defer func() {
					for range 10 {
						m.RUnlock()
					}
				}()

				time.Sleep(10 * time.Millisecond) // Simulate read
			}()
		}

		wg.Wait()
	})

	t.Run("TestTryReadLock", func(t *testing.T) {
		m := New()
		defer m.Stop()

		m.Lock()

		if m.TryRLock() {
			t.Log("try lock granted when mutex was already locked")
			t.FailNow()
		}

		m.Unlock()

		if !m.TryRLock() {
			t.Log("try lock failed when mutex was unlocked")
			t.FailNow()
		}

		m.RUnlock()
	})

	t.Run("TestTryLock", func(t *testing.T) {
		m := New()
		defer m.Stop()

		m.RLock()

		if m.TryLock() {
			t.Log("try lock granted when mutex was already locked")
			t.FailNow()
		}

		m.RUnlock()

		if !m.TryLock() {
			t.Log("try lock failed when mutex was unlocked")
			t.FailNow()
		}

		m.Unlock()
	})

	t.Run("TestReadWriteExclusivity", func(t *testing.T) {
		m := New()
		defer m.Stop()

		var counter int32
		var wg sync.WaitGroup

		wg.Add(6)

		// One writer
		go func() {
			defer wg.Done()

			m.Lock()
			defer m.Unlock()

			atomic.AddInt32(&counter, 10)
			time.Sleep(50 * time.Millisecond) // Hold lock
		}()

		// Multiple readers
		for range 5 {
			go func() {
				defer wg.Done()

				m.RLock()
				defer m.RUnlock()

				_ = atomic.LoadInt32(&counter)
			}()
		}

		wg.Wait()

		if counter != 10 {
			t.Errorf("Expected counter to be 10, got %d", counter)
		}
	})

	t.Run("TestPanicUnlockingWithoutALock", func(t *testing.T) {
		m := New()
		defer m.Stop()

		// Expect panics for standard methods
		assertPanic(t, "Unlock", func() { m.Unlock() })
		assertPanic(t, "RUnlock", func() { m.RUnlock() })
	})

	t.Run("TestAfterStop", func(t *testing.T) {
		m := New()

		<-time.After(time.Millisecond * 10)

		m.Stop()

		<-time.After(time.Millisecond * 10)

		// Expect panics for standard methods
		assertPanic(t, "Lock", func() { m.Lock() })
		assertPanic(t, "Unlock", func() { m.Unlock() })
		assertPanic(t, "RLock", func() { m.RLock() })
		assertPanic(t, "RUnlock", func() { m.RUnlock() })
		assertPanic(t, "TryLock", func() { m.TryLock() })
		assertPanic(t, "TryRLock", func() { m.TryRLock() })
		assertPanic(t, "RLockSet", func() { m.RLockSet(1) })
	})

	t.Run("TestStopAfterStop", func(t *testing.T) {
		m := New()

		<-time.After(time.Millisecond * 10)

		m.Stop()

		<-time.After(time.Millisecond * 10)

		m.Stop()

		<-time.After(time.Millisecond * 10)

		// Expect panics for standard methods
		assertPanic(t, "Lock", func() { m.Lock() })
		assertPanic(t, "Unlock", func() { m.Unlock() })
		assertPanic(t, "RLock", func() { m.RLock() })
		assertPanic(t, "RUnlock", func() { m.RUnlock() })
		assertPanic(t, "TryLock", func() { m.TryLock() })
		assertPanic(t, "TryRLock", func() { m.TryRLock() })
		assertPanic(t, "RLockSet", func() { m.RLockSet(1) })
	})

	t.Run("TestStopAfterLock", func(t *testing.T) {
		m := New()

		<-time.After(time.Millisecond * 10)

		m.Lock()

		m.Stop()

		<-time.After(time.Millisecond * 10)

		// Expect panics for standard methods
		assertPanic(t, "Lock", func() { m.Lock() })
		assertPanic(t, "Unlock", func() { m.Unlock() })
		assertPanic(t, "RLock", func() { m.RLock() })
		assertPanic(t, "RUnlock", func() { m.RUnlock() })
		assertPanic(t, "TryLock", func() { m.TryLock() })
		assertPanic(t, "TryRLock", func() { m.TryRLock() })
		assertPanic(t, "RLockSet", func() { m.RLockSet(1) })
	})

	t.Run("TestStopAfterLockUnlock", func(t *testing.T) {
		m := New()

		<-time.After(time.Millisecond * 10)

		m.Lock()
		m.Unlock() //nolint:staticcheck

		// Allow time to wait for the next locks
		<-time.After(time.Millisecond)

		m.Stop()

		<-time.After(time.Millisecond * 10)

		// Expect panics for standard methods
		assertPanic(t, "Lock", func() { m.Lock() })
		assertPanic(t, "Unlock", func() { m.Unlock() })
		assertPanic(t, "RLock", func() { m.RLock() })
		assertPanic(t, "RUnlock", func() { m.RUnlock() })
		assertPanic(t, "TryLock", func() { m.TryLock() })
		assertPanic(t, "TryRLock", func() { m.TryRLock() })
		assertPanic(t, "RLockSet", func() { m.RLockSet(1) })
	})

	t.Run("TestStopAfterLocks", func(t *testing.T) {
		m := New()

		<-time.After(time.Millisecond * 10)

		m.Lock()

		// Each queued request is either granted before the stop signal is
		// processed, or woken into a panic by the drain in cleanup; either
		// way the goroutines must finish rather than leak.
		results := make(chan any, 2)

		for range 2 {
			go func() {
				defer func() { results <- recover() }()

				m.Lock()
			}()
		}

		// Allow time for the locks to be queued
		<-time.After(time.Millisecond)

		m.Unlock()

		m.Stop()

		timeout := time.After(5 * time.Second)

	RESULT_LOOP:
		for range 2 {
			select {

			case <-results:
				continue RESULT_LOOP

			case <-timeout:
				t.Fatal("a queued lock request neither completed nor panicked after Stop")

			}
		}

		// Expect panics for standard methods
		assertPanic(t, "Lock", func() { m.Lock() })
		assertPanic(t, "Unlock", func() { m.Unlock() })
		assertPanic(t, "RLock", func() { m.RLock() })
		assertPanic(t, "RUnlock", func() { m.RUnlock() })
		assertPanic(t, "TryLock", func() { m.TryLock() })
		assertPanic(t, "TryRLock", func() { m.TryRLock() })
		assertPanic(t, "RLockSet", func() { m.RLockSet(1) })
	})

	t.Run("TestStopAfterRLock", func(t *testing.T) {
		m := New()

		<-time.After(time.Millisecond * 10)

		m.RLock()

		m.Stop()

		<-time.After(time.Millisecond * 10)

		// Expect panics for standard methods
		assertPanic(t, "Lock", func() { m.Lock() })
		assertPanic(t, "Unlock", func() { m.Unlock() })
		assertPanic(t, "RLock", func() { m.RLock() })
		assertPanic(t, "RUnlock", func() { m.RUnlock() })
		assertPanic(t, "TryLock", func() { m.TryLock() })
		assertPanic(t, "TryRLock", func() { m.TryRLock() })
		assertPanic(t, "RLockSet", func() { m.RLockSet(1) })
	})

	t.Run("TestAfterCleanup", func(t *testing.T) {
		m := New()

		<-time.After(time.Millisecond * 10)

		m.Stop()

		<-time.After(time.Millisecond * 10)

		// Expect panics for standard methods
		assertPanic(t, "Lock", func() { m.Lock() })
		assertPanic(t, "Unlock", func() { m.Unlock() })
		assertPanic(t, "RLock", func() { m.RLock() })
		assertPanic(t, "RUnlock", func() { m.RUnlock() })
		assertPanic(t, "TryLock", func() { m.TryLock() })
		assertPanic(t, "TryRLock", func() { m.TryRLock() })
		assertPanic(t, "RLockSet", func() { m.RLockSet(1) })
	})

	t.Run("TestRLocker", func(t *testing.T) {
		m := New()
		defer m.Stop()

		<-time.After(time.Millisecond * 10)

		locker := m.RLocker()

		if locker == nil {
			t.Log("locker is nil")
			t.FailNow()
		}

		var wg sync.WaitGroup

		wg.Add(5)

		// Multiple readers accessing shared resource
		for range 5 {
			go func() {
				defer wg.Done()

				locker.Lock()
				defer locker.Unlock()

				time.Sleep(10 * time.Millisecond) // Simulate read
			}()
		}

		wg.Wait()

	})

	t.Run("TestOrderedLockingAllQueued", func(t *testing.T) {
		m := New(
			WithMaxReadQueueSize(1),
			WithMaxWriteQueueSize(1024),
		)
		defer m.Stop()

		out := make([]int, 0, 1000)

		var wg sync.WaitGroup

		// Lock the mutex initially to allow lock requests to be queued
		m.Lock()

		for i := 0; i < 1000; i++ {
			<-time.After(time.Millisecond) // Ensure that the funcs start in the correct order

			wg.Add(1)
			go func() {
				defer wg.Done()

				m.Lock()
				defer m.Unlock()

				out = append(out, i)
			}()
		}

		m.Unlock()

		// Wait for the go funcs to acquire and release the locks
		wg.Wait()

		for i, v := range out {
			if i != v {
				t.Errorf("Expected ordered value to be %d, got %d", i, v)
			}
		}
	})

	t.Run("TestOrderedLockingInExcessOfQueue", func(t *testing.T) {
		m := New(
			WithMaxReadQueueSize(1),
		)
		defer m.Stop()

		out := make([]int, 0, 1000)

		var wg sync.WaitGroup

		// Lock the mutex initially to allow lock requests to be queued
		m.Lock()

		for i := 0; i < 1000; i++ {
			<-time.After(time.Millisecond) // Ensure that the funcs start in the correct order

			wg.Add(1)
			go func() {
				defer wg.Done()

				m.Lock()
				defer m.Unlock()

				out = append(out, i)
			}()
		}

		m.Unlock()

		// Wait for the go funcs to acquire and release the locks
		wg.Wait()

		for i := 0; i < 256; i++ {
			if i != out[i] {
				t.Errorf("Expected ordered value to be %d, got %d", i, out[i])
			}
		}
	})

	t.Run("TestOrderedRLockingAllQueued", func(t *testing.T) {
		m := New(
			WithMaxReadBatchSize(1),
			WithMaxWriteQueueSize(1),
		)
		defer m.Stop()

		out := make([]int, 0, 1000)

		var wg sync.WaitGroup

		// Lock the mutex initially to allow lock requests to be queued
		m.Lock()

		for i := 0; i < 1000; i++ {
			<-time.After(time.Millisecond) // Ensure that the funcs start in the correct order

			wg.Add(1)
			go func() {
				defer wg.Done()

				m.RLock()
				defer m.RUnlock()

				// Note: in normal usage, this *should* be a Lock() not an RLock() for this type of operation.
				out = append(out, i)
			}()
		}

		m.Unlock()

		// Wait for the go funcs to acquire and release the locks
		wg.Wait()

		for i, v := range out {
			if i != v {
				t.Errorf("Expected ordered value to be %d, got %d", i, v)
			}
		}
	})

	t.Run("TestRLockQueueExceeded", func(t *testing.T) {
		m := New(WithMaxReadQueueSize(5))
		defer m.Stop()

		if m.HasRQueueBeenExceeded() {
			t.Fatal("HasRQueueBeenExceeded is true with no RLock queued")
		}

		// Lock the mutex so we can queue requests
		m.Lock()

		wg := new(sync.WaitGroup)
		wg.Add(5)

		for i := 0; i < 5; i++ {
			go func() {
				defer wg.Done()

				m.RLock()
				defer m.RUnlock() //nolint:staticcheck
			}()
		}

		// Delay to allow the request above to be executed
		<- time.After(time.Millisecond*5)

		if m.HasRQueueBeenExceeded() {
			t.Fatal("HasRQueueBeenExceeded is true with RLock queued")
		}

		wg.Add(1)
		go func() {
			defer wg.Done()

			m.RLock()
			defer m.RUnlock() //nolint:staticcheck
		}()

		// Delay to allow the request above to be executed
		<- time.After(time.Millisecond*5)

		if !m.HasRQueueBeenExceeded() {
			t.Fatal("HasRQueueBeenExceeded is false with RLock queue exceeded")
		}

		// Release the lock to allow the read locks to be granted
		m.Unlock()

		wg.Wait()

		<- time.After(time.Millisecond*5)
	})

	t.Run("TestLockQueueExceeded", func(t *testing.T) {
		m := New(WithMaxWriteQueueSize(5))
		defer m.Stop()

		if m.HasQueueBeenExceeded() {
			t.Fatal("HasQueueBeenExceeded is true with no Lock queued")
		}

		// Lock the mutex so we can queue requests
		m.RLock()

		wg := new(sync.WaitGroup)
		wg.Add(5)

		for i := 0; i < 5; i++ {
			go func() {
				defer wg.Done()

				m.Lock()
				defer m.Unlock() //nolint:staticcheck
			}()
		}

		// Delay to allow the request above to be executed
		<- time.After(time.Millisecond*5)

		if m.HasQueueBeenExceeded() {
			t.Fatal("HasQueueBeenExceeded is true with Lock queued")
		}

		wg.Add(1)
		go func() {
			defer wg.Done()

			m.Lock()
			defer m.Unlock() //nolint:staticcheck
		}()

		// Delay to allow the request above to be executed
		<- time.After(time.Millisecond*5)

		// Release the lock to allow the read locks to be granted
		m.RUnlock()

		wg.Wait()

		// We need to give time for the first batch of Locks to be granted and
		// released, and the final lock to get into the queue.
		<- time.After(time.Second)

		if !m.HasQueueBeenExceeded() {
			t.Fatal("HasQueueBeenExceeded is false with Lock queue exceeded")
		}
	})

}

// Helper function to assert panic
func assertPanic(t *testing.T, name string, f func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("%s: expected panic but none occurred", name)
		}
	}()

	f()
}

// === Extended Test Suite ===

func TestFairMutexFairness_ReadLocks(t *testing.T) {
	maxQueueSize := 100
	maxBatchSize := 10

	m := New(
		WithMaxReadQueueSize(maxQueueSize),
		WithMaxReadBatchSize(maxBatchSize),
	)
	defer m.Stop()

	var wg sync.WaitGroup
	order := make([]time.Time, maxQueueSize)

	// Get and hold a lock while we fill the queue
	m.Lock()

	wg.Add(maxQueueSize)

	for i := range maxQueueSize {
		go func() {
			defer wg.Done()

			m.RLock()
			defer m.RUnlock()

			// Record order
			order[i] = time.Now()

			// delay here for one record
			if i%10 == 9 {
				<-time.After(time.Second)
			}
		}()

		// Ensure that the queueing order is correct
		<-time.After(time.Millisecond * 5)
	}

	m.Unlock()

	wg.Wait()

	// Check that readers are processed in roughly FIFO order within batches
	outOfOrder := 0

	for i := range 9 {
		i1 := (i+1)*10 + 1
		i2 := i * 10
		if order[i1].Sub(order[i2]) < time.Millisecond*750 {
			t.Logf("[%d] %s <=> [%d] %s\n", i1, order[i1].Format("15:04:05.999"), i2, order[i2].Format("15:04:05.999"))
			outOfOrder++
		}
	}

	if outOfOrder > 0 {
		t.Errorf("Too many out-of-order readers (%d), expected <1 due to batching", outOfOrder)
	}
}

func TestFairMutexFairness_WriteLocks(t *testing.T) {
	m := New(
		WithMaxWriteQueueSize(20),
		WithMaxWriteBatchSize(5),
	)
	defer m.Stop()

	var wg sync.WaitGroup
	var order []int64
	var mu sync.Mutex
	const numWriters = 25

	m.RLock()

	wg.Add(numWriters)

	for i := range numWriters {
		id := int64(i)
		go func() {
			defer wg.Done()

			m.Lock()
			defer m.Unlock()

			mu.Lock()
			order = append(order, id)
			mu.Unlock()

		}()

		<-time.After(time.Millisecond * 5)
	}

	m.RUnlock()

	wg.Wait()

	if int64(len(order)) != numWriters {
		t.Errorf("Expected %d writers, got %d", numWriters, len(order))
	}

	// Strict FIFO: writers should be in order
	for i := 1; i < len(order); i++ {
		if order[i] != order[i-1]+1 {
			t.Errorf("Write lock order broken at index %d: %d -> %d", i, order[i-1], order[i])
		}
	}
}

func TestHighVolume_ReadContention(t *testing.T) {
	m := New(
		WithMaxReadQueueSize(2048),
		WithMaxReadBatchSize(256),
	)
	defer m.Stop()

	const numReaders = 10_000
	var wg sync.WaitGroup
	var active int32
	var maxActive int32

	wg.Add(numReaders)
	start := time.Now()

	for i := 0; i < numReaders; i++ {
		go func() {
			defer wg.Done()
			m.RLock()
			defer m.RUnlock()

			curr := atomic.AddInt32(&active, 1)
			if curr > atomic.LoadInt32(&maxActive) {
				atomic.CompareAndSwapInt32(&maxActive, atomic.LoadInt32(&maxActive), curr)
			}
			atomic.AddInt32(&active, -1)

			// Simulate work
			time.Sleep(time.Duration(rand.Intn(5)) * time.Microsecond)
		}()
	}

	wg.Wait()

	duration := time.Since(start)

	t.Logf("10k readers completed in %v, max concurrent: %d", duration, maxActive)

	if maxActive > 256 {
		t.Errorf("Max concurrent readers %d exceeds batch size 256", maxActive)
	}

	if maxActive == 0 {
		t.Error("No readers were active")
	}
}

func TestHighVolume_WriteContention(t *testing.T) {
	m := New(
		WithMaxWriteQueueSize(512),
		WithMaxWriteBatchSize(64),
	)
	defer m.Stop()

	const numWriters = 5000
	var wg sync.WaitGroup
	var counter int64
	var maxConcurrent int32

	wg.Add(numWriters)

	start := time.Now()

	for range numWriters {
		go func() {
			defer wg.Done()

			m.Lock()
			defer m.Unlock()

			curr := atomic.AddInt32(&maxConcurrent, 1)
			if curr > 1 {
				t.Error("Write lock allows concurrent writers")
			}

			atomic.AddInt32(&maxConcurrent, -1)

			atomic.AddInt64(&counter, 1)

			time.Sleep(10 * time.Microsecond)
		}()
	}

	wg.Wait()

	duration := time.Since(start)

	if counter != numWriters {
		t.Errorf("Expected %d writes, got %d", numWriters, counter)
	}

	t.Logf("5k writers completed in %v", duration)
}

func TestMixedReadWrite_StarvationPrevention(t *testing.T) {
	m := New(
		WithMaxReadQueueSize(512),
		WithMaxReadBatchSize(64),
		WithMaxWriteQueueSize(64),
		WithMaxWriteBatchSize(1), // Only one writer at a time
	)
	defer m.Stop()

	var wg sync.WaitGroup
	var readCount, writeCount atomic.Int32
	writerDone := make(chan struct{})

	// Start continuous readers
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for {
				select {
				case <-writerDone:
					return
				default:
					m.RLock()
					readCount.Add(1)
					time.Sleep(1 * time.Millisecond)
					m.RUnlock()
				}
			}
		}()
	}

	// Let readers warm up
	time.Sleep(50 * time.Millisecond)

	// Now inject a writer
	start := time.Now()

	m.Lock()
	writeCount.Add(1)
	time.Sleep(10 * time.Millisecond)
	m.Unlock()

	writerDuration := time.Since(start)

	close(writerDone)

	wg.Wait()

	if writerDuration > 150*time.Millisecond {
		t.Errorf("Writer starved for %.2fms — possible reader starvation", writerDuration.Seconds()*1000)
	}

	if writeCount.Load() != 1 {
		t.Error("Writer did not execute")
	}

	if readCount.Load() == 0 {
		t.Error("No readers executed")
	}

	t.Logf("Writer acquired lock in %.2fms under heavy read load", writerDuration.Seconds()*1000)
}

func TestBatchedReadProcessing(t *testing.T) {
	m := New(
		WithMaxReadQueueSize(10),
		WithMaxReadBatchSize(3),
	)
	defer m.Stop()

	var wg sync.WaitGroup
	var order = []int{}
	var orderMu sync.Mutex

	const numReaders = 15

	// Inject a writer to separate batches
	go func() {
		time.Sleep(10 * time.Millisecond)

		m.Lock()

		orderMu.Lock()
		order = append(order, 999)
		orderMu.Unlock()

		time.Sleep(10 * time.Millisecond)
		m.Unlock()
	}()

	m.RLock()

	wg.Add(numReaders)
	for i := range numReaders {
		id := i
		go func() {
			defer wg.Done()

			m.RLock()
			defer m.RUnlock()

			orderMu.Lock()
			order = append(order, id)
			orderMu.Unlock()

			time.Sleep(time.Millisecond * time.Duration(id))
		}()
	}

	m.RUnlock()

	wg.Wait()

	if order[0] == 999 {
		t.Error("Did not expect the write lock at the start")
	}

	if order[len(order)-1] == 999 {
		t.Error("Did not expect the write lock at the end")
	}
}

// TestTryLockDoesNotRecordQueueExceeded - exposes the bug where TryLock
// checked the length of the shared queue against the exclusive queue's
// capacity. With a write queue size of zero, a single TryLock on a free mutex
// wrongly recorded the write queue as having been exceeded.
//
// Note: queue sizes below one are now clamped to one, so the zero-size setup
// that originally exposed the bug no longer occurs; the test remains as a
// check that the Try methods never record queue-exceeded state.
func TestTryLockDoesNotRecordQueueExceeded(t *testing.T) {
	m := New(WithMaxWriteQueueSize(0))
	defer m.Stop()

	if !m.TryLock() {
		t.Fatal("TryLock failed on a free mutex")
	}

	m.Unlock()

	if m.HasQueueBeenExceeded() {
		t.Error("TryLock recorded the write queue as exceeded on a free mutex")
	}

	if m.HasRQueueBeenExceeded() {
		t.Error("TryLock recorded the read queue as exceeded on a free mutex")
	}
}

// TestTryLocksDoNotBlock - exposes the bug where two concurrent Try* calls
// could both pass the availability check and both enqueue, leaving the loser
// blocked until the winner released the lock - which is exactly what a
// try-lock must never do. Every Try* call must return promptly whether or not
// it obtained the lock.
func TestTryLocksDoNotBlock(t *testing.T) {
	const iterations = 1000
	const contenders = 4

	for range iterations {
		m := New()

		results := make(chan bool, contenders)

		// Half the contenders race for write locks, half for read locks
		for i := range contenders {
			exclusive := i%2 == 0
			go func() {
				if exclusive {
					results <- m.TryLock()
				} else {
					results <- m.TryRLock()
				}
			}()
		}

		// Any contender that fails to obtain the lock must return false
		// promptly; it must not wait for the lock to be released, as the
		// winner does not release until all contenders have reported.
		granted := 0
		timeout := time.After(5 * time.Second)

	CONTENDER_LOOP:
		for range contenders {
			select {

			case r := <-results:
				if r {
					granted++
				}

				continue CONTENDER_LOOP

			case <-timeout:
				t.Fatal("a Try* call blocked waiting for the lock to be released")

			}
		}

		if granted > 1 {
			t.Fatalf("%d Try* calls succeeded simultaneously; expected at most 1", granted)
		}

		// Whichever type won, a single release of that type is correct; the
		// winner reported true for exactly one of TryLock/TryRLock.
		if granted == 1 {
			if m.heldExclusive.Load() > 0 {
				m.Unlock()
			} else {
				m.RUnlock()
			}
		}

		m.Stop()
	}
}

// TestQueueExceededFlagsAreRaceFree - exposes the data race on the queue
// exceeded flags: they were plain bools written by every concurrent RLock and
// Lock caller and readable by any other goroutine. This test fails under the
// race detector against the unfixed code.
func TestQueueExceededFlagsAreRaceFree(t *testing.T) {
	m := New(WithMaxReadQueueSize(2), WithMaxWriteQueueSize(2))
	defer m.Stop()

	var wg sync.WaitGroup

	for i := range 16 {
		exclusive := i%2 == 0

		wg.Add(1)
		go func() {
			defer wg.Done()

			for range 200 {
				if exclusive {
					m.Lock()
					m.Unlock()
				} else {
					m.RLock()
					m.RUnlock()
				}

				_ = m.HasQueueBeenExceeded()
				_ = m.HasRQueueBeenExceeded()
			}
		}()
	}

	wg.Wait()
}

// TestStopIsIdempotentAndSynchronous - exposes two Stop() bugs: a second
// concurrent Stop() could load initialised before cleanup stored false and
// then block forever sending on stopProcessing, and Stop() could return
// before cleanup had run, leaving the mutex briefly marked as initialised.
func TestStopIsIdempotentAndSynchronous(t *testing.T) {
	for range 200 {
		m := New()

		done := make(chan struct{})

		for range 2 {
			go func() {
				defer func() { done <- struct{}{} }()

				m.Stop()
			}()
		}

		timeout := time.After(5 * time.Second)

	WAIT_LOOP:
		for range 2 {
			select {

			case <-done:
				continue WAIT_LOOP

			case <-timeout:
				t.Fatal("a concurrent Stop call blocked forever")

			}
		}

		if m.initialised.Load() {
			t.Fatal("mutex still marked initialised after Stop returned")
		}
	}
}

// TestStopWakesPendingLockRequests - exposes the goroutine leak where a lock
// request queued but not yet granted when Stop was called blocked forever
// waiting for a grant that could never come. Such requests must instead be
// woken into a panic.
func TestStopWakesPendingLockRequests(t *testing.T) {
	m := New()

	// Hold the write lock for the life of the test so that the queued
	// requests below can never be granted.
	m.Lock()

	pending := make(chan any, 2)

	go func() {
		defer func() { pending <- recover() }()

		m.Lock()
	}()

	go func() {
		defer func() { pending <- recover() }()

		m.RLock()
	}()

	// Allow time for the lock requests to be queued
	<-time.After(time.Millisecond * 10)

	// Stop with the lock still held and the requests still queued
	m.Stop()

	timeout := time.After(5 * time.Second)

WAIT_LOOP:
	for range 2 {
		select {

		case p := <-pending:
			if p == nil {
				t.Error("expected a queued lock request to panic when Stop was called")
			}

			continue WAIT_LOOP

		case <-timeout:
			t.Fatal("a queued lock request was leaked by Stop")

		}
	}
}

// TestRLockSetValidation - exposes the bug where RLockSet accepted a
// non-positive number of locks. RLockSet(0) as the only entry in a batch left
// waitingOnShared set forever - the release loop never ran - permanently
// breaking TryLock and TryRLock, and a negative number corrupted the release
// accounting for the whole batch.
func TestRLockSetValidation(t *testing.T) {
	m := New()
	defer m.Stop()

	assertPanic(t, "RLockSet(0)", func() { m.RLockSet(0) })
	assertPanic(t, "RLockSet(-1)", func() { m.RLockSet(-1) })
}

// TestRLockSetRecordsQueueExceeded - exposes the inconsistency where
// RLockSet, unlike RLock, never recorded that the read queue had been
// exceeded.
func TestRLockSetRecordsQueueExceeded(t *testing.T) {
	m := New(WithMaxReadQueueSize(5))
	defer m.Stop()

	// Lock the mutex so that read lock requests queue behind it
	m.Lock()

	wg := new(sync.WaitGroup)

	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()

			m.RLock()
			defer m.RUnlock() //nolint:staticcheck
		}()
	}

	// Delay to allow the requests above to fill the queue
	<-time.After(time.Millisecond * 5)

	if m.HasRQueueBeenExceeded() {
		t.Fatal("HasRQueueBeenExceeded is true with the read queue full but not exceeded")
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		m.RLockSet(2)
		defer func() {
			m.RUnlock()
			m.RUnlock()
		}()
	}()

	// Delay to allow the request above to be executed
	<-time.After(time.Millisecond * 5)

	if !m.HasRQueueBeenExceeded() {
		t.Fatal("HasRQueueBeenExceeded is false after RLockSet with the read queue exceeded")
	}

	// Release the lock to allow the read locks to be granted
	m.Unlock()

	wg.Wait()
}
