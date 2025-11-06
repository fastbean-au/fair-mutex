# Fair-Mutex

<div align="center">
  <img src="logo.png" alt="Fair-Mutex" width="200"/>
</div>

[![Go Reference](https://pkg.go.dev/badge/github.com/fastbean-au/fair-mutex.svg)](https://pkg.go.dev/github.com/fastbean-au/fair-mutex)
[![Go Report Card](https://goreportcard.com/badge/github.com/fastbean-au/fair-mutex)](https://goreportcard.com/report/github.com/fastbean-au/fair-mutex)

**fair-mutex** is a Go implementation of a fair RW mutex; that is, a mutex where write locks will not be prevented in a high volume read-lock use case. The larger the number of write locks required, the larger the performance benefit over `sync.RWMutex`. This is perhaps a fairly narrow use-case; if you don't need this then consider using [go-lock](https://github.com/viney-shih/go-lock) if the built-in `sync.RWMutex` or `sync.Mutex` do not meet your needs. To see if perhaps **fair-mutex** meets your needs, start by looking at the benchmark results. 

This implementation can be used as *functional* a drop-in replacement for Go's [`sync.RWMutex`](https://pkg.go.dev/sync#RWMutex) or [`sync.Mutex`](https://pkg.go.dev/sync#Mutex) as at Go 1.25 (*Note:* the `New()` function must be called to initialise the mutex prior to use, and the `Stop()` method must be called in order to release the resources associated with the mutex. *NB*: calling any method on the mutex after calling `Stop()` will result in a panic).

The general principle on which **fair-mutex** operates is that locks are given in batches alternating between write locks and read locks. The batch size is determined at the beginning of a locking cycle based on the number of requests for locks. Read locks are given concurrently for the entire batch, white write locks are given sequentially for the entire batch. While batches are being processed, both type of lock requests are queued. Batch sizes are simply the lesser of the number of locks queued of the lock type at the beginning of a cycle or the maximum size limit set for that lock type. So, in practice, what this means is that read locks are not automatically given if there is no write lock taken.

An OpenTelemetry (OTEL) metric is provided to record the lock wait times, allowing an evaluation of the effective performance of the mutex, and identification of problematic lock contention issues.

**Caution**: like a `sync.Mutex` or a `sync.RWMutex`, **fair-mutex** cannot be safely copied, unlike with `sync.Mutex` AND `sync.RWMutex`, **fair-mutex** cannot be copied at any time.

## Configuration options

**fair-mutex**  provides configurable read and write queue and batch size options, as well as an options for the metric name and default metric attributes.

### WithMaxReadBatchSize
The maximum batch size for read (also known as shared) locks. The batch size does not determine the number of calls to obtain a lock that are waiting, but the maximum number that will be processed in one locking cycle.

This value cannot be larger than the MaxReadQueueSize.

Defaults to the value of MaxReadQueueSize.

### WithMaxReadQueueSize
The maximum queue size for read (also known as shared) locks. The queue size does not determine the number of calls to obtain a lock that are waiting, but the number during which we can guarantee order. This setting will effect the memory required.

Set to 1 if this mutex will only be used as a write-only mutex (but you probably don't want to do that).

Defaults to 1024.

### WithMaxWriteBatchSize
The maximum batch size for write (also known as exclusive) locks. The batch size does not determine the number of calls to obtain a lock that are waiting, but the maximum number that will be processed in one locking cycle.

This value cannot be larger than the MaxWriteQueueSize.

Defaults to 32.

### WithMaxWriteQueueSize
The maximum queue size for write (also known as exclusive) locks. The queue size does not determine the number of calls to obtain a lock that are waiting, but the number during which we can guarantee order. This setting will effect the memory required.

Defaults to 256.

### WithMetricAttributes
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
	"fmt"
	"math/rand/v2"
	"sync"
	"time"

	fairmutex "github.com/fastbean-au/fair-mutex"
)

func main() {

	mtx := fairmutex.New()

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

    // Stop the mutex to release the resources
	mtx.Stop()
}
```

### Benchmarks

Side-by-side comparison of `fair-mutex` and `sync.RWMutex`.

| Test | Operations | NS/Operation | Memory Bytes/Op | Memory Allocs/Op |
|------|       ---: |         ---: |            ---: |             ---: |
|Fair-Mutex Read|1,371,511|874.7|336|6 |
|SyncRWMutex Read|86,194,485 |13.89|0|0 |
||||||
|Fair-Mutex Write|1,296,486|905.8|336|6 |
|SyncRWMutex Write|64,918,521|18.67|0|0 |
||||||
|Fair-Mutex UnderReadLoadWithGaps|1,063 |1,159,781|916|14 |
|SyncRWMutex UnderReadLoadWithGaps|1,065|1,147,985|247|2 |
||||||
|Fair-Mutex UnderReadAndWriteLoad/WriteLocks=1|998|1,189,580|3,293|52 |
|SyncRWMutex UnderReadAndWriteLoad/WriteLocks=1|1,029|1,169,390|1,278|16 |
||||||
|Fair-Mutex UnderReadAndWriteLoad/WriteLocks=2|1,008|1,198,702|3,654|59 |
|SyncRWMutex UnderReadAndWriteLoad/WriteLocks=2|499|2,326,232|2,547|33 |
||||||
|Fair-Mutex UnderReadAndWriteLoad/WriteLocks=3|1,023|1,222,054|4,015|66 |
|SyncRWMutex UnderReadAndWriteLoad/WriteLocks=3|340|3,550,499|3,804|48 |
||||||
|Fair-Mutex UnderReadAndWriteLoad/WriteLocks=4|982|1,211,956|4,378|74 |
|SyncRWMutex UnderReadAndWriteLoad/WriteLocks=4|259|4,735,191|5,067|64 |
||||||
|Fair-Mutex UnderReadAndWriteLoad/WriteLocks=5|985|1,245,484|4,736|80 |
|SyncRWMutex UnderReadAndWriteLoad/WriteLocks=5|204|5,866,520|6,335|80 |
||||||
|Fair-Mutex UnderReadAndWriteLoad/WriteLocks=6|1,042|1,238,162|5,097|88 |
|SyncRWMutex UnderReadAndWriteLoad/WriteLocks=6|169|6,973,617|7,613|97 |
||||||
|Fair-Mutex UnderReadAndWriteLoad/WriteLocks=7|963|1,220,483|5,453|94 |
|SyncRWMutex UnderReadAndWriteLoad/WriteLocks=7|144|8,126,199|8,855|112 |
||||||
|Fair-Mutex UnderReadAndWriteLoad/WriteLocks=8|950|1,262,626|5,813|101 |
|SyncRWMutex UnderReadAndWriteLoad/WriteLocks=8|127|9,399,258|10,123|128 |
||||||
|Fair-Mutex UnderReadAndWriteLoad/WriteLocks=9|928|1,246,964|6,178|109 |å
|SyncRWMutex UnderReadAndWriteLoad/WriteLocks=9|100|10,664,838|11,401|145 |
||||||
|Fair-Mutex UnderReadAndWriteLoad/WriteLocks=10|922|1,243,630|6533|115 |
|SyncRWMutex UnderReadAndWriteLoad/WriteLocks=10|100|11,763,149|12,658|161 |
