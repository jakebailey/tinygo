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
declare void @collectingConsumer(ptr)

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
  ; Only one stack slot should be created for this (but at the moment, one is
  ; created for each call to runtime.trackPointer).
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

define void @liveDuringCall() {
entry:
  %ptr = call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  call void @runtime.trackPointer(ptr %ptr)
  call void @collectingConsumer(ptr %ptr)
  call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  ret void
}

define void @parameterUsedDuringCall(ptr %owner) {
entry:
  call void @collectingConsumer(ptr %owner)
  call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  ret void
}

define i8 @deadBeforeCollect() {
entry:
  %ptr = call ptr @getPointer()
  call void @runtime.trackPointer(ptr %ptr)
  %byte = load i8, ptr %ptr
  call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  ret i8 %byte
}

define i8 @aggregateAfterCollect() {
entry:
  %ptr = call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  call void @runtime.trackPointer(ptr %ptr)
  %aggregate = insertvalue {ptr, i32} poison, ptr %ptr, 0
  call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  %extracted = extractvalue {ptr, i32} %aggregate, 0
  %byte = load i8, ptr %extracted
  call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  ret i8 %byte
}

define void @parameterThroughAggregate(ptr %owner) {
entry:
  %aggregate = insertvalue {ptr, i32} poison, ptr %owner, 0
  call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  %extracted = extractvalue {ptr, i32} %aggregate, 0
  store i8 1, ptr %extracted
  ret void
}

define {ptr, i32} @returnedInAggregate() {
entry:
  %ptr = call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  call void @runtime.trackPointer(ptr %ptr)
  %result = insertvalue {ptr, i32} poison, ptr %ptr, 0
  ret {ptr, i32} %result
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

define void @argumentAcrossAlloc(ptr %owner) {
entry:
  %field = getelementptr i8, ptr %owner, i32 4
  %new = tail call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  store i8 1, ptr %field
  ret void
}

define void @argumentAcrossTwoAllocs(ptr %owner) {
entry:
  %field = getelementptr i8, ptr %owner, i32 4
  call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  store i8 1, ptr %field
  ret void
}

define void @argumentDeadBeforeAlloc(ptr %owner) {
entry:
  %field = getelementptr i8, ptr %owner, i32 4
  call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  store i8 1, ptr %field
  call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  ret void
}

define void @deadArgument(ptr %dead) {
entry:
  %byte = load i8, ptr %dead
  %new = call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  store i8 %byte, ptr %new
  ret void
}

define void @indirectCall(ptr %owner, ptr %fn) {
entry:
  %field = getelementptr i8, ptr %owner, i32 4
  tail call void %fn()
  %byte = load i8, ptr %field
  store i8 %byte, ptr @someGlobal
  ret void
}

define void @deadCallerArgument(ptr %dead) {
entry:
  tail call void @allocAndSave(ptr %dead)
  ret void
}

declare void @"internal/task.Pause"()

define ptr @liveRootAfterPause() {
entry:
  %ptr = call ptr @getPointer()
  call void @runtime.trackPointer(ptr %ptr)
  call void @"internal/task.Pause"()
  store i8 1, ptr %ptr
  ret ptr %ptr
}

define void @deadRootAtPause() {
entry:
  %ptr = call ptr @getPointer()
  call void @runtime.trackPointer(ptr %ptr)
  store i8 1, ptr %ptr
  call void @"internal/task.Pause"()
  ret void
}

define void @loopRootAtPause() {
entry:
  br label %loop
loop:
  %ptr = call ptr @getPointer()
  call void @runtime.trackPointer(ptr %ptr)
  call void @"internal/task.Pause"()
  %again = load i1, ptr @someGlobal
  br i1 %again, label %loop, label %end
end:
  ret void
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
