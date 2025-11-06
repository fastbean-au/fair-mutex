# Fair-Mutex

<div align="center">
  <img src="logo.png" alt="Fair-Mutex" width="200"/>
</div>

**fair-mutex** is a Go implementation of a fair RW mutex; that is, a mutex where write locks will not be prevented in a high volume read-lock use case. The larger the number of write locks required, the larger the performance benefit over `sync.RWMutex`. This is perhaps a fairly narrow use-case; if you don't need this then consider using [go-lock](https://github.com/viney-shih/go-lock) if the built-in `sync.RWMutex` or `sync.Mutex` do not meet your needs.

This implementation can be used as *functional* a drop-in replacement for Go's [`sync.RWMutex`](https://pkg.go.dev/sync#RWMutex) or [`sync.Mutex`](https://pkg.go.dev/sync#Mutex) as at Go 1.25 (*Note:* the `New()` method must be called to initialise the mutex prior to use, and the context used to create the mutex must be cancelled in order to release the resources associated with the mutex).

The general principle on which **fair-mutex** operates is that locks are given in batches alternating between write locks and read locks. The batch size is determined at the beginning of a locking cycle based on the number of requests for locks. Read locks are given concurrently for the entire batch, white write locks are given sequentially for the entire batch. While batches are being processed, both type of lock requests are queued. Batch sizes are simply the lesser of the number of locks queued of the lock type at the beginning of a cycle or the maximum size limit set for that lock type. So, in practice, what this means is that read locks are not automatically given if there is no write lock taken.

An OpenTelemetry (OTEL) metric is provided to record the lock wait times, allowing an evaluation of the effective performance of the mutex, and identification of problematic lock contention issues.

### Configuration options

**fair-mutex**  provides configurable read and write queue and batch size options, as well as an options for the metric name and default metric attributes.

#### WithMaxReadBatchSize
The maximum batch size for read (also known as shared) locks. The batch size does not determine the number of calls to obtain a lock that are waiting, but the maximum number that will be processed in one locking cycle.

This value cannot be larger than the MaxReadQueueSize.

Defaults to the value of MaxReadQueueSize.

#### WithMaxReadQueueSize
The maximum queue size for read (also known as shared) locks. The queue size does not determine the number of calls to obtain a lock that are waiting, but the number during which we can guarantee order. This setting will effect the memory required.

Set to 1 if this mutex will only be used as a write-only mutex (but you probably don't want to do that).

Defaults to 1024.

#### WithMaxWriteBatchSize
The maximum batch size for write (also known as exclusive) locks. The batch size does not determine the number of calls to obtain a lock that are waiting, but the maximum number that will be processed in one locking cycle.

This value cannot be larger than the MaxWriteQueueSize.

Defaults to 32.

#### WithMaxWriteQueueSize
The maximum queue size for write (also known as exclusive) locks. The queue size does not determine the number of calls to obtain a lock that are waiting, but the number during which we can guarantee order. This setting will effect the memory required.

Defaults to 256.

#### WithMetricAttributes
A set of attributes with pre-set values to provide on every recording of the mutex lock wait time metric.

WithMetricName - name for the metric.

Defaults to "go.mutex.wait.seconds".

## Installation

```bash
go get github.com/fastbean-au/fair-mutex
```

## Example usage

```bash
package main

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sync"
	"time"

	fairmutex "github.com/fastbean-au/fair-mutex"
)

func main() {

	// Create a context with a cancel function to use when creating the mutex.
	// Calling cancel() will release the resources used by the mutex (i.e. it
	// will end the go func and close the channels), preventing resource
	// leakage.
	ctx, cancel := context.WithCancel(context.Background())

	mtx := fairmutex.New(ctx)

	mtx.Lock()
	// Do something
	mtx.Unlock()

	mtx.RLock()
	// Do something
	mtx.RUnlock()

	<-time.After(time.Millisecond)

	if !mtx.TryLock() {
		fmt.Println("Couldn't get a lock")
	} else {
		fmt.Println("Have a lock")
		mtx.Unlock()
	}

	<-time.After(time.Millisecond)

	if !mtx.TryRLock() {
		fmt.Println("Couldn't get a read lock")
	} else {
		fmt.Println("Have a read lock")
		mtx.RUnlock()
	}

	wg := new(sync.WaitGroup)

	for range 1000 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if rand.IntN(5) == 4 {
				mtx.Lock()
				defer mtx.Unlock()
				<-time.After(time.Millisecond)
			} else {
				mtx.RLock()
				defer mtx.RUnlock()
				<-time.After(time.Millisecond)
			}
		}()
	}

	wg.Wait()

	cancel()
}
```

### Benchmarks

```
BenchmarkFairMutex_Read-8                                                1371511             874.7 ns/op             336 B/op          6 allocs/op
BenchmarkSyncRWMutex_Read-8                                             86194485             13.89 ns/op               0 B/op          0 allocs/op

BenchmarkFairMutex_Write-8                                               1296486             905.8 ns/op             336 B/op          6 allocs/op
BenchmarkSyncRWMutex_Write-8                                            64918521             18.67 ns/op               0 B/op          0 allocs/op

BenchmarkFairMutex_Write_UnderReadLoadWithGaps-8                            1063           1159781 ns/op             916 B/op         14 allocs/op
BenchmarkSyncRW_Write_UnderReadLoadWithGaps-8                               1065           1147985 ns/op             247 B/op          2 allocs/op

BenchmarkFairMutex_Write_UnderReadAndWriteLoad/writes=1-8                    998           1189580 ns/op            3293 B/op         52 allocs/op
BenchmarkSyncRW_Write_UnderReadAndWriteLoad/writes=1-8                      1029           1169390 ns/op            1278 B/op         16 allocs/op

BenchmarkFairMutex_Write_UnderReadAndWriteLoad/writes=2-8                   1008           1198702 ns/op            3654 B/op         59 allocs/op
BenchmarkSyncRW_Write_UnderReadAndWriteLoad/writes=2-8                       499           2326232 ns/op            2547 B/op         33 allocs/op

BenchmarkFairMutex_Write_UnderReadAndWriteLoad/writes=3-8                   1023           1222054 ns/op            4015 B/op         66 allocs/op
BenchmarkSyncRW_Write_UnderReadAndWriteLoad/writes=3-8                       340           3550499 ns/op            3804 B/op         48 allocs/op

BenchmarkFairMutex_Write_UnderReadAndWriteLoad/writes=4-8                    982           1211956 ns/op            4378 B/op         74 allocs/op
BenchmarkSyncRW_Write_UnderReadAndWriteLoad/writes=4-8                       259           4735191 ns/op            5067 B/op         64 allocs/op

BenchmarkFairMutex_Write_UnderReadAndWriteLoad/writes=5-8                    985           1245484 ns/op            4736 B/op         80 allocs/op
BenchmarkSyncRW_Write_UnderReadAndWriteLoad/writes=5-8                       204           5866520 ns/op            6335 B/op         80 allocs/op

BenchmarkFairMutex_Write_UnderReadAndWriteLoad/writes=6-8                   1042           1238162 ns/op            5097 B/op         88 allocs/op
BenchmarkSyncRW_Write_UnderReadAndWriteLoad/writes=6-8                       169           6973617 ns/op            7613 B/op         97 allocs/op

BenchmarkFairMutex_Write_UnderReadAndWriteLoad/writes=7-8                    963           1220483 ns/op            5453 B/op         94 allocs/op
BenchmarkSyncRW_Write_UnderReadAndWriteLoad/writes=7-8                       144           8126199 ns/op            8855 B/op        112 allocs/op

BenchmarkFairMutex_Write_UnderReadAndWriteLoad/writes=8-8                    950           1262626 ns/op            5813 B/op        101 allocs/op
BenchmarkSyncRW_Write_UnderReadAndWriteLoad/writes=8-8                       127           9399258 ns/op           10123 B/op        128 allocs/op

BenchmarkFairMutex_Write_UnderReadAndWriteLoad/writes=9-8                    928           1246964 ns/op            6178 B/op        109 allocs/op
BenchmarkSyncRW_Write_UnderReadAndWriteLoad/writes=9-8                       100          10664838 ns/op           11401 B/op        145 allocs/op

BenchmarkFairMutex_Write_UnderReadAndWriteLoad/writes=10-8                   922           1243630 ns/op            6533 B/op        115 allocs/op
BenchmarkSyncRW_Write_UnderReadAndWriteLoad/writes=10-8                      100          11763149 ns/op           12658 B/op        161 allocs/op
```