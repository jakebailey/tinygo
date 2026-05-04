target datalayout = "e-m:e-i64:64-f80:128-n8:16:32:64-S128"
target triple = "x86_64--linux"

@effect.read = global i64 7
@effect.write = global i64 0
@effect.atomic = global i32 0
@effect.cas = global i32 0
@effect.returned = global i32 0
@effect.after = global i64 0
@effect.afterAtomic = global i32 0
@effect.afterCAS = global i32 0
@effect.afterReturned = global i32 0

define void @runtime.initAll() unnamed_addr {
entry:
  call void @effect.init(ptr undef)
  call void @after.init(ptr undef)
  ret void
}

define internal void @effect.init(ptr %context) unnamed_addr {
  call void @runtimeOnly()
  ret void
}

define internal void @runtimeOnly() unnamed_addr {
  %val = load i64, ptr @effect.read
  store i64 %val, ptr @effect.write
  %old = atomicrmw add ptr @effect.atomic, i32 1 seq_cst
  %pair = cmpxchg ptr @effect.cas, i32 0, i32 1 seq_cst seq_cst
  %ptr = call ptr @returnWritePtr()
  store i32 1, ptr %ptr
  fence seq_cst
  ret void
}

define internal ptr @returnWritePtr() unnamed_addr {
  ret ptr @effect.returned
}

define internal void @after.init(ptr %context) unnamed_addr {
  %val = load i64, ptr @effect.read
  store i64 %val, ptr @effect.after
  %atomic = load i32, ptr @effect.atomic
  store i32 %atomic, ptr @effect.afterAtomic
  %cas = load i32, ptr @effect.cas
  store i32 %cas, ptr @effect.afterCAS
  %returned = load i32, ptr @effect.returned
  store i32 %returned, ptr @effect.afterReturned
  ret void
}
