# Fair-Mutex


<p align="center">
  <img src="logo.png" title="fair-mutex />
</p>

**fair-mutex** is a pure-Go implementation of a fair RW mutex; that is, a mutex where write locks will not be prevented in a high volume read-lock use case. This is perhaps a fairly narrow use-case; if you don't need this then consider using [go-lock](https://github.com/viney-shih/go-lock) if the built-in `sync.RWMutex` does not meet your needs.

This implementation can be used as *functional* a drop-in replacement for Go's `sync.RWMutex` or `sync.Mutex` as at Go 1.25 (*Note:* the `New()` method must be called to initialise the mutex prior to use, and the context used to create the mutex must be cancelled in order to release the resources associated with the mutex).

The general principle on which **fair-mutex** operates is that locks are given in batches alternating between write locks and read locks. The batch size is determined at the beginning of a locking cycle based on the number of requests for locks. Read locks are given concurrently for the entire batch, white write locks are given sequentially for the entire batch. While batches are being processed, both type of lock requests are queued. Batch sizes are simply the lesser of the number of locks queued of the lock type at the beginning of a cycle or the maximum size limit set for that lock type.

An OpenTelemetry (OTEL) metric is provided to record the lock wait times, allowing an evaluation of the effective performance of the mutex, and identification of problematic lock contention issues.

**fair-mutex**  provides configurable read and write queue and batch size options, as well as an option for the metric name, and an option for default metric attributes.

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
