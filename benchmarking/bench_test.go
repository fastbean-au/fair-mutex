package benchmarking

/*
   This benchmark suite compares sync.RWMutex and fairmutex.RWMutex.

   Methodology:

   - Implementations are called through a typed interface (no reflection), so
     the per-operation adapter cost is a single virtual dispatch for both.
   - All access to the shared data is inside the appropriate critical section:
     readers sum a fixed window of the data under RLock, and writers fill the
     same window under Lock. The data does not grow, so the workload is
     stationary and runs are comparable.
   - Latency benchmarks use a single measured goroutine so that ns/op is the
     cost of one lock cycle. Each operation is individually timed, and the
     p50/p99/max latencies are reported as extra metrics; a fair mutex's value
     shows in the tail, which a mean alone hides. Per-operation timing adds
     ~50ns of sampling overhead to each op.
   - Background load goroutines are created and torn down per sub-benchmark,
     with a fresh mutex per sub-benchmark, so there is no cross-contamination
     between sub-benchmarks and no shared timeout to expire mid-run.

   Benchmark families:

   - Uncontended/Lock, Uncontended/RLock: the raw cost of a lock cycle with no
     contention. This is where sync.RWMutex is strongest.
   - SaturatedReadThroughput: per-read cost with all processors reading
     concurrently and no writers. Also sync.RWMutex home turf.
   - WriteLatency_SaturatedReads/Writers=N: the latency of a write lock while
     100 spinning readers saturate the mutex, with N-1 additional contending
     writers. This is the scenario fair-mutex is designed for.
   - ReadLatency_ContendingWrites/Readers=N: the latency of a read lock while
     4 writers continuously contend, with N-1 additional readers.
*/

import (
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	fairmutex "github.com/fastbean-au/fair-mutex"
)

const (
	// The size of the shared data slice.
	dataSize = 1024

	// The number of elements read or written within a critical section.
	workWindow = 64

	// The number of spinning readers used to saturate the mutex in the
	// write-latency benchmarks.
	saturatingReaders = 100

	// The number of continuously contending writers in the read-latency
	// benchmarks.
	contendingWriters = 4
)

// benchSink accumulates results of the read work so that the compiler cannot
// eliminate the critical sections.
var benchSink atomic.Int64

type rwLocker interface {
	Lock()
	Unlock()
	RLock()
	RUnlock()
}

// The second return value is a cleanup function, called when the benchmark is
// finished with the mutex.
var lockImplementations = []struct {
	name    string
	newLock func() (rwLocker, func())
}{
	{
		name: "sync.RWMutex",
		newLock: func() (rwLocker, func()) {
			return &sync.RWMutex{}, func() {}
		},
	},
	{
		name: "fairmutex",
		newLock: func() (rwLocker, func()) {
			m := fairmutex.New()

			return m, m.Stop
		},
	},
}

// readWork - the critical section for readers: sum a fixed window of the
// shared data. Must be called with the read lock held.
func readWork(data []int) int {
	total := 0

	for i := 0; i < workWindow; i++ {
		total += data[i]
	}

	return total
}

// writeWork - the critical section for writers: fill a fixed window of the
// shared data. Must be called with the write lock held.
func writeWork(data []int, value int) {
	for i := 0; i < workWindow; i++ {
		data[i] = value + i
	}
}

// reportLatencyPercentiles - reports the p50, p99, and maximum of the sampled
// per-operation latencies as extra benchmark metrics.
func reportLatencyPercentiles(b *testing.B, latencies []time.Duration) {
	if len(latencies) == 0 {
		return
	}

	slices.Sort(latencies)

	p50 := latencies[len(latencies)/2]
	p99 := latencies[min(len(latencies)-1, len(latencies)*99/100)]
	worst := latencies[len(latencies)-1]

	b.ReportMetric(float64(p50.Nanoseconds()), "p50-ns")
	b.ReportMetric(float64(p99.Nanoseconds()), "p99-ns")
	b.ReportMetric(float64(worst.Nanoseconds()), "max-ns")
}

type latencyConfig struct {
	backgroundReaders int
	backgroundWriters int

	// When true the measured goroutine takes write locks; otherwise it takes
	// read locks.
	measureWrites bool
}

// runLatencyBenchmark - measures the latency of lock cycles taken by a single
// goroutine while background readers and writers contend for the same mutex.
// The critical section work is inside the lock and included in the timing.
func runLatencyBenchmark(b *testing.B, newLock func() (rwLocker, func()), cfg latencyConfig) {
	m, cleanup := newLock()
	defer cleanup()

	data := make([]int, dataSize)

	stop := make(chan struct{})

	wg := new(sync.WaitGroup)

	for range cfg.backgroundReaders {
		wg.Add(1)
		go func() {
			defer wg.Done()

			total := 0

			for {
				select {

				case <-stop:
					benchSink.Add(int64(total))

					return

				default:
					m.RLock()
					total += readWork(data)
					m.RUnlock()

				}
			}
		}()
	}

	for w := range cfg.backgroundWriters {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for {
				select {

				case <-stop:
					return

				default:
					m.Lock()
					writeWork(data, w)
					m.Unlock()

				}
			}
		}()
	}

	// Allow the background load to establish before measuring
	time.Sleep(10 * time.Millisecond)

	latencies := make([]time.Duration, 0, b.N)
	total := 0

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		start := time.Now()

		if cfg.measureWrites {
			m.Lock()
			writeWork(data, n)
			m.Unlock()
		} else {
			m.RLock()
			total += readWork(data)
			m.RUnlock()
		}

		latencies = append(latencies, time.Since(start))
	}

	b.StopTimer()

	benchSink.Add(int64(total))

	close(stop)
	wg.Wait()

	reportLatencyPercentiles(b, latencies)
}

func benchmarkUncontendedLock(b *testing.B, newLock func() (rwLocker, func())) {
	m, cleanup := newLock()
	defer cleanup()

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		m.Lock()
		m.Unlock()
	}
}

func benchmarkUncontendedRLock(b *testing.B, newLock func() (rwLocker, func())) {
	m, cleanup := newLock()
	defer cleanup()

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		m.RLock()
		m.RUnlock()
	}
}

// benchmarkSaturatedReadThroughput - the per-read cost with every processor
// reading concurrently and no writers present.
func benchmarkSaturatedReadThroughput(b *testing.B, newLock func() (rwLocker, func())) {
	m, cleanup := newLock()
	defer cleanup()

	data := make([]int, dataSize)

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		total := 0

		for pb.Next() {
			m.RLock()
			total += readWork(data)
			m.RUnlock()
		}

		benchSink.Add(int64(total))
	})
}

func BenchmarkLocks(b *testing.B) {
	for _, impl := range lockImplementations {
		b.Run(impl.name, func(b *testing.B) {
			b.Run("Uncontended/Lock", func(b *testing.B) {
				benchmarkUncontendedLock(b, impl.newLock)
			})

			b.Run("Uncontended/RLock", func(b *testing.B) {
				benchmarkUncontendedRLock(b, impl.newLock)
			})

			b.Run("SaturatedReadThroughput", func(b *testing.B) {
				benchmarkSaturatedReadThroughput(b, impl.newLock)
			})

			for i := 1; i <= 10; i++ {
				b.Run(fmt.Sprintf("WriteLatency_SaturatedReads/Writers=%d", i), func(b *testing.B) {
					runLatencyBenchmark(b, impl.newLock, latencyConfig{
						backgroundReaders: saturatingReaders,
						backgroundWriters: i - 1,
						measureWrites:     true,
					})
				})
			}

			for i := 1; i <= 10; i++ {
				b.Run(fmt.Sprintf("ReadLatency_ContendingWrites/Readers=%d", i), func(b *testing.B) {
					runLatencyBenchmark(b, impl.newLock, latencyConfig{
						backgroundReaders: i - 1,
						backgroundWriters: contendingWriters,
						measureWrites:     false,
					})
				})
			}
		})
	}
}
