package main

import (
	"golang.org/x/tools/go/analysis/singlechecker"

	"github.com/fastbean-au/fair-mutex/fairmutexcheck"
)

func main() {
	singlechecker.Main(fairmutexcheck.Analyzer)
}
