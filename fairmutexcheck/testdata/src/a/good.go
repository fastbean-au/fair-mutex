package a

import fairmutex "github.com/fastbean-au/fair-mutex"

func Good1() {
	mtx := fairmutex.New()
	defer mtx.Stop() // Correct: Stop() is called

	mtx.Lock()
	mtx.Unlock()
}

func Good2() {
	mtx := fairmutex.New()

	mtx.Lock()
	mtx.Unlock()

	mtx.Stop() // Correct: Stop() is called
}

func Good3() {
	mtx := fairmutex.New()

	wait := make(chan interface{})

	go func() {
		<-wait
		mtx.Stop()
	}()

	mtx.Lock()
	mtx.Unlock()
	
	close(wait)
}

func Good4() {
	mtx1 := fairmutex.New()
	defer mtx1.Stop() // Correct: Stop() is called

	mtx2 := fairmutex.New()
	defer mtx2.Stop() // Correct: Stop() is called

	mtx1.Lock()
	mtx2.Lock()
	mtx1.Unlock()
	mtx2.Unlock()
}

func Good5() {
	mtx := fairmutex.New()

	go func() {
		defer mtx.Stop() // Correct: Stop() is deferred inside the goroutine
		mtx.Lock()
		mtx.Unlock()
	}()
}

func Good6() {
	_ = new(int)                 // new() with a non-RWMutex type: no diagnostic
	_ = []fairmutex.RWMutex{{}} // inner {} has no explicit type expression: no diagnostic
}

