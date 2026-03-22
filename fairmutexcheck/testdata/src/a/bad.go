package a

import fairmutex "github.com/fastbean-au/fair-mutex"

func Bad1() {
	mtx := fairmutex.RWMutex{} // want "direct instantiation of fairmutex.RWMutex"
	mtx.Lock()
	mtx.Unlock()
}

func Bad2() {
	mtx := new(fairmutex.RWMutex) // want "use of new\\(fairmutex.RWMutex\\)"
	mtx.Lock()
	mtx.Unlock()
}

func Bad3() {
	mtx := fairmutex.New() // want "fairmutex.New\\(\\) called but Stop\\(\\) is never called"
	mtx.Lock()
	mtx.Unlock()
	// Missing Stop() call
}

func Bad4() *fairmutex.RWMutex {
	mtx := fairmutex.New() // want "fairmutex.New\\(\\) called but Stop\\(\\) is never called"
	return mtx
	// Stop() should be called by the caller, but we can't verify that
}

func Bad5() {
	mtx1 := fairmutex.New()
	defer mtx1.Stop() // Correct: Stop() is called

	mtx2 := fairmutex.New() // want "fairmutex.New\\(\\) called but Stop\\(\\) is never called"
	// Missing Stop() call

	mtx1.Lock()
	mtx2.Lock()
	mtx1.Unlock()
	mtx2.Unlock()
}

func Bad6() {
	mtx := &fairmutex.RWMutex{} // want "direct instantiation of fairmutex.RWMutex"
	mtx.Lock()
	mtx.Unlock()
}

func Bad7() {
	mtx := new(*fairmutex.RWMutex) // want "use of new\\(fairmutex.RWMutex\\)"
	_ = mtx
}

func Bad8() {
	m1 := fairmutex.New()
	m2 := fairmutex.New() // want "fairmutex.New\\(\\) called but Stop\\(\\) is never called"
	go func() {
		m1.Stop()
		m1.Lock()
		m1.Unlock()
		m2.Lock() // m2 is captured by this closure but Stop() is never called on it
		m2.Unlock()
	}()
}