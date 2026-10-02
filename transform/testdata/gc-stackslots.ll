target datalayout = "e-m:e-p:32:32-i64:64-n32:64-S128"
target triple = "wasm32-unknown-unknown-wasm"

@runtime.stackChainStart = external global ptr
@someGlobal = global i8 3
@ptrGlobal = global ptr null
@arrGlobal = global [8 x i8] zeroinitializer
@structGlobal = global {ptr, i32, [2 x ptr]} zeroinitializer
@ptrArrayGlobal = global [8 x ptr] zeroinitializer
@constantPtrGlobal = constant ptr @someGlobal

declare void @runtime.trackPointer(ptr nocapture readonly)

declare noalias nonnull ptr @runtime.alloc(i32, ptr)

declare i32 @runtime.gcGlobalRootCount()

declare ptr @runtime.gcGlobalRoot(i32)

declare i32 @runtime.gcGlobalRootSize(i32)

; Generic function that returns a pointer (that must be tracked).
define ptr @getPointer() {
    ret ptr @someGlobal
}

define ptr @needsStackSlots() {
  ; Tracked pointer. Although, in this case the value is immediately returned
  ; so tracking it is not really necessary.
  %ptr = call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  call void @runtime.trackPointer(ptr %ptr)
  call void @someArbitraryFunction()
  %val = load i8, ptr @someGlobal
  ret ptr %ptr
}

; Check some edge cases of pointer tracking.
define ptr @needsStackSlots2() {
  %ptr1 = call ptr @getPointer()
  call void @runtime.trackPointer(ptr %ptr1)
  call void @runtime.trackPointer(ptr %ptr1)
  call void @runtime.trackPointer(ptr %ptr1)

  ; Create a pointer that does not need to be tracked (but is tracked).
  %ptr2 = getelementptr i8, ptr @someGlobal, i32 0
  call void @runtime.trackPointer(ptr %ptr2)

  ; Here is finally the point where an allocation happens.
  %unused = call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  call void @runtime.trackPointer(ptr %unused)

  ret ptr %ptr1
}

; Return a pointer from a caller. Because it doesn't allocate, no stack objects
; need to be created.
define ptr @noAllocatingFunction() {
  %ptr = call ptr @getPointer()
  call void @runtime.trackPointer(ptr %ptr)
  ret ptr %ptr
}

define ptr @fibNext(ptr %x, ptr %y) {
  %x.val = load i8, ptr %x
  %y.val = load i8, ptr %y
  %out.val = add i8 %x.val, %y.val
  %out.alloc = call ptr @runtime.alloc(i32 1, ptr inttoptr (i32 3 to ptr))
  call void @runtime.trackPointer(ptr %out.alloc)
  store i8 %out.val, ptr %out.alloc
  ret ptr %out.alloc
}

define ptr @allocLoop() {
entry:
  %entry.x = call ptr @runtime.alloc(i32 1, ptr inttoptr (i32 3 to ptr))
  call void @runtime.trackPointer(ptr %entry.x)
  %entry.y = call ptr @runtime.alloc(i32 1, ptr inttoptr (i32 3 to ptr))
  call void @runtime.trackPointer(ptr %entry.y)
  store i8 1, ptr %entry.y
  br label %loop

loop:
  %prev.y = phi ptr [ %entry.y, %entry ], [ %prev.x, %loop ]
  %prev.x = phi ptr [ %entry.x, %entry ], [ %next.x, %loop ]
  call void @runtime.trackPointer(ptr %prev.x)
  call void @runtime.trackPointer(ptr %prev.y)
  %next.x = call ptr @fibNext(ptr %prev.x, ptr %prev.y)
  call void @runtime.trackPointer(ptr %next.x)
  %next.x.val = load i8, ptr %next.x
  %loop.done = icmp ult i8 40, %next.x.val
  br i1 %loop.done, label %end, label %loop

end:
  ret ptr %next.x
}

; Unlike @allocLoop above, the loop header phi here carries the only marker:
; neither incoming value is tracked on its own. This is what SimplifyCFG leaves
; behind when it sinks two runtime.trackPointer calls into a common successor
; and merges them. A slot for the phi alone roots whichever value the current
; iteration selected, so %loop.entry and %loop.next each need one too.
define ptr @loopPhiUntrackedInput(i1 %repeat) {
entry:
  %loop.entry = call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  br label %loop

loop:
  %loop.cur = phi ptr [ %loop.entry, %entry ], [ %loop.next, %loop ]
  call void @runtime.trackPointer(ptr %loop.cur)
  %loop.next = call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  br i1 %repeat, label %loop, label %end

end:
  ret ptr %loop.cur
}

; Nested loops merge the markers through one phi per loop, so the input of the
; inner phi is the outer phi rather than a plain value. Expanding only the
; inner one would leave %original unrooted once the outer loop repeats.
define ptr @nestedLoopPhiUntrackedInput(i1 %repeat.inner, i1 %repeat.outer) {
entry:
  %original = call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  br label %outer

outer:
  %outer.ptr = phi ptr [ %original, %entry ], [ %inner.ptr, %latch ]
  br label %inner

inner:
  %inner.ptr = phi ptr [ %outer.ptr, %outer ], [ %next, %inner ]
  call void @runtime.trackPointer(ptr %inner.ptr)
  %next = call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  br i1 %repeat.inner, label %inner, label %latch

latch:
  br i1 %repeat.outer, label %outer, label %end

end:
  ret ptr %original
}

define ptr @duplicateAcrossBranches(i1 %condition) {
entry:
  %original = call ptr @getPointer()
  br i1 %condition, label %left, label %right

left:
  call void @runtime.trackPointer(ptr %original)
  br label %join

right:
  call void @runtime.trackPointer(ptr %original)
  br label %join

join:
  call void @runtime.trackPointer(ptr %original)
  %unused = call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  ret ptr %original
}

define ptr @duplicateNestedLoopPhis(i1 %repeat.inner, i1 %repeat.outer) {
entry:
  %original = call ptr @getPointer()
  call void @runtime.trackPointer(ptr %original)
  call void @runtime.trackPointer(ptr %original)
  br label %outer

outer:
  %outer.ptr = phi ptr [ %original, %entry ], [ %inner.ptr, %latch ]
  call void @runtime.trackPointer(ptr %outer.ptr)
  call void @runtime.trackPointer(ptr %outer.ptr)
  br label %inner

inner:
  %inner.ptr = phi ptr [ %outer.ptr, %outer ], [ %next, %inner ]
  call void @runtime.trackPointer(ptr %inner.ptr)
  call void @runtime.trackPointer(ptr %inner.ptr)
  %next = call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  call void @runtime.trackPointer(ptr %next)
  call void @runtime.trackPointer(ptr %next)
  br i1 %repeat.inner, label %inner, label %latch

latch:
  br i1 %repeat.outer, label %outer, label %end

end:
  ret ptr %inner.ptr
}

define ptr @duplicateAcyclicPhi(i1 %condition) {
entry:
  br i1 %condition, label %left, label %right

left:
  %left.ptr = call ptr @getPointer()
  br label %join

right:
  %right.ptr = call ptr @getPointer()
  br label %join

join:
  %merged = phi ptr [ %left.ptr, %left ], [ %right.ptr, %right ]
  call void @runtime.trackPointer(ptr %merged)
  call void @runtime.trackPointer(ptr %merged)
  %unused = call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  ret ptr %merged
}

declare ptr @arrayAlloc()

define void @testGEPBitcast() {
  %arr = call ptr @arrayAlloc()
  %arr.bitcast = getelementptr [32 x i8], ptr %arr, i32 0, i32 0
  call void @runtime.trackPointer(ptr %arr.bitcast)
  %other = call ptr @runtime.alloc(i32 1, ptr inttoptr (i32 3 to ptr))
  call void @runtime.trackPointer(ptr %other)
  ret void
}

define void @someArbitraryFunction() {
  ret void
}

define void @earlyPopRegression() {
  %x.alloc = call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  call void @runtime.trackPointer(ptr %x.alloc)
  ; At this point the pass used to pop the stack chain, resulting in a potential use-after-free during allocAndSave.
  musttail call void @allocAndSave(ptr %x.alloc)
  ret void
}

define void @allocAndSave(ptr %x) {
  %y = call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  call void @runtime.trackPointer(ptr %y)
  store ptr %y, ptr %x
  store ptr %x, ptr @ptrGlobal
  ret void
}

declare void @usePointer(ptr)

define void @deadBeforeCollection() {
  %ptr = call ptr @getPointer()
  call void @runtime.trackPointer(ptr %ptr)
  call void @usePointer(ptr %ptr)
  call void @someArbitraryFunction()
  ret void
}

define void @liveInteriorPointer() {
  %ptr = call ptr @getPointer()
  call void @runtime.trackPointer(ptr %ptr)
  %field = getelementptr i8, ptr %ptr, i32 1
  call void @someArbitraryFunction()
  call void @usePointer(ptr %field)
  call void @someArbitraryFunction()
  ret void
}

define void @deadOnOneBranch(i1 %condition) {
entry:
  %ptr = call ptr @getPointer()
  call void @runtime.trackPointer(ptr %ptr)
  br i1 %condition, label %live, label %dead

live:
  call void @someArbitraryFunction()
  call void @usePointer(ptr %ptr)
  br label %end

dead:
  call void @someArbitraryFunction()
  br label %end

end:
  call void @someArbitraryFunction()
  ret void
}

define void @liveAcrossLoop(i1 %repeat) {
entry:
  %ptr = call ptr @getPointer()
  call void @runtime.trackPointer(ptr %ptr)
  br label %loop

loop:
  call void @someArbitraryFunction()
  call void @usePointer(ptr %ptr)
  br i1 %repeat, label %loop, label %end

end:
  call void @someArbitraryFunction()
  ret void
}

declare {ptr, i32} @getAggregate()
declare void @useAggregate({ptr, i32})

define void @liveAggregate() {
  %value = call {ptr, i32} @getAggregate()
  %ptr = extractvalue {ptr, i32} %value, 0
  call void @runtime.trackPointer(ptr %ptr)
  call void @someArbitraryFunction()
  call void @useAggregate({ptr, i32} %value)
  call void @someArbitraryFunction()
  ret void
}

define void @deadStackAllocation() {
  %storage = alloca ptr
  %ptr = call ptr @getPointer()
  call void @runtime.trackPointer(ptr %ptr)
  store ptr %ptr, ptr %storage
  call void @someArbitraryFunction()
  %loaded = load ptr, ptr %storage
  call void @runtime.trackPointer(ptr %loaded)
  call void @usePointer(ptr %loaded)
  call void @someArbitraryFunction()
  ret void
}

define void @escapedStackAllocation() {
  %storage = alloca ptr
  %ptr = call ptr @getPointer()
  call void @runtime.trackPointer(ptr %ptr)
  store ptr %ptr, ptr %storage
  store ptr %storage, ptr @ptrGlobal
  call void @someArbitraryFunction()
  ret void
}

define void @deadByteArrayStackAllocation() {
  %storage = alloca [32 x i8], align 4
  %ptr = call ptr @getPointer()
  call void @runtime.trackPointer(ptr %ptr)
  store ptr %ptr, ptr %storage
  call void @someArbitraryFunction()
  %loaded = load ptr, ptr %storage
  call void @runtime.trackPointer(ptr %loaded)
  call void @usePointer(ptr %loaded)
  call void @someArbitraryFunction()
  ret void
}

define void @deadPointerFreeStackAllocation() {
  %storage = alloca [32 x i8], align 4, !tinygo.gc.pointerfree !0
  store i32 42, ptr %storage
  call void @readPointerStorage(ptr %storage)
  call void @someArbitraryFunction()
  ret void
}

define void @promotedPointerFreeStackAllocation() {
  %storage = call align 4 ptr @runtime.alloc(i32 32, ptr inttoptr (i32 3 to ptr))
  store i32 42, ptr %storage
  call void @readPointerStorage(ptr %storage)
  call void @someArbitraryFunction()
  ret void
}

define void @promotedPointerStackAllocation() {
  %storage = call align 4 ptr @runtime.alloc(i32 32, ptr inttoptr (i32 67 to ptr))
  %ptr = call ptr @getPointer()
  store ptr %ptr, ptr %storage
  call void @readPointerStorage(ptr %storage)
  call void @someArbitraryFunction()
  ret void
}

define void @mergedPointerFreeStackAllocation([8 x i32] %bytes) {
  %source = alloca [32 x i8], align 4, !tinygo.gc.pointerfree !0
  %destination = alloca [32 x i8], align 4
  store [8 x i32] %bytes, ptr %source
  call void @readPointerStorage(ptr %source)
  call void @llvm.memcpy.p0.p0.i32(ptr align 4 %destination, ptr align 4 %source, i32 32, i1 false)
  %ptr = call ptr @getPointer()
  store ptr %ptr, ptr %destination
  call void @readPointerStorage(ptr %destination)
  call void @someArbitraryFunction()
  ret void
}

declare void @llvm.memcpy.p0.p0.i32(ptr, ptr, i32, i1)

declare void @llvm.lifetime.start.p0(ptr captures(none))
declare void @llvm.lifetime.end.p0(ptr captures(none))
declare void @readPointerStorage(ptr captures(none) readonly)

!0 = !{}

define void @deadStackLifetime(i1 %condition) {
entry:
  %storage = alloca ptr
  br i1 %condition, label %active, label %end

active:
  call void @llvm.lifetime.start.p0(ptr %storage)
  %ptr = call ptr @getPointer()
  call void @runtime.trackPointer(ptr %ptr)
  store ptr %ptr, ptr %storage
  call void @readPointerStorage(ptr %storage)
  call void @someArbitraryFunction()
  call void @llvm.lifetime.end.p0(ptr %storage)
  br label %end

end:
  call void @someArbitraryFunction()
  ret void
}

define void @restartedStackLifetime() {
  %storage = alloca ptr
  call void @llvm.lifetime.start.p0(ptr %storage)
  %first = call ptr @getPointer()
  call void @runtime.trackPointer(ptr %first)
  store ptr %first, ptr %storage
  call void @readPointerStorage(ptr %storage)
  call void @llvm.lifetime.end.p0(ptr %storage)
  call void @someArbitraryFunction()
  call void @llvm.lifetime.start.p0(ptr %storage)
  %second = call ptr @getPointer()
  call void @runtime.trackPointer(ptr %second)
  store ptr %second, ptr %storage
  call void @readPointerStorage(ptr %storage)
  call void @llvm.lifetime.end.p0(ptr %storage)
  call void @someArbitraryFunction()
  call void @llvm.lifetime.start.p0(ptr %storage)
  call void @llvm.lifetime.end.p0(ptr %storage)
  call void @someArbitraryFunction()
  ret void
}

declare void @"(internal/task).Pause"()

define ptr @getAndPause() {
	%ptr = call ptr @getPointer()
	call void @runtime.trackPointer(ptr %ptr)
	; Calling a function with unknown memory access forces stack slot creation.
	call void @"(internal/task).Pause"()
	ret ptr %ptr
}

; Function Attrs: memory(readwrite)
declare void @externCallWithMemAttr() #0

define ptr @getAndCallWithMemAttr() {
	%ptr = call ptr @getPointer()
	call void @runtime.trackPointer(ptr %ptr)
	; Calling an external function which may access non-arg memory forces stack slot creation.
	call void @externCallWithMemAttr()
	ret ptr %ptr
}

; Generic function that returns a slice (that must be tracked).
define {ptr, i32, i32} @getSlice() {
  ret {ptr, i32, i32} {ptr @someGlobal, i32 8, i32 8}
}

define i32 @copyToSlice(ptr %src.ptr, i32 %src.len, i32 %src.cap) {
  %dst = call {ptr, i32, i32} @getSlice()
  %dst.ptr = extractvalue {ptr, i32, i32} %dst, 0
  call void @runtime.trackPointer(ptr %dst.ptr)
  %dst.len = extractvalue {ptr, i32, i32} %dst, 1
  ; Math intrinsics do not need stack slots.
  %minLen = call i32 @llvm.umin.i32(i32 %dst.len, i32 %src.len)
  ; Intrinsics which only access argument memory do not need stack slots.
  call void @llvm.memmove.p0.p0.i32(ptr %dst.ptr, ptr %src.ptr, i32 %minLen, i1 false)
  ret i32 %minLen
}

; Function Attrs: nocallback nofree nosync nounwind speculatable willreturn memory(none)
declare i32 @llvm.umin.i32(i32, i32) #1

; Function Attrs: nocallback nofree nounwind willreturn memory(argmem: readwrite)
declare void @llvm.memmove.p0.p0.i32(ptr nocapture writeonly, ptr nocapture readonly, i32, i1 immarg) #2

attributes #0 = { memory(readwrite) }
attributes #1 = { nocallback nofree nosync nounwind speculatable willreturn memory(none) }
attributes #2 = { nocallback nofree nounwind willreturn memory(argmem: readwrite) }
