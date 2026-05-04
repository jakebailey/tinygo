target datalayout = "e-m:e-i64:64-f80:128-n8:16:32:64-S128"
target triple = "x86_64--linux"

@effect.read = local_unnamed_addr global i64 7
@effect.write = local_unnamed_addr global i64 0
@effect.atomic = global i32 0
@effect.cas = global i32 0
@effect.returned = global i32 0
@effect.after = local_unnamed_addr global i64 0
@effect.afterAtomic = local_unnamed_addr global i32 0
@effect.afterCAS = local_unnamed_addr global i32 0
@effect.afterReturned = local_unnamed_addr global i32 0

define void @runtime.initAll() unnamed_addr {
entry:
  call fastcc void @runtimeOnly()
  %val = load i64, ptr @effect.read, align 8
  store i64 %val, ptr @effect.after, align 8
  %atomic = load i32, ptr @effect.atomic, align 4
  store i32 %atomic, ptr @effect.afterAtomic, align 4
  %cas = load i32, ptr @effect.cas, align 4
  store i32 %cas, ptr @effect.afterCAS, align 4
  %returned = load i32, ptr @effect.returned, align 4
  store i32 %returned, ptr @effect.afterReturned, align 4
  ret void
}

define internal fastcc void @runtimeOnly() unnamed_addr {
  %val = load i64, ptr @effect.read, align 8
  store i64 %val, ptr @effect.write, align 8
  %old = atomicrmw add ptr @effect.atomic, i32 1 seq_cst, align 4
  %pair = cmpxchg ptr @effect.cas, i32 0, i32 1 seq_cst seq_cst, align 4
  %ptr = call fastcc ptr @returnWritePtr()
  store i32 1, ptr %ptr, align 4
  fence seq_cst
  ret void
}

define internal fastcc ptr @returnWritePtr() unnamed_addr {
  ret ptr @effect.returned
}
