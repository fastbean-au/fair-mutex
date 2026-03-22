package fairmutexcheck

import (
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestStripInstructions(t *testing.T) {
	base := new(ssa.Alloc) // a value with no strippable wrapper — hits the default case

	t.Run("default/base value returned unchanged", func(t *testing.T) {
		if got := stripInstructions(base); got != base {
			t.Errorf("got %v, want base", got)
		}
	})

	t.Run("UnOp stripped", func(t *testing.T) {
		v := &ssa.UnOp{X: base}
		if got := stripInstructions(v); got != base {
			t.Errorf("got %v, want base", got)
		}
	})

	t.Run("FieldAddr stripped", func(t *testing.T) {
		v := &ssa.FieldAddr{X: base}
		if got := stripInstructions(v); got != base {
			t.Errorf("got %v, want base", got)
		}
	})

	t.Run("IndexAddr stripped", func(t *testing.T) {
		v := &ssa.IndexAddr{X: base}
		if got := stripInstructions(v); got != base {
			t.Errorf("got %v, want base", got)
		}
	})

	t.Run("Extract stripped", func(t *testing.T) {
		v := &ssa.Extract{Tuple: base}
		if got := stripInstructions(v); got != base {
			t.Errorf("got %v, want base", got)
		}
	})

	t.Run("chained wrappers all stripped", func(t *testing.T) {
		// UnOp(FieldAddr(IndexAddr(base))) — all three new cases chained together
		v := &ssa.UnOp{X: &ssa.FieldAddr{X: &ssa.IndexAddr{X: base}}}
		if got := stripInstructions(v); got != base {
			t.Errorf("got %v, want base", got)
		}
	})
}
