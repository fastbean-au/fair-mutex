package fairmutex

import (
	"context"
	"testing"
	"time"
)

// Benchmark: Read lock
func BenchmarkFairMutex_Read(b *testing.B) {
	m := New(b.Context(),
		WithMaxReadQueueSize(1024),
		WithMaxReadBatchSize(128),
	)

	b.ResetTimer()

	for b.Loop() {
		m.RLock()
		m.RUnlock() //nolint:staticcheck
	}
}

// Benchmark: Write lock
func BenchmarkFairMutex_Write(b *testing.B) {
	m := New(b.Context(),
		WithMaxReadQueueSize(1024),
		WithMaxReadBatchSize(128),
	)

	b.ResetTimer()

	for b.Loop() {
		m.Lock()
		m.Unlock() //nolint:staticcheck
	}
}

// Benchmark: Write under read pressure
func BenchmarkFairMutex_Write_UnderReadLoad(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())

	m := New(b.Context())

	// Keep readers running
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				m.RLock()
				time.Sleep(1 * time.Microsecond)
				m.RUnlock()
			}
		}
	}()

	time.Sleep(10 * time.Millisecond) // Warm up

	b.ResetTimer()

	for b.Loop() {
		m.Lock()
		m.Unlock() //nolint:staticcheck
	}

	cancel()
}
