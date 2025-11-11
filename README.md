# Fair-Mutex

<div align="center">
  <img src="assets/logo.png" alt="Fair-Mutex" width="200"/>
</div>

[![Go Report Card](https://goreportcard.com/badge/github.com/fastbean-au/fair-mutex)](https://goreportcard.com/report/github.com/fastbean-au/fair-mutex)
[![Coverage Status](https://coveralls.io/repos/github/fastbean-au/fair-mutex/badge.svg?branch=main)](https://coveralls.io/github/fastbean-au/fair-mutex?branch=main)
![Dependabot](https://img.shields.io/badge/dependabot-enabled-brightgreen)
[![Known Vulnerabilities](https://snyk.io/test/github/fastbean-au/fair-mutex/badge.svg)](https://snyk.io/test/github/fastbean-au/fair-mutex)
[![Go Reference](https://pkg.go.dev/badge/github.com/fastbean-au/fair-mutex.svg)](https://pkg.go.dev/github.com/fastbean-au/fair-mutex)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/fastbean-au/fair-mutex)


**fair-mutex** is a Fair RW Mutex for Go.

`fair-mutex` is a mutex where write locks will not be significantly delayed in a high volume read-lock use case, while providing a limited guarantee of honouring lock request ordering (see below in [limitations](#limitations)). The larger the number of write locks required under a persistent read lock demand, the larger the performance benefit over `sync.RWMutex` that can be seen (and this is the only scenario where you might see a performance benefit - in all other cases `fair-mutex` *will* be less performant).

These are perhaps fairly narrow use-cases; if you do not need either of these, then you would perhaps be better off using [go-lock](https://github.com/viney-shih/go-lock) if the built-in `sync.RWMutex` or `sync.Mutex` does not meet your needs.

This implementation can be used as *functional* a drop-in replacement for Go's [`sync.RWMutex`](https://pkg.go.dev/sync#RWMutex) or [`sync.Mutex`](https://pkg.go.dev/sync#Mutex) as at Go 1.25 (with [limitations](#limitations)).

The general principle on which `fair-mutex` operates is that locks are given in batches alternating between write locks and read locks when both types of lock requests are queued. The batch size is determined at the beginning of a locking cycle, and are simply the lesser of the number of locks queued of the lock type at the beginning of a cycle or the maximum size limit set for that lock type.

Read locks are given concurrently for the entire batch, white write locks are given (and returned) sequentially for the entire batch.

While batches are being processed, both type of lock requests are queued.

Additionally, an OpenTelemetry (OTEL) metric is provided to record the lock wait times, allowing an evaluation of the effective performance of the mutex, and identification of problematic lock contention issues.

## Installation

```bash
go get github.com/fastbean-au/fair-mutex
```

## Usage

*Note:* The `New()` function must be called to initialise the mutex prior to use, and the `Stop()` method must be called in order to release the resources associated with the mutex.

*NB*: Calling any method on the mutex after calling `Stop()` will result in a panic.

### Example usage

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

## Limitations

1. Because of the way that **fair-mutex** batches locking, there is a scenario where it can cause a deadlock. This scenario is exposed by the `sync.RWMutex` unit test [doParallelReaders](https://cs.opensource.google/go/go/+/master:src/sync/rwmutex_test.go;l=28) when modified to run `fair-mutex`. Briefly, this occurs when a set of locks must be granted before any locks are released. To overcome this limitation, use the `RLockSet(n)` method to request a set of read locks. No matter how many read locks are requested in a set, only the set itself counts towards the batch limit.

2. Like `sync.Mutex` or `sync.RWMutex`, `fair-mutex` cannot be safely copied; however, unlike `sync.Mutex` and `sync.RWMutex`, `fair-mutex` cannot be copied at any time.

3. The ordering of locks is maintained as long as the number of locks of a type being requested does not exceed the queue size for that type of lock. Once exceeded, the ordering is no longer guaranteed until a new batch begins with the queue not in an overflow state.

## Configuration options

**fair-mutex** provides configurable read and write queue and batch size options, as well as an options for the metric name and default metric attributes.

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

### WithMetricName

The name for the metric.

Defaults to "go.mutex.wait.seconds".

## Go Benchmark Results: `sync.RWMutex` vs `fairmutex`

<div align="center">
  <img src="assets/trivial_comparison.png" alt="Trivial Comparison" width="300"/>
  <img src="assets/modest_comparison.png" alt="Modest Comparison" width="300"/>
</div>

### Overview

The following benchmarks compare the performance of `sync.RWMutex` and `fairmutex` under read and write contention with **Trivial** and **Modest** work per operation. All benchmarks were run with varying numbers of concurrent readers or writers (1–10) on an 8-core system.

Metrics shown:

- **Iterations** (higher = more stable result)
- **ns/op** = average nanoseconds per operation (lower is better)

---

#### Trivial Readers

| Readers | sync.RWMutex (ns/op) | sync.RWMutex (iters) | fairmutex (ns/op) | fairmutex (iters) |
|--------|----------------------|----------------------|-------------------|-------------------|
| 1      | 71,387               | 15,393               | 2,068,023         | 560               |
| 2      | 170,750              | 9,316                | 2,247,204         | 500               |
| 3      | 342,880              | 4,084                | 2,071,890         | 495               |
| 4      | 354,638              | 3,127                | 2,166,477         | 501               |
| 5      | 594,191              | 2,258                | 2,227,330         | 474               |
| 6      | 570,264              | 2,373                | 2,139,925         | 584               |
| 7      | 731,147              | 1,696                | 2,187,649         | 582               |
| 8      | 755,529              | 1,726                | 2,104,850         | 589               |
| 9      | 952,063              | 1,410                | 2,194,007         | 530               |
| 10     | 877,765              | 1,218                | 2,239,112         | 540               |

---

#### Modest Readers

| Readers | sync.RWMutex (ns/op) | sync.RWMutex (iters) | fairmutex (ns/op) | fairmutex (iters) |
|--------|----------------------|----------------------|-------------------|-------------------|
| 1      | 58,027               | 20,646               | 1,885,479         | 554               |
| 2      | 156,849              | 8,422                | 1,919,078         | 852               |
| 3      | 176,630              | 7,348                | 2,191,179         | 507               |
| 4      | 135,049              | 8,884                | 1,753,620         | 782               |
| 5      | 120,680              | 10,592               | 1,656,292         | 610               |
| 6      | 129,623              | 8,719                | 1,271,239         | 829               |
| 7      | 151,856              | 8,761                | 1,078,044         | 1,015             |
| 8      | 139,373              | 10,000               | 905,072           | 1,140             |
| 9      | 139,517              | 8,458                | 858,912           | 1,417             |
| 10     | 127,528              | 10,000               | 797,771           | 1,899             |

---

#### Trivial Writers

| Writers | sync.RWMutex (ns/op) | sync.RWMutex (iters) | fairmutex (ns/op) | fairmutex (iters) |
|---------|----------------------|----------------------|-------------------|-------------------|
| 1       | 14,791,490           | 120                  | 746,416           | 1,430             |
| 2       | 7,786,349            | 162                  | 751,823           | 1,560             |
| 3       | 6,775,873            | 156                  | 813,926           | 1,264             |
| 4       | 7,464,469            | 162                  | 876,813           | 1,484             |
| 5       | 12,431,794           | 129                  | 892,349           | 1,558             |
| 6       | 11,465,916           | 134                  | 936,943           | 1,335             |
| 7       | 10,068,969           | 144                  | 948,705           | 1,244             |
| 8       | 12,624,012           | 100                  | 990,205           | 1,386             |
| 9       | 13,319,460           | 100                  | 965,553           | 1,089             |
| 10      | 14,599,778           | 100                  | 1,024,979         | 1,020             |

---

#### Modest Writers

| Writers | sync.RWMutex (ns/op) | sync.RWMutex (iters) | fairmutex (ns/op) | fairmutex (iters) |
|---------|----------------------|----------------------|-------------------|-------------------|
| 1       | 10,924,570           | 601                  | 770,237           | 1,796             |
| 2       | 8,361,460            | 214                  | 765,214           | 1,622             |
| 3       | 6,664,625            | 164                  | 840,543           | 1,368             |
| 4       | 9,111,152            | 177                  | 820,902           | 1,494             |
| 5       | 7,132,459            | 141                  | 893,106           | 1,435             |
| 6       | 11,637,746           | 100                  | 890,071           | 1,444             |
| 7       | 11,830,056           | 100                  | 902,716           | 1,521             |
| 8       | 11,588,830           | 100                  | 938,670           | 1,138             |
| 9       | 14,063,770           | 96                   | 960,568           | 1,530             |
| 10      | 15,817,532           | 100                  | 902,018           | 1,200             |

---

## Key Observations

### Readers

- **`sync.RWMutex` dominates** in both Trivial and Modest read-heavy workloads.
- `fairmutex` is **~20–30x slower** under read contention due to fairness overhead.
- As reader count increases, `sync.RWMutex` scales better; `fairmutex` remains flat but high-latency.

### Writers

- **`fairmutex` wins decisively** in write-heavy scenarios — **10–20x faster** than `sync.RWMutex`.
- `sync.RWMutex` suffers from writer starvation and lock contention; performance degrades sharply.
- `fairmutex` maintains **consistent ~750k–1M ns/op** even at high writer counts.

### Trade-off Summary

| Scenario                  | Winner          | Reason |
|---------------------------|-----------------|--------|
| Read-heavy (Trivial/Modest) | `sync.RWMutex`  | Low overhead, excellent scalability |
| Write-heavy (Trivial/Modest)| `fairmutex`     | Prevents starvation, consistent latency |
| Mixed or fairness-critical | `fairmutex`     | Guarantees progress for all threads |

---

**Conclusion**:

Use **`sync.RWMutex`** for **read-dominant** workloads.

Use **`fairmutex`** for **write-heavy or fairness-sensitive** applications where preventing writer starvation is critical.
