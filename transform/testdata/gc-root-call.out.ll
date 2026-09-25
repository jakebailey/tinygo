target datalayout = "e-m:e-p:32:32-i64:64-n32:64-S128"
target triple = "wasm32-unknown-unknown-wasm"

; Function Attrs: nomerge
declare void @runtime.trackPointer(ptr, ptr) local_unnamed_addr #0

define void @trackRoots(ptr %first, ptr %second, ptr %stack, i1 %condition) local_unnamed_addr {
entry:
  br i1 %condition, label %track.first, label %track.second

track.first:                                      ; preds = %entry
  tail call void @runtime.trackPointer(ptr %first, ptr %stack)
  br label %return

track.second:                                     ; preds = %entry
  tail call void @runtime.trackPointer(ptr %second, ptr %stack)
  br label %return

return:                                           ; preds = %track.second, %track.first
  ret void
}

define ptr @inlinedHelperLoop(ptr %stack, i32 %limit) local_unnamed_addr {
entry:
  %stable = tail call ptr @newPointer()
  tail call void @runtime.trackPointer(ptr %stable, ptr %stack)
  br label %loop

loop:                                             ; preds = %backedge, %entry
  %count = phi i32 [ 0, %entry ], [ %next, %backedge ]
  %done = icmp eq i32 %count, %limit
  br i1 %done, label %exit, label %backedge

backedge:                                         ; preds = %loop
  %next = add i32 %count, 1
  %updated = tail call ptr @newPointer()
  tail call void @runtime.trackPointer(ptr %updated, ptr %stack)
  br label %loop

exit:                                             ; preds = %loop
  ret ptr %stable
}

declare ptr @newPointer() local_unnamed_addr

attributes #0 = { nomerge }
