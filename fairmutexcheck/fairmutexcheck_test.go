package fairmutexcheck_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/fastbean-au/fair-mutex/fairmutexcheck"
)

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, fairmutexcheck.Analyzer, "a")
}
