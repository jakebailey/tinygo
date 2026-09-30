package compiler

import (
	"go/token"
	"go/types"

	"tinygo.org/x/go-llvm"
)

func (c *compilerContext) finalizerCallSignature() *types.Signature {
	return types.NewSignatureType(nil, nil, nil, types.NewTuple(
		types.NewVar(token.NoPos, nil, "obj", types.NewInterfaceType(nil, nil).Complete()),
		types.NewVar(token.NoPos, nil, "fn", types.Typ[types.UnsafePointer]),
	), nil, false)
}

func (c *compilerContext) getFinalizerCall(sig *types.Signature, name string, local bool) (llvm.Value, llvm.Value) {
	callType := c.getFuncType(c.finalizerCallSignature())
	if sig.Variadic() || sig.Params().Len() != 1 {
		return llvm.ConstNull(c.dataPtrType), llvm.ConstNull(callType)
	}
	argType := sig.Params().At(0).Type()
	switch argType.Underlying().(type) {
	case *types.Pointer, *types.Interface:
	default:
		return llvm.ConstNull(c.dataPtrType), llvm.ConstNull(callType)
	}
	wrapper := llvm.AddFunction(c.mod, name+"$finalizer", c.getLLVMFunctionType(c.finalizerCallSignature()))
	c.addStandardAttributes(wrapper)
	if local {
		wrapper.SetLinkage(llvm.InternalLinkage)
	} else {
		wrapper.SetLinkage(llvm.LinkOnceODRLinkage)
	}
	b := builder{compilerContext: c, Builder: c.ctx.NewBuilder(), llvmFn: wrapper}
	defer b.Dispose()
	b.SetInsertPointAtEnd(c.ctx.AddBasicBlock(wrapper, "entry"))
	fn := b.CreateLoad(c.getFuncType(sig), wrapper.Param(2), "fn")
	callee, context := b.decodeFuncValue(fn)
	arg := wrapper.Param(1)
	if _, ok := argType.Underlying().(*types.Interface); ok {
		arg = llvm.Undef(c.getLLVMRuntimeType("_interface"))
		arg = b.CreateInsertValue(arg, wrapper.Param(0), 0, "")
		arg = b.CreateInsertValue(arg, wrapper.Param(1), 1, "")
	}
	var args []llvm.Value
	abi := c.getFunctionABI(sig, false)
	if abi.indirectResult {
		args = append(args, b.CreateAlloca(abi.resultType, "result"))
	}
	args = append(args, arg, context)
	b.createCall(c.getLLVMFunctionType(sig), callee, args, "")
	b.CreateRetVoid()
	return c.getTypeCode(argType), c.ctx.ConstStruct([]llvm.Value{
		llvm.ConstNull(c.dataPtrType), wrapper,
	}, false)
}
