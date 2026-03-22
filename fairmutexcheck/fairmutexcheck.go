// Package fairmutexcheck defines an analyzer that checks proper usage of fair-mutex
package fairmutexcheck

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

const pkgPath = "github.com/fastbean-au/fair-mutex"

// Analyzer checks for proper fair-mutex usage
var Analyzer = &analysis.Analyzer{
	Name:     "fairmutexcheck",
	Doc:      "checks that fair-mutex variables are created with New() and Stop() is called",
	Run:      run,
	Requires: []*analysis.Analyzer{buildssa.Analyzer},
}

func run(pass *analysis.Pass) (interface{}, error) {
	ssaInfo := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)

	// Check for direct instantiation (struct literals and new())
	checkDirectInstantiation(pass)

	// Check for New() calls without Stop()
	for _, fn := range ssaInfo.SrcFuncs {
		checkMissingStop(pass, fn)
	}

	return nil, nil
}

// checkDirectInstantiation looks for direct struct initialization
func checkDirectInstantiation(pass *analysis.Pass) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {

			case *ast.CompositeLit:
				// Check for fairmutex.RWMutex{}
				if isRWMutexType(pass.TypesInfo, node.Type) {
					pass.Reportf(node.Pos(),
						"direct instantiation of fairmutex.RWMutex; use fairmutex.New() instead")
				}

			case *ast.CallExpr:
				// Check for new(fairmutex.RWMutex)
				if isBuiltinNew(pass.TypesInfo, node.Fun) {
					if len(node.Args) == 1 {
						if isRWMutexType(pass.TypesInfo, node.Args[0]) {
							pass.Reportf(node.Pos(),
								"use of new(fairmutex.RWMutex); use fairmutex.New() instead")
						}
					}
				}
				
			}

			return true
		})
	}
}

// checkMissingStop analyzes SSA to find New() calls without corresponding Stop()
func checkMissingStop(pass *analysis.Pass, fn *ssa.Function) {
	if fn == nil || fn.Blocks == nil {
		return
	}

	// Track variables created with New()
	newVars := make(map[ssa.Value]bool)
	stoppedVars := make(map[ssa.Value]bool)

	// Find all New() calls and Stop() calls within this function's own blocks
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			if call, ok := instr.(ssa.CallInstruction); ok {
				if isNewCall(call) {
					if v := call.Value(); v != nil {
						newVars[v] = true
					}
					continue
				}

				if isStopCall(call) {
					if recv := getReceiver(call); recv != nil {
						for v := range newVars {
							if mayAlias(v, recv) {
								stoppedVars[v] = true
							}
						}
					}
				}
			}
		}
	}

	// For any newVar not yet stopped, check if it escapes to a closure that calls Stop()
	for v := range newVars {
		if !stoppedVars[v] && closureHasStop(fn, v) {
			stoppedVars[v] = true
		}
	}

	// Report variables created with New() but never stopped
	for v := range newVars {
		if !stoppedVars[v] {
			if pos := v.Pos(); pos.IsValid() {
				pass.Reportf(pos,
					"fairmutex.New() called but Stop() is never called; resources will leak")
			}
		}
	}
}

// closureHasStop checks if newVar is stored into an alloc captured by a closure that calls Stop()
func closureHasStop(fn *ssa.Function, newVar ssa.Value) bool {
	// Find alloc cells where newVar is stored (closures capture variables via allocs)
	var allocs []ssa.Value
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			if store, ok := instr.(*ssa.Store); ok {
				if store.Val == newVar {
					allocs = append(allocs, store.Addr)
				}
			}
		}
	}
	if len(allocs) == 0 {
		return false
	}

	// Find MakeClosure instructions that capture those allocs
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			mc, ok := instr.(*ssa.MakeClosure)
			if !ok {
				continue
			}
			closureFn, ok := mc.Fn.(*ssa.Function)
			if !ok {
				continue
			}
			for bindIdx, binding := range mc.Bindings {
				for _, alloc := range allocs {
					if binding == alloc && bindIdx < len(closureFn.FreeVars) {
						if closureFnCallsStop(closureFn, closureFn.FreeVars[bindIdx]) {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

// closureFnCallsStop checks if the closure function calls Stop() on the given captured variable
func closureFnCallsStop(fn *ssa.Function, capturedVar *ssa.FreeVar) bool {
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			if call, ok := instr.(ssa.CallInstruction); ok {
				if isStopCall(call) {
					if recv := getReceiver(call); recv != nil {
						if stripInstructions(recv) == capturedVar {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

// isNewCall checks if a call instruction is to fairmutex.New()
func isNewCall(call ssa.CallInstruction) bool {
	common := call.Common()
	if common == nil {
		return false
	}

	if fn, ok := common.Value.(*ssa.Function); ok {
		if fn.Pkg != nil && fn.Pkg.Pkg != nil {
			return fn.Pkg.Pkg.Path() == pkgPath && fn.Name() == "New"
		}
	}

	return false
}

// isStopCall checks if a call instruction is to the Stop() method
func isStopCall(call ssa.CallInstruction) bool {
	common := call.Common()
	if common == nil {
		return false
	}

	if fn, ok := common.Value.(*ssa.Function); ok {
		if fn.Pkg != nil && fn.Pkg.Pkg != nil {
			return fn.Pkg.Pkg.Path() == pkgPath && fn.Name() == "Stop"
		}
	}

	return false
}

// getReceiver extracts the receiver from a method call
func getReceiver(call ssa.CallInstruction) ssa.Value {
	common := call.Common()
	if common != nil && common.IsInvoke() {
		return common.Value
	}

	if common != nil && len(common.Args) > 0 {
		return common.Args[0]
	}

	return nil
}

// mayAlias checks if two SSA values might refer to the same object
func mayAlias(v1, v2 ssa.Value) bool {
	// Simple alias check - could be enhanced with full alias analysis
	if v1 == v2 {
		return true
	}

	// Check through extracts, field addresses, etc.
	v1 = stripInstructions(v1)
	v2 = stripInstructions(v2)

	return v1 == v2
}

// stripInstructions removes wrapping instructions to get to the base value
func stripInstructions(v ssa.Value) ssa.Value {
	for {
		switch instr := v.(type) {
		case *ssa.UnOp:
			v = instr.X
		case *ssa.FieldAddr:
			v = instr.X
		case *ssa.IndexAddr:
			v = instr.X
		case *ssa.Extract:
			v = instr.Tuple
		default:
			return v
		}
	}
}

// isRWMutexType checks if the type is fairmutex.RWMutex
func isRWMutexType(info *types.Info, expr ast.Expr) bool {
	t := info.TypeOf(expr)
	if t == nil {
		return false
	}

	// Handle pointer types
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}

	if named, ok := t.(*types.Named); ok {
		if obj := named.Obj(); obj != nil && obj.Pkg() != nil {
			return obj.Pkg().Path() == pkgPath && obj.Name() == "RWMutex"
		}
	}

	return false
}

// isBuiltinNew checks if the expression is the built-in new() function
func isBuiltinNew(info *types.Info, expr ast.Expr) bool {
	if ident, ok := expr.(*ast.Ident); ok {
		if obj := info.Uses[ident]; obj != nil {
			if builtin, ok := obj.(*types.Builtin); ok {
				return builtin.Name() == "new"
			}
		}
	}

	return false
}
