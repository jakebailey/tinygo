target datalayout = "e-m:e-p:32:32-i64:64-n32:64-S128"
target triple = "wasm32-unknown-unknown-wasm"

@runtime.stackChainStart = internal global ptr null
@someGlobal = global i8 3
@ptrGlobal = global ptr null
@arrGlobal = global [8 x i8] zeroinitializer
@structGlobal = global { ptr, i32, [2 x ptr] } zeroinitializer
@ptrArrayGlobal = global [8 x ptr] zeroinitializer
@constantPtrGlobal = constant ptr @someGlobal
@runtime.gcGlobalRoots = internal constant [4 x { ptr, i32 }] [{ ptr, i32 } { ptr @ptrGlobal, i32 4 }, { ptr, i32 } { ptr @structGlobal, i32 4 }, { ptr, i32 } { ptr getelementptr (i8, ptr @structGlobal, i32 8), i32 8 }, { ptr, i32 } { ptr @ptrArrayGlobal, i32 32 }]

declare void @runtime.trackPointer(ptr nocapture readonly)

declare noalias nonnull ptr @runtime.alloc(i32, ptr)

define i32 @runtime.gcGlobalRootCount() {
entry:
  ret i32 4
}

define ptr @runtime.gcGlobalRoot(i32 %0) {
entry:
  %1 = getelementptr inbounds [4 x { ptr, i32 }], ptr @runtime.gcGlobalRoots, i32 0, i32 %0
  %2 = getelementptr inbounds nuw { ptr, i32 }, ptr %1, i32 0, i32 0
  %3 = load ptr, ptr %2, align 4
  ret ptr %3
}

define i32 @runtime.gcGlobalRootSize(i32 %0) {
entry:
  %1 = getelementptr inbounds [4 x { ptr, i32 }], ptr @runtime.gcGlobalRoots, i32 0, i32 %0
  %2 = getelementptr inbounds nuw { ptr, i32 }, ptr %1, i32 0, i32 1
  %3 = load i32, ptr %2, align 4
  ret i32 %3
}

define ptr @getPointer() {
  ret ptr @someGlobal
}

define ptr @needsStackSlots() {
  %gc.stackobject = alloca { ptr, i32, ptr }, align 8
  %1 = getelementptr { ptr, i32, ptr }, ptr %gc.stackobject, i32 0, i32 2
  store { ptr, i32, ptr } { ptr null, i32 1, ptr null }, ptr %gc.stackobject, align 4
  %2 = load ptr, ptr @runtime.stackChainStart, align 4
  %3 = getelementptr { ptr, i32, ptr }, ptr %gc.stackobject, i32 0, i32 0
  store ptr %2, ptr %3, align 4
  store ptr %gc.stackobject, ptr @runtime.stackChainStart, align 4
  %ptr = call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  store ptr %ptr, ptr %1, align 4
  call void @someArbitraryFunction()
  %val = load i8, ptr @someGlobal, align 1
  store volatile { ptr, i32, ptr } zeroinitializer, ptr %gc.stackobject, align 4
  store ptr %2, ptr @runtime.stackChainStart, align 4
  ret ptr %ptr
}

define ptr @needsStackSlots2() {
  %gc.stackobject = alloca { ptr, i32, ptr, ptr, ptr }, align 8
  %1 = getelementptr { ptr, i32, ptr, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 4
  %2 = getelementptr { ptr, i32, ptr, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 3
  %3 = getelementptr { ptr, i32, ptr, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 2
  store { ptr, i32, ptr, ptr, ptr } { ptr null, i32 3, ptr null, ptr null, ptr null }, ptr %gc.stackobject, align 4
  %4 = load ptr, ptr @runtime.stackChainStart, align 4
  %5 = getelementptr { ptr, i32, ptr, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 0
  store ptr %4, ptr %5, align 4
  store ptr %gc.stackobject, ptr @runtime.stackChainStart, align 4
  %ptr1 = call ptr @getPointer()
  store ptr %ptr1, ptr %3, align 4
  %ptr2 = getelementptr i8, ptr @someGlobal, i32 0
  store ptr %ptr2, ptr %2, align 4
  store ptr null, ptr %2, align 4
  %unused = call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  store ptr %unused, ptr %1, align 4
  store ptr null, ptr %1, align 4
  store volatile { ptr, i32, ptr, ptr, ptr } zeroinitializer, ptr %gc.stackobject, align 4
  store ptr %4, ptr @runtime.stackChainStart, align 4
  ret ptr %ptr1
}

define ptr @noAllocatingFunction() {
  %ptr = call ptr @getPointer()
  ret ptr %ptr
}

define ptr @fibNext(ptr %x, ptr %y) {
  %gc.stackobject = alloca { ptr, i32, ptr }, align 8
  %1 = getelementptr { ptr, i32, ptr }, ptr %gc.stackobject, i32 0, i32 2
  store { ptr, i32, ptr } { ptr null, i32 1, ptr null }, ptr %gc.stackobject, align 4
  %2 = load ptr, ptr @runtime.stackChainStart, align 4
  %3 = getelementptr { ptr, i32, ptr }, ptr %gc.stackobject, i32 0, i32 0
  store ptr %2, ptr %3, align 4
  store ptr %gc.stackobject, ptr @runtime.stackChainStart, align 4
  %x.val = load i8, ptr %x, align 1
  %y.val = load i8, ptr %y, align 1
  %out.val = add i8 %x.val, %y.val
  %out.alloc = call ptr @runtime.alloc(i32 1, ptr inttoptr (i32 3 to ptr))
  store ptr %out.alloc, ptr %1, align 4
  store i8 %out.val, ptr %out.alloc, align 1
  store volatile { ptr, i32, ptr } zeroinitializer, ptr %gc.stackobject, align 4
  store ptr %2, ptr @runtime.stackChainStart, align 4
  ret ptr %out.alloc
}

define ptr @allocLoop() {
entry:
  %gc.stackobject = alloca { ptr, i32, ptr, ptr, ptr, ptr, ptr }, align 8
  %0 = getelementptr { ptr, i32, ptr, ptr, ptr, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 6
  %1 = getelementptr { ptr, i32, ptr, ptr, ptr, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 5
  %2 = getelementptr { ptr, i32, ptr, ptr, ptr, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 4
  %3 = getelementptr { ptr, i32, ptr, ptr, ptr, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 3
  %4 = getelementptr { ptr, i32, ptr, ptr, ptr, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 2
  store { ptr, i32, ptr, ptr, ptr, ptr, ptr } { ptr null, i32 5, ptr null, ptr null, ptr null, ptr null, ptr null }, ptr %gc.stackobject, align 4
  %5 = load ptr, ptr @runtime.stackChainStart, align 4
  %6 = getelementptr { ptr, i32, ptr, ptr, ptr, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 0
  store ptr %5, ptr %6, align 4
  store ptr %gc.stackobject, ptr @runtime.stackChainStart, align 4
  %entry.x = call ptr @runtime.alloc(i32 1, ptr inttoptr (i32 3 to ptr))
  store ptr %entry.x, ptr %4, align 4
  %entry.y = call ptr @runtime.alloc(i32 1, ptr inttoptr (i32 3 to ptr))
  store ptr %entry.y, ptr %3, align 4
  store i8 1, ptr %entry.y, align 1
  br label %loop

loop:                                             ; preds = %loop, %entry
  %prev.y = phi ptr [ %entry.y, %entry ], [ %prev.x, %loop ]
  %prev.x = phi ptr [ %entry.x, %entry ], [ %next.x, %loop ]
  store ptr %prev.y, ptr %1, align 4
  store ptr %prev.x, ptr %2, align 4
  %next.x = call ptr @fibNext(ptr %prev.x, ptr %prev.y)
  store ptr null, ptr %1, align 4
  store ptr null, ptr %2, align 4
  store ptr %next.x, ptr %0, align 4
  %next.x.val = load i8, ptr %next.x, align 1
  %loop.done = icmp ult i8 40, %next.x.val
  br i1 %loop.done, label %end, label %loop

end:                                              ; preds = %loop
  store ptr null, ptr %3, align 4
  store ptr null, ptr %4, align 4
  store volatile { ptr, i32, ptr, ptr, ptr, ptr, ptr } zeroinitializer, ptr %gc.stackobject, align 4
  store ptr %5, ptr @runtime.stackChainStart, align 4
  ret ptr %next.x
}

define ptr @loopPhiUntrackedInput(i1 %repeat) {
entry:
  %gc.stackobject = alloca { ptr, i32, ptr, ptr, ptr }, align 8
  %0 = getelementptr { ptr, i32, ptr, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 4
  %1 = getelementptr { ptr, i32, ptr, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 3
  %2 = getelementptr { ptr, i32, ptr, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 2
  store { ptr, i32, ptr, ptr, ptr } { ptr null, i32 3, ptr null, ptr null, ptr null }, ptr %gc.stackobject, align 4
  %3 = load ptr, ptr @runtime.stackChainStart, align 4
  %4 = getelementptr { ptr, i32, ptr, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 0
  store ptr %3, ptr %4, align 4
  store ptr %gc.stackobject, ptr @runtime.stackChainStart, align 4
  %loop.entry = call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  store ptr %loop.entry, ptr %1, align 4
  br label %loop

loop:                                             ; preds = %loop, %entry
  %loop.cur = phi ptr [ %loop.entry, %entry ], [ %loop.next, %loop ]
  store ptr %loop.cur, ptr %2, align 4
  %loop.next = call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  store ptr %loop.next, ptr %0, align 4
  br i1 %repeat, label %loop, label %end

end:                                              ; preds = %loop
  store volatile { ptr, i32, ptr, ptr, ptr } zeroinitializer, ptr %gc.stackobject, align 4
  store ptr %3, ptr @runtime.stackChainStart, align 4
  ret ptr %loop.cur
}

define ptr @nestedLoopPhiUntrackedInput(i1 %repeat.inner, i1 %repeat.outer) {
entry:
  %gc.stackobject = alloca { ptr, i32, ptr, ptr, ptr, ptr }, align 8
  %0 = getelementptr { ptr, i32, ptr, ptr, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 5
  %1 = getelementptr { ptr, i32, ptr, ptr, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 4
  %2 = getelementptr { ptr, i32, ptr, ptr, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 3
  %3 = getelementptr { ptr, i32, ptr, ptr, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 2
  store { ptr, i32, ptr, ptr, ptr, ptr } { ptr null, i32 4, ptr null, ptr null, ptr null, ptr null }, ptr %gc.stackobject, align 4
  %4 = load ptr, ptr @runtime.stackChainStart, align 4
  %5 = getelementptr { ptr, i32, ptr, ptr, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 0
  store ptr %4, ptr %5, align 4
  store ptr %gc.stackobject, ptr @runtime.stackChainStart, align 4
  %original = call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  store ptr %original, ptr %0, align 4
  br label %outer

outer:                                            ; preds = %latch, %entry
  %outer.ptr = phi ptr [ %original, %entry ], [ %inner.ptr, %latch ]
  store ptr null, ptr %1, align 4
  store ptr null, ptr %3, align 4
  store ptr %outer.ptr, ptr %2, align 4
  br label %inner

inner:                                            ; preds = %inner, %outer
  %inner.ptr = phi ptr [ %outer.ptr, %outer ], [ %next, %inner ]
  store ptr %inner.ptr, ptr %3, align 4
  %next = call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  store ptr %next, ptr %1, align 4
  br i1 %repeat.inner, label %inner, label %latch

latch:                                            ; preds = %inner
  store ptr null, ptr %2, align 4
  br i1 %repeat.outer, label %outer, label %end

end:                                              ; preds = %latch
  store ptr null, ptr %1, align 4
  store ptr null, ptr %3, align 4
  store volatile { ptr, i32, ptr, ptr, ptr, ptr } zeroinitializer, ptr %gc.stackobject, align 4
  store ptr %4, ptr @runtime.stackChainStart, align 4
  ret ptr %original
}

define ptr @duplicateAcrossBranches(i1 %condition) {
entry:
  %gc.stackobject = alloca { ptr, i32, ptr }, align 8
  %0 = getelementptr { ptr, i32, ptr }, ptr %gc.stackobject, i32 0, i32 2
  store { ptr, i32, ptr } { ptr null, i32 1, ptr null }, ptr %gc.stackobject, align 4
  %1 = load ptr, ptr @runtime.stackChainStart, align 4
  %2 = getelementptr { ptr, i32, ptr }, ptr %gc.stackobject, i32 0, i32 0
  store ptr %1, ptr %2, align 4
  store ptr %gc.stackobject, ptr @runtime.stackChainStart, align 4
  %original = call ptr @getPointer()
  store ptr %original, ptr %0, align 4
  br i1 %condition, label %left, label %right

left:                                             ; preds = %entry
  br label %join

right:                                            ; preds = %entry
  br label %join

join:                                             ; preds = %right, %left
  %unused = call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  store volatile { ptr, i32, ptr } zeroinitializer, ptr %gc.stackobject, align 4
  store ptr %1, ptr @runtime.stackChainStart, align 4
  ret ptr %original
}

define ptr @duplicateNestedLoopPhis(i1 %repeat.inner, i1 %repeat.outer) {
entry:
  %gc.stackobject = alloca { ptr, i32, ptr, ptr, ptr, ptr }, align 8
  %0 = getelementptr { ptr, i32, ptr, ptr, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 5
  %1 = getelementptr { ptr, i32, ptr, ptr, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 4
  %2 = getelementptr { ptr, i32, ptr, ptr, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 3
  %3 = getelementptr { ptr, i32, ptr, ptr, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 2
  store { ptr, i32, ptr, ptr, ptr, ptr } { ptr null, i32 4, ptr null, ptr null, ptr null, ptr null }, ptr %gc.stackobject, align 4
  %4 = load ptr, ptr @runtime.stackChainStart, align 4
  %5 = getelementptr { ptr, i32, ptr, ptr, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 0
  store ptr %4, ptr %5, align 4
  store ptr %gc.stackobject, ptr @runtime.stackChainStart, align 4
  %original = call ptr @getPointer()
  store ptr %original, ptr %3, align 4
  br label %outer

outer:                                            ; preds = %latch, %entry
  %outer.ptr = phi ptr [ %original, %entry ], [ %inner.ptr, %latch ]
  store ptr null, ptr %0, align 4
  store ptr null, ptr %1, align 4
  store ptr %outer.ptr, ptr %2, align 4
  br label %inner

inner:                                            ; preds = %inner, %outer
  %inner.ptr = phi ptr [ %outer.ptr, %outer ], [ %next, %inner ]
  store ptr %inner.ptr, ptr %1, align 4
  %next = call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  store ptr %next, ptr %0, align 4
  br i1 %repeat.inner, label %inner, label %latch

latch:                                            ; preds = %inner
  br i1 %repeat.outer, label %outer, label %end

end:                                              ; preds = %latch
  store volatile { ptr, i32, ptr, ptr, ptr, ptr } zeroinitializer, ptr %gc.stackobject, align 4
  store ptr %4, ptr @runtime.stackChainStart, align 4
  ret ptr %inner.ptr
}

define ptr @duplicateAcyclicPhi(i1 %condition) {
entry:
  %gc.stackobject = alloca { ptr, i32, ptr }, align 8
  %0 = getelementptr { ptr, i32, ptr }, ptr %gc.stackobject, i32 0, i32 2
  store { ptr, i32, ptr } { ptr null, i32 1, ptr null }, ptr %gc.stackobject, align 4
  %1 = load ptr, ptr @runtime.stackChainStart, align 4
  %2 = getelementptr { ptr, i32, ptr }, ptr %gc.stackobject, i32 0, i32 0
  store ptr %1, ptr %2, align 4
  store ptr %gc.stackobject, ptr @runtime.stackChainStart, align 4
  br i1 %condition, label %left, label %right

left:                                             ; preds = %entry
  %left.ptr = call ptr @getPointer()
  br label %join

right:                                            ; preds = %entry
  %right.ptr = call ptr @getPointer()
  br label %join

join:                                             ; preds = %right, %left
  %merged = phi ptr [ %left.ptr, %left ], [ %right.ptr, %right ]
  store ptr %merged, ptr %0, align 4
  %unused = call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  store volatile { ptr, i32, ptr } zeroinitializer, ptr %gc.stackobject, align 4
  store ptr %1, ptr @runtime.stackChainStart, align 4
  ret ptr %merged
}

declare ptr @arrayAlloc()

define void @testGEPBitcast() {
  %gc.stackobject = alloca { ptr, i32, ptr, ptr }, align 8
  %1 = getelementptr { ptr, i32, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 3
  %2 = getelementptr { ptr, i32, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 2
  store { ptr, i32, ptr, ptr } { ptr null, i32 2, ptr null, ptr null }, ptr %gc.stackobject, align 4
  %3 = load ptr, ptr @runtime.stackChainStart, align 4
  %4 = getelementptr { ptr, i32, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 0
  store ptr %3, ptr %4, align 4
  store ptr %gc.stackobject, ptr @runtime.stackChainStart, align 4
  %arr = call ptr @arrayAlloc()
  %arr.bitcast = getelementptr [32 x i8], ptr %arr, i32 0, i32 0
  store ptr %arr.bitcast, ptr %2, align 4
  store ptr null, ptr %2, align 4
  %other = call ptr @runtime.alloc(i32 1, ptr inttoptr (i32 3 to ptr))
  store ptr %other, ptr %1, align 4
  store ptr null, ptr %1, align 4
  store volatile { ptr, i32, ptr, ptr } zeroinitializer, ptr %gc.stackobject, align 4
  store ptr %3, ptr @runtime.stackChainStart, align 4
  ret void
}

define void @someArbitraryFunction() {
  ret void
}

define void @earlyPopRegression() {
  %gc.stackobject = alloca { ptr, i32, ptr }, align 8
  %1 = getelementptr { ptr, i32, ptr }, ptr %gc.stackobject, i32 0, i32 2
  store { ptr, i32, ptr } { ptr null, i32 1, ptr null }, ptr %gc.stackobject, align 4
  %2 = load ptr, ptr @runtime.stackChainStart, align 4
  %3 = getelementptr { ptr, i32, ptr }, ptr %gc.stackobject, i32 0, i32 0
  store ptr %2, ptr %3, align 4
  store ptr %gc.stackobject, ptr @runtime.stackChainStart, align 4
  %x.alloc = call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  store ptr %x.alloc, ptr %1, align 4
  call void @allocAndSave(ptr %x.alloc)
  store ptr null, ptr %1, align 4
  store volatile { ptr, i32, ptr } zeroinitializer, ptr %gc.stackobject, align 4
  store ptr %2, ptr @runtime.stackChainStart, align 4
  ret void
}

define void @allocAndSave(ptr %x) {
  %gc.stackobject = alloca { ptr, i32, ptr }, align 8
  %1 = getelementptr { ptr, i32, ptr }, ptr %gc.stackobject, i32 0, i32 2
  store { ptr, i32, ptr } { ptr null, i32 1, ptr null }, ptr %gc.stackobject, align 4
  %2 = load ptr, ptr @runtime.stackChainStart, align 4
  %3 = getelementptr { ptr, i32, ptr }, ptr %gc.stackobject, i32 0, i32 0
  store ptr %2, ptr %3, align 4
  store ptr %gc.stackobject, ptr @runtime.stackChainStart, align 4
  %y = call ptr @runtime.alloc(i32 4, ptr inttoptr (i32 3 to ptr))
  store ptr %y, ptr %1, align 4
  store ptr %y, ptr %x, align 4
  store ptr null, ptr %1, align 4
  store ptr %x, ptr @ptrGlobal, align 4
  store volatile { ptr, i32, ptr } zeroinitializer, ptr %gc.stackobject, align 4
  store ptr %2, ptr @runtime.stackChainStart, align 4
  ret void
}

declare void @usePointer(ptr)

define void @deadBeforeCollection() {
  %gc.stackobject = alloca { ptr, i32, ptr }, align 8
  %1 = getelementptr { ptr, i32, ptr }, ptr %gc.stackobject, i32 0, i32 2
  store { ptr, i32, ptr } { ptr null, i32 1, ptr null }, ptr %gc.stackobject, align 4
  %2 = load ptr, ptr @runtime.stackChainStart, align 4
  %3 = getelementptr { ptr, i32, ptr }, ptr %gc.stackobject, i32 0, i32 0
  store ptr %2, ptr %3, align 4
  store ptr %gc.stackobject, ptr @runtime.stackChainStart, align 4
  %ptr = call ptr @getPointer()
  store ptr %ptr, ptr %1, align 4
  call void @usePointer(ptr %ptr)
  store ptr null, ptr %1, align 4
  call void @someArbitraryFunction()
  store volatile { ptr, i32, ptr } zeroinitializer, ptr %gc.stackobject, align 4
  store ptr %2, ptr @runtime.stackChainStart, align 4
  ret void
}

define void @liveInteriorPointer() {
  %gc.stackobject = alloca { ptr, i32, ptr }, align 8
  %1 = getelementptr { ptr, i32, ptr }, ptr %gc.stackobject, i32 0, i32 2
  store { ptr, i32, ptr } { ptr null, i32 1, ptr null }, ptr %gc.stackobject, align 4
  %2 = load ptr, ptr @runtime.stackChainStart, align 4
  %3 = getelementptr { ptr, i32, ptr }, ptr %gc.stackobject, i32 0, i32 0
  store ptr %2, ptr %3, align 4
  store ptr %gc.stackobject, ptr @runtime.stackChainStart, align 4
  %ptr = call ptr @getPointer()
  store ptr %ptr, ptr %1, align 4
  %field = getelementptr i8, ptr %ptr, i32 1
  call void @someArbitraryFunction()
  call void @usePointer(ptr %field)
  store ptr null, ptr %1, align 4
  call void @someArbitraryFunction()
  store volatile { ptr, i32, ptr } zeroinitializer, ptr %gc.stackobject, align 4
  store ptr %2, ptr @runtime.stackChainStart, align 4
  ret void
}

define void @deadOnOneBranch(i1 %condition) {
entry:
  %gc.stackobject = alloca { ptr, i32, ptr }, align 8
  %0 = getelementptr { ptr, i32, ptr }, ptr %gc.stackobject, i32 0, i32 2
  store { ptr, i32, ptr } { ptr null, i32 1, ptr null }, ptr %gc.stackobject, align 4
  %1 = load ptr, ptr @runtime.stackChainStart, align 4
  %2 = getelementptr { ptr, i32, ptr }, ptr %gc.stackobject, i32 0, i32 0
  store ptr %1, ptr %2, align 4
  store ptr %gc.stackobject, ptr @runtime.stackChainStart, align 4
  %ptr = call ptr @getPointer()
  store ptr %ptr, ptr %0, align 4
  br i1 %condition, label %live, label %dead

live:                                             ; preds = %entry
  call void @someArbitraryFunction()
  call void @usePointer(ptr %ptr)
  store ptr null, ptr %0, align 4
  br label %end

dead:                                             ; preds = %entry
  store ptr null, ptr %0, align 4
  call void @someArbitraryFunction()
  br label %end

end:                                              ; preds = %dead, %live
  call void @someArbitraryFunction()
  store volatile { ptr, i32, ptr } zeroinitializer, ptr %gc.stackobject, align 4
  store ptr %1, ptr @runtime.stackChainStart, align 4
  ret void
}

define void @liveAcrossLoop(i1 %repeat) {
entry:
  %gc.stackobject = alloca { ptr, i32, ptr }, align 8
  %0 = getelementptr { ptr, i32, ptr }, ptr %gc.stackobject, i32 0, i32 2
  store { ptr, i32, ptr } { ptr null, i32 1, ptr null }, ptr %gc.stackobject, align 4
  %1 = load ptr, ptr @runtime.stackChainStart, align 4
  %2 = getelementptr { ptr, i32, ptr }, ptr %gc.stackobject, i32 0, i32 0
  store ptr %1, ptr %2, align 4
  store ptr %gc.stackobject, ptr @runtime.stackChainStart, align 4
  %ptr = call ptr @getPointer()
  store ptr %ptr, ptr %0, align 4
  br label %loop

loop:                                             ; preds = %loop, %entry
  call void @someArbitraryFunction()
  call void @usePointer(ptr %ptr)
  br i1 %repeat, label %loop, label %end

end:                                              ; preds = %loop
  store ptr null, ptr %0, align 4
  call void @someArbitraryFunction()
  store volatile { ptr, i32, ptr } zeroinitializer, ptr %gc.stackobject, align 4
  store ptr %1, ptr @runtime.stackChainStart, align 4
  ret void
}

declare { ptr, i32 } @getAggregate()

declare void @useAggregate({ ptr, i32 })

define void @liveAggregate() {
  %gc.stackobject = alloca { ptr, i32, ptr }, align 8
  %1 = getelementptr { ptr, i32, ptr }, ptr %gc.stackobject, i32 0, i32 2
  store { ptr, i32, ptr } { ptr null, i32 1, ptr null }, ptr %gc.stackobject, align 4
  %2 = load ptr, ptr @runtime.stackChainStart, align 4
  %3 = getelementptr { ptr, i32, ptr }, ptr %gc.stackobject, i32 0, i32 0
  store ptr %2, ptr %3, align 4
  store ptr %gc.stackobject, ptr @runtime.stackChainStart, align 4
  %value = call { ptr, i32 } @getAggregate()
  %ptr = extractvalue { ptr, i32 } %value, 0
  store ptr %ptr, ptr %1, align 4
  call void @someArbitraryFunction()
  call void @useAggregate({ ptr, i32 } %value)
  store ptr null, ptr %1, align 4
  call void @someArbitraryFunction()
  store volatile { ptr, i32, ptr } zeroinitializer, ptr %gc.stackobject, align 4
  store ptr %2, ptr @runtime.stackChainStart, align 4
  ret void
}

define void @deadStackAllocation() {
  %gc.stackobject = alloca { ptr, i32, ptr, ptr }, align 8
  %1 = getelementptr { ptr, i32, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 3
  %2 = getelementptr { ptr, i32, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 2
  store { ptr, i32, ptr, ptr } { ptr null, i32 2, ptr null, ptr null }, ptr %gc.stackobject, align 4
  %3 = load ptr, ptr @runtime.stackChainStart, align 4
  %4 = getelementptr { ptr, i32, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 0
  store ptr %3, ptr %4, align 4
  store ptr %gc.stackobject, ptr @runtime.stackChainStart, align 4
  %storage = alloca ptr, align 4
  %ptr = call ptr @getPointer()
  store ptr %ptr, ptr %2, align 4
  store ptr %ptr, ptr %storage, align 4
  store ptr null, ptr %2, align 4
  call void @someArbitraryFunction()
  %loaded = load ptr, ptr %storage, align 4
  store volatile ptr null, ptr %storage, align 4
  store ptr %loaded, ptr %1, align 4
  call void @usePointer(ptr %loaded)
  store ptr null, ptr %1, align 4
  call void @someArbitraryFunction()
  store volatile { ptr, i32, ptr, ptr } zeroinitializer, ptr %gc.stackobject, align 4
  store ptr %3, ptr @runtime.stackChainStart, align 4
  ret void
}

define void @escapedStackAllocation() {
  %storage = alloca ptr, align 4
  %ptr = call ptr @getPointer()
  store ptr %ptr, ptr %storage, align 4
  store ptr %storage, ptr @ptrGlobal, align 4
  call void @someArbitraryFunction()
  ret void
}

define void @deadByteArrayStackAllocation() {
  %gc.stackobject = alloca { ptr, i32, ptr, ptr }, align 8
  %1 = getelementptr { ptr, i32, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 3
  %2 = getelementptr { ptr, i32, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 2
  store { ptr, i32, ptr, ptr } { ptr null, i32 2, ptr null, ptr null }, ptr %gc.stackobject, align 4
  %3 = load ptr, ptr @runtime.stackChainStart, align 4
  %4 = getelementptr { ptr, i32, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 0
  store ptr %3, ptr %4, align 4
  store ptr %gc.stackobject, ptr @runtime.stackChainStart, align 4
  %storage = alloca [32 x i8], align 4
  %ptr = call ptr @getPointer()
  store ptr %ptr, ptr %2, align 4
  store ptr %ptr, ptr %storage, align 4
  store ptr null, ptr %2, align 4
  call void @someArbitraryFunction()
  %loaded = load ptr, ptr %storage, align 4
  store volatile [32 x i8] zeroinitializer, ptr %storage, align 1
  store ptr %loaded, ptr %1, align 4
  call void @usePointer(ptr %loaded)
  store ptr null, ptr %1, align 4
  call void @someArbitraryFunction()
  store volatile { ptr, i32, ptr, ptr } zeroinitializer, ptr %gc.stackobject, align 4
  store ptr %3, ptr @runtime.stackChainStart, align 4
  ret void
}

define void @deadPointerFreeStackAllocation() {
  %storage = alloca [32 x i8], align 4, !tinygo.gc.pointerfree !0
  store i32 42, ptr %storage, align 4
  call void @readPointerStorage(ptr %storage)
  call void @someArbitraryFunction()
  ret void
}

define void @promotedPointerFreeStackAllocation() {
  %storage = call align 4 ptr @runtime.alloc(i32 32, ptr inttoptr (i32 3 to ptr))
  store i32 42, ptr %storage, align 4
  call void @readPointerStorage(ptr %storage)
  call void @someArbitraryFunction()
  ret void
}

define void @promotedPointerStackAllocation() {
  %storage = call align 4 ptr @runtime.alloc(i32 32, ptr inttoptr (i32 67 to ptr))
  %ptr = call ptr @getPointer()
  store ptr %ptr, ptr %storage, align 4
  call void @readPointerStorage(ptr %storage)
  call void @someArbitraryFunction()
  ret void
}

define void @mergedPointerFreeStackAllocation([8 x i32] %bytes) {
  %source = alloca [32 x i8], align 4, !tinygo.gc.pointerfree !0
  %destination = alloca [32 x i8], align 4
  store [8 x i32] %bytes, ptr %source, align 4
  call void @readPointerStorage(ptr %source)
  call void @llvm.memcpy.p0.p0.i32(ptr align 4 %destination, ptr align 4 %source, i32 32, i1 false)
  %ptr = call ptr @getPointer()
  store ptr %ptr, ptr %destination, align 4
  call void @readPointerStorage(ptr %destination)
  store volatile [32 x i8] zeroinitializer, ptr %destination, align 1
  call void @someArbitraryFunction()
  ret void
}

; Function Attrs: nocallback nofree nounwind willreturn memory(argmem: readwrite)
declare void @llvm.memcpy.p0.p0.i32(ptr noalias nocapture writeonly, ptr noalias nocapture readonly, i32, i1 immarg) #0

; Function Attrs: nocallback nofree nosync nounwind willreturn memory(argmem: readwrite)
declare void @llvm.lifetime.start.p0(ptr nocapture) #1

; Function Attrs: nocallback nofree nosync nounwind willreturn memory(argmem: readwrite)
declare void @llvm.lifetime.end.p0(ptr nocapture) #1

declare void @readPointerStorage(ptr nocapture readonly)

define void @deadStackLifetime(i1 %condition) {
entry:
  %gc.stackobject = alloca { ptr, i32, ptr }, align 8
  %0 = getelementptr { ptr, i32, ptr }, ptr %gc.stackobject, i32 0, i32 2
  store { ptr, i32, ptr } { ptr null, i32 1, ptr null }, ptr %gc.stackobject, align 4
  %1 = load ptr, ptr @runtime.stackChainStart, align 4
  %2 = getelementptr { ptr, i32, ptr }, ptr %gc.stackobject, i32 0, i32 0
  store ptr %1, ptr %2, align 4
  store ptr %gc.stackobject, ptr @runtime.stackChainStart, align 4
  %storage = alloca ptr, align 4
  br i1 %condition, label %active, label %end

active:                                           ; preds = %entry
  call void @llvm.lifetime.start.p0(ptr %storage)
  %ptr = call ptr @getPointer()
  store ptr %ptr, ptr %0, align 4
  store ptr %ptr, ptr %storage, align 4
  store ptr null, ptr %0, align 4
  call void @readPointerStorage(ptr %storage)
  store volatile ptr null, ptr %storage, align 4
  call void @someArbitraryFunction()
  call void @llvm.lifetime.end.p0(ptr %storage)
  br label %end

end:                                              ; preds = %active, %entry
  call void @someArbitraryFunction()
  store volatile { ptr, i32, ptr } zeroinitializer, ptr %gc.stackobject, align 4
  store ptr %1, ptr @runtime.stackChainStart, align 4
  ret void
}

define void @restartedStackLifetime() {
  %gc.stackobject = alloca { ptr, i32, ptr, ptr }, align 8
  %1 = getelementptr { ptr, i32, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 3
  %2 = getelementptr { ptr, i32, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 2
  store { ptr, i32, ptr, ptr } { ptr null, i32 2, ptr null, ptr null }, ptr %gc.stackobject, align 4
  %3 = load ptr, ptr @runtime.stackChainStart, align 4
  %4 = getelementptr { ptr, i32, ptr, ptr }, ptr %gc.stackobject, i32 0, i32 0
  store ptr %3, ptr %4, align 4
  store ptr %gc.stackobject, ptr @runtime.stackChainStart, align 4
  %storage = alloca ptr, align 4
  call void @llvm.lifetime.start.p0(ptr %storage)
  %first = call ptr @getPointer()
  store ptr %first, ptr %2, align 4
  store ptr %first, ptr %storage, align 4
  store ptr null, ptr %2, align 4
  call void @readPointerStorage(ptr %storage)
  store volatile ptr null, ptr %storage, align 4
  call void @llvm.lifetime.end.p0(ptr %storage)
  call void @someArbitraryFunction()
  call void @llvm.lifetime.start.p0(ptr %storage)
  %second = call ptr @getPointer()
  store ptr %second, ptr %1, align 4
  store ptr %second, ptr %storage, align 4
  store ptr null, ptr %1, align 4
  call void @readPointerStorage(ptr %storage)
  store volatile ptr null, ptr %storage, align 4
  call void @llvm.lifetime.end.p0(ptr %storage)
  call void @someArbitraryFunction()
  call void @llvm.lifetime.start.p0(ptr %storage)
  store volatile ptr null, ptr %storage, align 4
  call void @llvm.lifetime.end.p0(ptr %storage)
  call void @someArbitraryFunction()
  store volatile { ptr, i32, ptr, ptr } zeroinitializer, ptr %gc.stackobject, align 4
  store ptr %3, ptr @runtime.stackChainStart, align 4
  ret void
}

declare void @"(internal/task).Pause"()

define ptr @getAndPause() {
  %gc.stackobject = alloca { ptr, i32, ptr }, align 8
  %1 = getelementptr { ptr, i32, ptr }, ptr %gc.stackobject, i32 0, i32 2
  store { ptr, i32, ptr } { ptr null, i32 1, ptr null }, ptr %gc.stackobject, align 4
  %2 = load ptr, ptr @runtime.stackChainStart, align 4
  %3 = getelementptr { ptr, i32, ptr }, ptr %gc.stackobject, i32 0, i32 0
  store ptr %2, ptr %3, align 4
  store ptr %gc.stackobject, ptr @runtime.stackChainStart, align 4
  %ptr = call ptr @getPointer()
  store ptr %ptr, ptr %1, align 4
  call void @"(internal/task).Pause"()
  store volatile { ptr, i32, ptr } zeroinitializer, ptr %gc.stackobject, align 4
  store ptr %2, ptr @runtime.stackChainStart, align 4
  ret ptr %ptr
}

; Function Attrs: memory(readwrite)
declare void @externCallWithMemAttr() #2

define ptr @getAndCallWithMemAttr() {
  %gc.stackobject = alloca { ptr, i32, ptr }, align 8
  %1 = getelementptr { ptr, i32, ptr }, ptr %gc.stackobject, i32 0, i32 2
  store { ptr, i32, ptr } { ptr null, i32 1, ptr null }, ptr %gc.stackobject, align 4
  %2 = load ptr, ptr @runtime.stackChainStart, align 4
  %3 = getelementptr { ptr, i32, ptr }, ptr %gc.stackobject, i32 0, i32 0
  store ptr %2, ptr %3, align 4
  store ptr %gc.stackobject, ptr @runtime.stackChainStart, align 4
  %ptr = call ptr @getPointer()
  store ptr %ptr, ptr %1, align 4
  call void @externCallWithMemAttr()
  store volatile { ptr, i32, ptr } zeroinitializer, ptr %gc.stackobject, align 4
  store ptr %2, ptr @runtime.stackChainStart, align 4
  ret ptr %ptr
}

define { ptr, i32, i32 } @getSlice() {
  ret { ptr, i32, i32 } { ptr @someGlobal, i32 8, i32 8 }
}

define i32 @copyToSlice(ptr %src.ptr, i32 %src.len, i32 %src.cap) {
  %dst = call { ptr, i32, i32 } @getSlice()
  %dst.ptr = extractvalue { ptr, i32, i32 } %dst, 0
  %dst.len = extractvalue { ptr, i32, i32 } %dst, 1
  %minLen = call i32 @llvm.umin.i32(i32 %dst.len, i32 %src.len)
  call void @llvm.memmove.p0.p0.i32(ptr %dst.ptr, ptr %src.ptr, i32 %minLen, i1 false)
  ret i32 %minLen
}

; Function Attrs: nocallback nofree nosync nounwind speculatable willreturn memory(none)
declare i32 @llvm.umin.i32(i32, i32) #3

; Function Attrs: nocallback nofree nounwind willreturn memory(argmem: readwrite)
declare void @llvm.memmove.p0.p0.i32(ptr nocapture writeonly, ptr nocapture readonly, i32, i1 immarg) #0

attributes #0 = { nocallback nofree nounwind willreturn memory(argmem: readwrite) }
attributes #1 = { nocallback nofree nosync nounwind willreturn memory(argmem: readwrite) }
attributes #2 = { memory(readwrite) }
attributes #3 = { nocallback nofree nosync nounwind speculatable willreturn memory(none) }

!0 = !{}
