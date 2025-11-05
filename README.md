# Fair-Mutex

<div align="center">
  <img src="logo.png" alt="Fair-Mutex" width="200"/>
</div>

**fair-mutex** is a pure-Go implementation of a fair RW mutex; that is, a mutex where write locks will not be prevented in a high volume read-lock use case. This is perhaps a fairly narrow use-case; if you don't need this then consider using [go-lock](https://github.com/viney-shih/go-lock) if the built-in `sync.RWMutex` or `sync.Mutex` do not meet your needs.

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

Set to 1 if this mutex will only be used as a write-only mutex.

Defaults to 1024.

#### WithMaxWriteBatchSize
The maximum batch size for write (also known as exclusive) locks. The batch size does not determine the number of calls to obtain a lock that are waiting, but the maximum number that will be processed in one locking cycle.

This value cannot be larger than the MaxWriteQueueSize.

Defaults to 256.

#### WithMaxWriteQueueSize
The maximum queue size for write (also known as exclusive) locks. The queue size does not determine the number of calls to obtain a lock that are waiting, but the number during which we can guarantee order. This setting will effect the memory required.

Defaults to 32.

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
