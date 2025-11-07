# Fair-Mutex

<div align="center">
  <img src="assets/logo.png" alt="Fair-Mutex" width="200"/>
</div>

[![Go Report Card](https://goreportcard.com/badge/github.com/fastbean-au/fair-mutex)](https://goreportcard.com/report/github.com/fastbean-au/fair-mutex)
[![Coverage Status](https://coveralls.io/repos/github/fastbean-au/fair-mutex/badge.svg?branch=main)](https://coveralls.io/github/fastbean-au/fair-mutex?branch=main)
![Dependabot](https://img.shields.io/badge/dependabot-enabled-brightgreen)
[![Known Vulnerabilities](https://snyk.io/test/github/fastbean-au/fair-mutex/badge.svg)](https://snyk.io/test/github/fastbean-au/fair-mutex)
[![Go Reference](https://pkg.go.dev/badge/github.com/fastbean-au/fair-mutex.svg)](https://pkg.go.dev/github.com/fastbean-au/fair-mutex)


**fair-mutex** is a Go implementation of a fair RW mutex; that is, a mutex where write locks will not be prevented in a high volume read-lock use case. The larger the number of write locks required, the larger the performance benefit over `sync.RWMutex`. This is perhaps a fairly narrow use-case; if you don't need this then consider using [go-lock](https://github.com/viney-shih/go-lock) if the built-in `sync.RWMutex` or `sync.Mutex` do not meet your needs. To see if **fair-mutex** might meet your needs, start by looking at the benchmark results.

<div align="center">
  <img src="assets/comparison.jpg" alt="Comparison" width="300"/>
</div>

This implementation can be used as *functional* a drop-in replacement for Go's [`sync.RWMutex`](https://pkg.go.dev/sync#RWMutex) or [`sync.Mutex`](https://pkg.go.dev/sync#Mutex) as at Go 1.25 (with limitations - please see below). (*Note:* the `New()` function must be called to initialise the mutex prior to use, and the `Stop()` method must be called in order to release the resources associated with the mutex. *NB*: calling any method on the mutex after calling `Stop()` will result in a panic).

The general principle on which **fair-mutex** operates is that locks are given in batches alternating between write locks and read locks. The batch size is determined at the beginning of a locking cycle based on the number of requests for locks. Read locks are given concurrently for the entire batch, white write locks are given sequentially for the entire batch. While batches are being processed, both type of lock requests are queued. Batch sizes are simply the lesser of the number of locks queued of the lock type at the beginning of a cycle or the maximum size limit set for that lock type. So, in practice, what this means is that read locks are not automatically given if there is no write lock taken.

An OpenTelemetry (OTEL) metric is provided to record the lock wait times, allowing an evaluation of the effective performance of the mutex, and identification of problematic lock contention issues.

## Limitations

Because of the way that **fair-mutex** batches locking, there is a scenario where it can cause a deadlock. This scenario is exposed by the `sync.RWMutex` unit test [https://cs.opensource.google/go/go/+/master:src/sync/rwmutex_test.go;l=28](doParallelReaders). Briefly, this occurs when a set of locks must be granted before any locks are released. To address this issue, use the `RLockSet(n)` method to request a set of read locks. No matter how many read locks are requested in a set, only the set itself counts towards the batch limit.

Like `sync.Mutex` or `sync.RWMutex`, **fair-mutex** cannot be safely copied; however, unlike `sync.Mutex` and `sync.RWMutex`, **fair-mutex** cannot be copied at any time.

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

| Test                                            | Operations | NS/Operation | Memory Bytes/Op | Memory Allocs/Op |
| ----------------------------------------------- | ---------: | -----------: | --------------: | ---------------: |
| Fair-Mutex Read                                 |  1,380,320 |        856.0 |             336 |                6 |
| SyncRWMutex Read                                | 85,814,031 |        14.06 |               0 |                0 |
|                                                 |            |              |                 |                  |
| Fair-Mutex Write                                |  1,430,215 |        840.0 |             336 |                6 |
| SyncRWMutex Write                               | 57,727,576 |        18.76 |               0 |                0 |
|                                                 |            |              |                 |                  |
| Fair-Mutex UnderReadLoadWithGaps                |      1,052 |    1,155,759 |             916 |               14 |
| SyncRWMutex UnderReadLoadWithGaps               |      1,065 |    1,079,798 |             231 |                2 |
|                                                 |            |              |                 |                  |
| Fair-Mutex UnderReadAndWriteLoad/WriteLocks=1   |      1,062 |    1,223,412 |           4,371 |               65 |
| SyncRWMutex UnderReadAndWriteLoad/WriteLocks=1  |      1,036 |    1,189,577 |           2,333 |               29 |
|                                                 |            |              |                 |                  |
| Fair-Mutex UnderReadAndWriteLoad/WriteLocks=2   |      1,004 |    1,207,823 |           4,768 |               73 |
| SyncRWMutex UnderReadAndWriteLoad/WriteLocks=2  |        519 |    2,359,645 |           4,417 |               55 |
|                                                 |            |              |                 |                  |
| Fair-Mutex UnderReadAndWriteLoad/WriteLocks=3   |      1,006 |    1,215,710 |           5,196 |               81 |
| SyncRWMutex UnderReadAndWriteLoad/WriteLocks=3  |        336 |    3,540,986 |           6,553 |               82 |
|                                                 |            |              |                 |                  |
| Fair-Mutex UnderReadAndWriteLoad/WriteLocks=4   |      1,044 |    1,243,997 |           5,583 |               88 |
| SyncRWMutex UnderReadAndWriteLoad/WriteLocks=4  |        252 |    4,726,490 |           8,759 |              109 |
|                                                 |            |              |                 |                  |
| Fair-Mutex UnderReadAndWriteLoad/WriteLocks=5   |        993 |    1,227,232 |           5,948 |               95 |
| SyncRWMutex UnderReadAndWriteLoad/WriteLocks=5  |        205 |    5,959,623 |          11,397 |              142 |
|                                                 |            |              |                 |                  |
| Fair-Mutex UnderReadAndWriteLoad/WriteLocks=6   |        981 |    1,261,444 |           6,314 |              102 |
| SyncRWMutex UnderReadAndWriteLoad/WriteLocks=6  |        170 |    7,181,301 |          13,814 |              172 |
|                                                 |            |              |                 |                  |
| Fair-Mutex UnderReadAndWriteLoad/WriteLocks=7   |      1,032 |    1,224,795 |           6,680 |              109 |
| SyncRWMutex UnderReadAndWriteLoad/WriteLocks=7  |        145 |    8,291,939 |          15,529 |              193 |
|                                                 |            |              |                 |                  |
| Fair-Mutex UnderReadAndWriteLoad/WriteLocks=8   |        970 |    1,254,130 |           7,041 |              116 |
| SyncRWMutex UnderReadAndWriteLoad/WriteLocks=8  |        127 |    9,519,004 |          18,017 |              224 |
|                                                 |            |              |                 |                  |
| Fair-Mutex UnderReadAndWriteLoad/WriteLocks=9   |        957 |    1,279,178 |           7,406 |              123 |
| SyncRWMutex UnderReadAndWriteLoad/WriteLocks=9  |        100 |   10,633,652 |          19,358 |              241 |
|                                                 |            |              |                 |                  |
| Fair-Mutex UnderReadAndWriteLoad/WriteLocks=10  |        957 |    1,253,480 |           7,763 |              130 |
| SyncRWMutex UnderReadAndWriteLoad/WriteLocks=10 |        100 |   11,867,612 |          22,725 |              282 |
