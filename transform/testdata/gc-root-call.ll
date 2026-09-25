target datalayout = "e-m:e-p:32:32-i64:64-n32:64-S128"
target triple = "wasm32-unknown-unknown-wasm"

declare void @runtime.trackPointer(ptr, ptr) #0

define void @trackRoots(ptr %first, ptr %second, ptr %stack, i1 %condition) {
entry:
  br i1 %condition, label %track.first, label %track.second

track.first:
  call void @runtime.trackPointer(ptr %first, ptr %stack)
  br label %return

track.second:
  call void @runtime.trackPointer(ptr %second, ptr %stack)
  br label %return

return:
  ret void
}

define ptr @inlinedHelperLoop(ptr %stack, i32 %limit) {
entry:
  %stable = call ptr @newPointer()
  call void @runtime.trackPointer(ptr %stable, ptr %stack)
  br label %loop

loop:
  %count = phi i32 [ 0, %entry ], [ %next, %backedge ]
  %done = icmp eq i32 %count, %limit
  br i1 %done, label %exit, label %backedge

backedge:
  %next = add i32 %count, 1
  %updated = call ptr @newPointer()
  call void @runtime.trackPointer(ptr %updated, ptr %stack)
  br label %loop

exit:
  ret ptr %stable
}

declare ptr @newPointer()

attributes #0 = { nomerge }
