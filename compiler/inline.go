package compiler

import (
	"go/ast"

	"golang.org/x/tools/go/ssa"
	"tinygo.org/x/go-llvm"
)

const (
	// These costs match Go's default budget and non-leaf call cost.
	// See https://go.dev/src/cmd/compile/internal/inline/inl.go.
	inlineMaxBudget     = 80
	inlineExtraCallCost = 57
)

type inlineDecision uint8

const (
	inlineUnknown inlineDecision = iota
	inlineAllowed
	inlineDisallowed
)

type inlineCost struct {
	cost     int
	decision inlineDecision
}

func (c *compilerContext) inlineCost(fn *ssa.Function) inlineCost {
	if cost, ok := c.inlineCosts[fn]; ok {
		return cost
	}
	if fn.Syntax() == nil || fn.Synthetic != "" {
		return inlineCost{decision: inlineUnknown}
	}
	if decl, ok := fn.Syntax().(*ast.FuncDecl); ok && decl.Doc != nil {
		for _, comment := range decl.Doc.List {
			if comment.Text == "//go:noinline" {
				result := inlineCost{decision: inlineDisallowed}
				c.inlineCosts[fn] = result
				return result
			}
		}
	}
	cost := 0
	for _, block := range fn.Blocks {
		for _, instruction := range block.Instrs {
			if _, ok := instruction.(*ssa.DebugRef); ok {
				continue
			}
			cost++
			switch instruction := instruction.(type) {
			case *ssa.Call:
				common := instruction.Common()
				if builtin, ok := common.Value.(*ssa.Builtin); ok {
					if builtin.Name() == "recover" {
						result := inlineCost{decision: inlineDisallowed}
						c.inlineCosts[fn] = result
						return result
					}
					break
				}
				// Calls are expensive because they can expose much larger call trees.
				cost += inlineExtraCallCost
			case *ssa.Defer, *ssa.Go, *ssa.RunDefers:
				result := inlineCost{decision: inlineDisallowed}
				c.inlineCosts[fn] = result
				return result
			}
			if cost > inlineMaxBudget {
				result := inlineCost{cost: cost, decision: inlineDisallowed}
				c.inlineCosts[fn] = result
				return result
			}
		}
	}
	result := inlineCost{cost: cost, decision: inlineAllowed}
	c.inlineCosts[fn] = result
	return result
}

func (c *compilerContext) hasInlineCycle(fn *ssa.Function) bool {
	if recursive, ok := c.inlineCycles[fn]; ok {
		return recursive
	}

	var visits func(*ssa.Function, map[*ssa.Function]bool) bool
	visits = func(current *ssa.Function, seen map[*ssa.Function]bool) bool {
		if seen[current] {
			return false
		}
		seen[current] = true
		for _, block := range current.Blocks {
			for _, instruction := range block.Instrs {
				call, ok := instruction.(*ssa.Call)
				if !ok {
					continue
				}
				callee := call.Common().StaticCallee()
				if callee == nil || c.inlineCost(callee).decision != inlineAllowed {
					continue
				}
				if callee == fn || visits(callee, seen) {
					return true
				}
			}
		}
		return false
	}

	recursive := visits(fn, map[*ssa.Function]bool{})
	c.inlineCycles[fn] = recursive
	return recursive
}

// hasInlineFastPath finds a wrapper with one call that returns immediately
// and another path that bypasses it. See https://go.dev/issue/48195.
func hasInlineFastPath(fn *ssa.Function) bool {
	if len(fn.Blocks) == 0 {
		return false
	}
	var callBlock *ssa.BasicBlock
	for _, block := range fn.Blocks {
		for _, instruction := range block.Instrs {
			call, ok := instruction.(*ssa.Call)
			if !ok {
				continue
			}
			if _, ok := call.Common().Value.(*ssa.Builtin); ok {
				continue
			}
			if callBlock != nil {
				return false
			}
			callBlock = block
		}
	}
	if callBlock == nil {
		return false
	}

	callReturns := false
	for _, instruction := range callBlock.Instrs {
		if _, ok := instruction.(*ssa.Return); ok {
			callReturns = true
		}
	}
	if !callReturns && len(callBlock.Succs) == 1 {
		for _, instruction := range callBlock.Succs[0].Instrs {
			if _, ok := instruction.(*ssa.Return); ok {
				callReturns = true
			}
		}
	}
	if !callReturns {
		return false
	}

	seen := make(map[*ssa.BasicBlock]bool)
	var reachesReturn func(*ssa.BasicBlock) bool
	reachesReturn = func(block *ssa.BasicBlock) bool {
		if block == callBlock || seen[block] {
			return false
		}
		seen[block] = true
		for _, instruction := range block.Instrs {
			if _, ok := instruction.(*ssa.Return); ok {
				return true
			}
		}
		for _, successor := range block.Succs {
			if reachesReturn(successor) {
				return true
			}
		}
		return false
	}
	return reachesReturn(fn.Blocks[0])
}

func (b *builder) shouldInlineCall(fn *ssa.Function) bool {
	if (b.SpeedLevel != 2 && b.SizeLevel == 0) ||
		b.getFunctionInfo(fn).inline != inlineDefault {
		return false
	}
	cost := b.inlineCost(fn)
	if cost.decision != inlineAllowed || b.hasInlineCycle(fn) {
		return false
	}
	return b.SizeLevel == 0 || hasInlineFastPath(fn)
}

func (b *builder) addInlineCallSiteAttribute(call llvm.Value, fn *ssa.Function) {
	if !b.shouldInlineCall(fn) {
		return
	}
	call.AddCallSiteAttribute(-1, b.ctx.CreateEnumAttribute(
		llvm.AttributeKindID("alwaysinline"), 0))
}
