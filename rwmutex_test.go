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

		<-time.After(time.Millisecond)

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

		<-time.After(time.Millisecond)

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
