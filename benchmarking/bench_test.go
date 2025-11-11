package benchmarking

/*
   This benchmark suite compares multiple implementations of read-write locks
   (including sync.RWMutex and fairmutex.RWMutex) under controlled conditions.
   It dynamically adapts to any type with Lock, Unlock, RLock, and RUnlock methods.

   Features:
   - Automatically benchmarks all configured lock constructors.
   - Enforces a per-benchmark timeout (60s) internally.
   - Excludes workload time from benchmark measurements.
   - Includes tests for both write and read locks under mixed loads.
*/

import (
	"context"
	"fmt"
	"math/rand/v2"
	"reflect"
	"sync"
	"testing"
	"time"

	fairmutex "github.com/fastbean-au/fair-mutex"
)

// --------------------------------------
// Reflection-based lock helpers
// --------------------------------------

type lockAdapter struct{ v reflect.Value }

func newLockAdapter(x any) *lockAdapter { return &lockAdapter{v: reflect.ValueOf(x)} }

func (l *lockAdapter) call(method string) {
	m := l.v.MethodByName(method)
	if !m.IsValid() {
		panic(fmt.Sprintf("missing method %q on %T", method, l.v.Interface()))
	}
	m.Call(nil)
}

func (l *lockAdapter) Lock()   { l.call("Lock") }
func (l *lockAdapter) Unlock() { l.call("Unlock") }
func (l *lockAdapter) RLock()  { l.call("RLock") }
func (l *lockAdapter) RUnlock() { l.call("RUnlock") }

// --------------------------------------
// Benchmark timeout wrapper
// --------------------------------------

func runWithTimeout(b *testing.B, d time.Duration, fn func(ctx context.Context)) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	done := make(chan struct{})
	go func() {
		fn(ctx)
		close(done)
	}()
	select {
	case <-done:
		return
	case <-ctx.Done():
		b.Fatalf("benchmark exceeded timeout (%v)", d)
	}
}

// --------------------------------------
// Benchmark helpers (write-focused)
// --------------------------------------

func benchmarkReadLock(b *testing.B, newLock func() any) {
	runWithTimeout(b, 60*time.Second, func(ctx context.Context) {
		m := newLockAdapter(newLock())
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			select {
			case <-ctx.Done():
				return
			default:
				m.RLock()
				m.RUnlock()
			}
		}
	})
}

func benchmarkWriteLock(b *testing.B, newLock func() any) {
	runWithTimeout(b, 60*time.Second, func(ctx context.Context) {
		m := newLockAdapter(newLock())
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			select {
			case <-ctx.Done():
				return
			default:
				m.Lock()
				m.Unlock()
			}
		}
	})
}

func benchmarkWriteUnderReadPressure(b *testing.B, newLock func() any) {
	runWithTimeout(b, 60*time.Second, func(ctx context.Context) {
		m := newLockAdapter(newLock())

		// background readers
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				default:
					m.RLock()
					time.Sleep(time.Millisecond)
					m.RUnlock()
				}
			}
		}()

		time.Sleep(10 * time.Millisecond)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			select {
			case <-ctx.Done():
				return
			default:
				m.Lock()
				m.Unlock()
			}
		}
	})
}

func benchmarkWriteUnderReadAndWriteLoad(b *testing.B, newLock func() any) {
	runWithTimeout(b, 60*time.Second, func(ctx context.Context) {
		m := newLockAdapter(newLock())

		// background readers
		for i := 0; i < 5; i++ {
			time.Sleep(time.Microsecond * time.Duration(i))
			go func() {
				for {
					select {
					case <-ctx.Done():
						return
					default:
						m.RLock()
						time.Sleep(time.Millisecond)
						m.RUnlock()
					}
				}
			}()
		}

		time.Sleep(10 * time.Millisecond)

		for i := 0; i < 10; i++ {
			b.Run(fmt.Sprintf("Writers=%d", i+1), func(b *testing.B) {
				b.ResetTimer()
				for n := 0; n < b.N; n++ {
					select {
					case <-ctx.Done():
						return
					default:
					}
					wg := sync.WaitGroup{}
					wg.Add(i + 1)
					for w := 0; w < i+1; w++ {
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
	})
}

func benchmarkWriteUnderReadAndWriteLoad_WithWork(b *testing.B, newLock func() any, work func(data *[]int)) {
	runWithTimeout(b, 60*time.Second, func(ctx context.Context) {
		m := newLockAdapter(newLock())
		data := make([]int, 0)

		// background readers
		for i := 0; i < 1000; i++ {
			time.Sleep(time.Microsecond * time.Duration(i))
			go func() {
				for {
					select {
					case <-ctx.Done():
						return
					default:
						m.RLock()
						_ = len(data)
						m.RUnlock()
					}
				}
			}()
		}

		time.Sleep(time.Millisecond)

		for i := 0; i < 10; i++ {
			b.Run(fmt.Sprintf("Writers=%d", i+1), func(b *testing.B) {
				for n := 0; n < b.N; n++ {
					select {
					case <-ctx.Done():
						return
					default:
					}
					wg := sync.WaitGroup{}
					wg.Add(i + 1)
					for w := 0; w < i+1; w++ {
						go func() {
							defer wg.Done()
							b.StopTimer()
							work(&data)
							b.StartTimer()
							m.Lock()
							m.Unlock() //nolint:staticcheck
						}()
					}
					wg.Wait()
				}
			})
		}
	})
}

// --------------------------------------
// NEW: Read benchmarks under write load
// --------------------------------------

// Read locks while writers are running
func benchmarkReadUnderWritePressure(b *testing.B, newLock func() any) {
	runWithTimeout(b, 60*time.Second, func(ctx context.Context) {
		m := newLockAdapter(newLock())

		// background writers
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				default:
					m.Lock()
					time.Sleep(time.Millisecond)
					m.Unlock()
				}
			}
		}()

		time.Sleep(10 * time.Millisecond)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			select {
			case <-ctx.Done():
				return
			default:
				m.RLock()
				m.RUnlock()
			}
		}
	})
}

// Read locks under concurrent write and read load
func benchmarkReadUnderReadAndWriteLoad(b *testing.B, newLock func() any) {
	runWithTimeout(b, 60*time.Second, func(ctx context.Context) {
		m := newLockAdapter(newLock())

		// background writers
		for i := 0; i < 5; i++ {
			time.Sleep(time.Microsecond * time.Duration(i))
			go func() {
				for {
					select {
					case <-ctx.Done():
						return
					default:
						m.Lock()
						time.Sleep(time.Millisecond)
						m.Unlock()
					}
				}
			}()
		}

		time.Sleep(10 * time.Millisecond)

		for i := 0; i < 10; i++ {
			b.Run(fmt.Sprintf("Readers=%d", i+1), func(b *testing.B) {
				b.ResetTimer()
				for n := 0; n < b.N; n++ {
					select {
					case <-ctx.Done():
						return
					default:
					}
					wg := sync.WaitGroup{}
					wg.Add(i + 1)
					for r := 0; r < i+1; r++ {
						go func() {
							defer wg.Done()
							m.RLock()
							m.RUnlock() //nolint:staticcheck
						}()
					}
					wg.Wait()
				}
			})
		}
	})
}

// Read locks under concurrent writers performing small work
func benchmarkReadUnderReadAndWriteLoad_WithWork(b *testing.B, newLock func() any, work func(data *[]int)) {
	runWithTimeout(b, 60*time.Second, func(ctx context.Context) {
		m := newLockAdapter(newLock())
		data := make([]int, 0)

		// background writers
		for i := 0; i < 100; i++ {
			time.Sleep(time.Microsecond * time.Duration(i))
			go func() {
				for {
					select {
					case <-ctx.Done():
						return
					default:
						b.StopTimer()
						work(&data)
						b.StartTimer()
						m.Lock()
						m.Unlock()
					}
				}
			}()
		}

		time.Sleep(time.Millisecond)

		for i := 0; i < 10; i++ {
			b.Run(fmt.Sprintf("Readers=%d", i+1), func(b *testing.B) {
				for n := 0; n < b.N; n++ {
					select {
					case <-ctx.Done():
						return
					default:
					}
					wg := sync.WaitGroup{}
					wg.Add(i + 1)
					for r := 0; r < i+1; r++ {
						go func() {
							defer wg.Done()
							m.RLock()
							m.RUnlock() //nolint:staticcheck
						}()
					}
					wg.Wait()
				}
			})
		}
	})
}

// --------------------------------------
// Benchmark registry and entry point
// --------------------------------------

var lockConstructors = map[string]func() any{
	"sync.RWMutex": func() any { return &sync.RWMutex{} },
	"fairmutex":    func() any { return fairmutex.New() },
}

func BenchmarkLocks(b *testing.B) {
	for name, factory := range lockConstructors {
		b.Run(name, func(b *testing.B) {
			// Write lock benchmarks
			b.Run("WriteLock", func(b *testing.B) {
				benchmarkWriteLock(b, factory)
			})
			b.Run("Write_UnderReadPressure", func(b *testing.B) {
				benchmarkWriteUnderReadPressure(b, factory)
			})
			b.Run("Write_UnderReadAndWriteLoad", func(b *testing.B) {
				benchmarkWriteUnderReadAndWriteLoad(b, factory)
			})
			b.Run("Write_UnderReadAndWriteLoad_TrivialWork", func(b *testing.B) {
				work := func(data *[]int) {
					*data = append(*data, rand.IntN(1000000))
				}
				benchmarkWriteUnderReadAndWriteLoad_WithWork(b, factory, work)
			})
			b.Run("Write_UnderReadAndWriteLoad_ModestWork", func(b *testing.B) {
				work := func(data *[]int) {
					n := rand.IntN(1000000)
					for _, v := range *data {
						n += v
					}
					*data = append(*data, n)
				}
				benchmarkWriteUnderReadAndWriteLoad_WithWork(b, factory, work)
			})

			// Read lock benchmarks
			b.Run("ReadLock", func(b *testing.B) {
				benchmarkReadLock(b, factory)
			})
			b.Run("Read_UnderWritePressure", func(b *testing.B) {
				benchmarkReadUnderWritePressure(b, factory)
			})
			b.Run("Read_UnderReadAndWriteLoad", func(b *testing.B) {
				benchmarkReadUnderReadAndWriteLoad(b, factory)
			})
			b.Run("Read_UnderReadAndWriteLoad_TrivialWork", func(b *testing.B) {
				work := func(data *[]int) {
					*data = append(*data, rand.IntN(1000000))
				}
				benchmarkReadUnderReadAndWriteLoad_WithWork(b, factory, work)
			})
			b.Run("Read_UnderReadAndWriteLoad_ModestWork", func(b *testing.B) {
				work := func(data *[]int) {
					n := rand.IntN(1000000)
					for _, v := range *data {
						n += v
					}
					*data = append(*data, n)
				}
				benchmarkReadUnderReadAndWriteLoad_WithWork(b, factory, work)
			})
		})
	}
}
