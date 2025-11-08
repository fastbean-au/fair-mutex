package fairmutex

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// Benchmark: Read lock
func BenchmarkFairMutex_Read(b *testing.B) {
	m := New(
		WithMaxReadQueueSize(1024),
		WithMaxReadBatchSize(128),
	)

	defer m.Stop()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		m.RLock()
		m.RUnlock() //nolint:staticcheck
	}
}

// Benchmark: Write lock
func BenchmarkFairMutex_Write(b *testing.B) {
	m := New(
		WithMaxReadQueueSize(1024),
		WithMaxReadBatchSize(128),
	)

	defer m.Stop()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		m.Lock()
		m.Unlock() //nolint:staticcheck
	}
}

// Benchmark: Write under read pressure
func BenchmarkFairMutex_Write_UnderReadLoadWithGaps(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := New()
	defer m.Stop()

	// Keep readers running
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				m.RLock()

				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Millisecond):
					m.RUnlock()
				}
			}
		}
	}()

	time.Sleep(10 * time.Millisecond) // Warm up

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		m.Lock()
		m.Unlock() //nolint:staticcheck
	}
}

func BenchmarkFairMutex_Write_UnderReadAndWriteLoad(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := New()
	defer m.Stop()

	// Keep readers running
	for i := range 5 {
		go func() {
			for {
				<-time.After(time.Microsecond * time.Duration(i))
				select {
				case <-ctx.Done():
					return
				default:
					m.RLock()

					select {
					case <-ctx.Done():
						return
					case <-time.After(time.Millisecond):
						m.RUnlock()
					}
				}
			}
		}()
	}

	time.Sleep(10 * time.Millisecond) // Warm up

	b.ResetTimer()

	for i := range 10 {
		b.Run(fmt.Sprintf("WriteLocks=%d", i+1), func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				wg := new(sync.WaitGroup)
				wg.Add(i + 1)
				for range i + 1 {
					go func() {
						defer wg.Done()
						m.Lock()
						m.Unlock() //nolint:staticcheck
					}()
				}
				wg.Wait()
			}
		})
	}
}
