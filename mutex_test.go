package fairmutex

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMutexBasicOperations(t *testing.T) {
	t.Run("TestNew", func(t *testing.T) {
		m := New()
		defer m.Stop()

		if m == nil || !m.initialised {
			t.Fatal("New() did not return an initialized mutex")
		}
	})

	t.Run("TestUninitializedMutex", func(t *testing.T) {
		m := &Mutex{} // Not initialized via New()

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

		var counter int32
		var wg sync.WaitGroup

		wg.Add(2)

		// Two goroutines trying to increment counter with write lock
		for range 2 {
			go func() {
				defer wg.Done()

				locker.Lock()
				defer locker.Unlock()

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
