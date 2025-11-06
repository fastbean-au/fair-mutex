package fairmutex

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// Benchmark: Read lock
func BenchmarkSyncRWMutex_Read(b *testing.B) {
	m := new(sync.RWMutex)

	b.ResetTimer()

	for b.Loop() {
		m.RLock()
		m.RUnlock() //nolint:staticcheck
	}
}

// Benchmark: Write lock
func BenchmarkSyncRWMutex_Write(b *testing.B) {
	m := new(sync.RWMutex)

	b.ResetTimer()

	for b.Loop() {
		m.Lock()
		m.Unlock() //nolint:staticcheck
	}
}

// Benchmark: Write under read pressure
func BenchmarkSyncRW_Write_UnderReadLoadWithGaps(b *testing.B) {
	ctx := b.Context()

	m := new(sync.RWMutex)

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

	for b.Loop() {
		m.Lock()
		m.Unlock() //nolint:staticcheck
	}
}

func BenchmarkSyncRW_Write_UnderReadAndWriteLoad(b *testing.B) {
	ctx := b.Context()

	m := new(sync.RWMutex)

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

	go func() {
		<-time.After(time.Microsecond * 100)
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

	go func() {
		<-time.After(time.Microsecond * 200)
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

	go func() {
		<-time.After(time.Microsecond * 300)
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

	go func() {
		<-time.After(time.Microsecond * 400)
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

	for i := range 10 {
		b.Run(fmt.Sprintf("writes=%d", i+1), func(b *testing.B) {
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
