; ModuleID = 'string.go'
source_filename = "string.go"
target datalayout = "e-m:e-p:32:32-p10:8:8-p20:8:8-i64:64-i128:128-n32:64-S128-ni:1:10:20"
target triple = "wasm32-unknown-wasi"

%runtime._string = type { ptr, i32 }

@"main$string" = internal unnamed_addr constant [3 x i8] c"foo", align 1

declare void @runtime.trackPointer(ptr nocapture readonly, ptr, ptr) #0

; Function Attrs: nounwind
define hidden void @main.init(ptr %context) unnamed_addr #1 {
entry:
  ret void
}

; Function Attrs: nounwind
define hidden %runtime._string @main.someString(ptr %context) unnamed_addr #1 {
entry:
  ret %runtime._string { ptr @"main$string", i32 3 }
}

; Function Attrs: nounwind
define hidden %runtime._string @main.zeroLengthString(ptr %context) unnamed_addr #1 {
entry:
  ret %runtime._string zeroinitializer
}

; Function Attrs: nounwind
define hidden i32 @main.stringLen(ptr readonly %s.data, i32 %s.len, ptr %context) unnamed_addr #1 {
entry:
  ret i32 %s.len
}

; Function Attrs: nounwind
define hidden i8 @main.stringIndex(ptr readonly %s.data, i32 %s.len, i32 %index, ptr %context) unnamed_addr #1 {
entry:
  %.not = icmp ult i32 %index, %s.len
  br i1 %.not, label %lookup.next, label %lookup.throw

lookup.next:                                      ; preds = %entry
  %0 = getelementptr inbounds i8, ptr %s.data, i32 %index
  %1 = load i8, ptr %0, align 1
  ret i8 %1

lookup.throw:                                     ; preds = %entry
  call void @runtime.lookupPanic(ptr undef) #3
  unreachable
}

declare void @runtime.lookupPanic(ptr) #0

; Function Attrs: nounwind
define hidden i1 @main.stringCompareEqual(ptr readonly %s1.data, i32 %s1.len, ptr readonly %s2.data, i32 %s2.len, ptr %context) unnamed_addr #1 {
entry:
  %0 = call i1 @runtime.stringEqual(ptr %s1.data, i32 %s1.len, ptr %s2.data, i32 %s2.len, ptr undef) #3
  ret i1 %0
}

declare i1 @runtime.stringEqual(ptr readonly, i32, ptr readonly, i32, ptr) #0

; Function Attrs: nounwind
define hidden i1 @main.stringCompareUnequal(ptr readonly %s1.data, i32 %s1.len, ptr readonly %s2.data, i32 %s2.len, ptr %context) unnamed_addr #1 {
entry:
  %0 = call i1 @runtime.stringEqual(ptr %s1.data, i32 %s1.len, ptr %s2.data, i32 %s2.len, ptr undef) #3
  %1 = xor i1 %0, true
  ret i1 %1
}

; Function Attrs: nounwind
define hidden i1 @main.stringCompareLarger(ptr readonly %s1.data, i32 %s1.len, ptr readonly %s2.data, i32 %s2.len, ptr %context) unnamed_addr #1 {
entry:
  %0 = call i1 @runtime.stringLess(ptr %s2.data, i32 %s2.len, ptr %s1.data, i32 %s1.len, ptr undef) #3
  ret i1 %0
}

declare i1 @runtime.stringLess(ptr readonly, i32, ptr readonly, i32, ptr) #0

; Function Attrs: nounwind
define hidden i8 @main.stringLookup(ptr readonly %s.data, i32 %s.len, i8 %x, ptr %context) unnamed_addr #1 {
entry:
  %0 = zext i8 %x to i32
  %.not = icmp ugt i32 %s.len, %0
  br i1 %.not, label %lookup.next, label %lookup.throw

lookup.next:                                      ; preds = %entry
  %1 = getelementptr inbounds nuw i8, ptr %s.data, i32 %0
  %2 = load i8, ptr %1, align 1
  ret i8 %2

lookup.throw:                                     ; preds = %entry
  call void @runtime.lookupPanic(ptr undef) #3
  unreachable
}

; Function Attrs: nounwind
define hidden i8 @main.stringMapLookupFromBytes(ptr dereferenceable_or_null(48) %m, ptr %b.data, i32 %b.len, i32 %b.cap, ptr %context) unnamed_addr #1 {
entry:
  %hashmap.value = alloca i8, align 1
  %stackalloc = alloca i8, align 1
  call void @runtime.trackPointer(ptr %b.data, ptr nonnull %stackalloc, ptr undef) #3
  call void @llvm.lifetime.start.p0(i64 1, ptr nonnull %hashmap.value)
  %0 = call i1 @runtime.hashmapStringGet(ptr %m, ptr %b.data, i32 %b.len, ptr nonnull %hashmap.value, i32 1, ptr undef) #3
  %1 = load i8, ptr %hashmap.value, align 1
  call void @llvm.lifetime.end.p0(i64 1, ptr nonnull %hashmap.value)
  ret i8 %1
}

; Function Attrs: nocallback nofree nosync nounwind willreturn memory(argmem: readwrite)
declare void @llvm.lifetime.start.p0(i64 immarg, ptr nocapture) #2

declare i1 @runtime.hashmapStringGet(ptr dereferenceable_or_null(48), ptr readonly, i32, ptr, i32, ptr) #0

; Function Attrs: nocallback nofree nosync nounwind willreturn memory(argmem: readwrite)
declare void @llvm.lifetime.end.p0(i64 immarg, ptr nocapture) #2

; Function Attrs: nounwind
define hidden i8 @main.stringMapLookupFromBytesAfterMutation(ptr dereferenceable_or_null(48) %m, ptr %b.data, i32 %b.len, i32 %b.cap, ptr %context) unnamed_addr #1 {
entry:
  %hashmap.value = alloca i8, align 1
  %stackalloc = alloca i8, align 1
  %0 = call %runtime._string @runtime.stringFromBytes(ptr %b.data, i32 %b.len, i32 %b.cap, ptr undef) #3
  %1 = extractvalue %runtime._string %0, 0
  call void @runtime.trackPointer(ptr %1, ptr nonnull %stackalloc, ptr undef) #3
  %2 = icmp eq i32 %b.len, 0
  br i1 %2, label %lookup.throw, label %lookup.next

lookup.next:                                      ; preds = %entry
  store i8 1, ptr %b.data, align 1
  call void @llvm.lifetime.start.p0(i64 1, ptr nonnull %hashmap.value)
  %3 = extractvalue %runtime._string %0, 0
  %4 = extractvalue %runtime._string %0, 1
  %5 = call i1 @runtime.hashmapStringGet(ptr %m, ptr %3, i32 %4, ptr nonnull %hashmap.value, i32 1, ptr undef) #3
  %6 = load i8, ptr %hashmap.value, align 1
  call void @llvm.lifetime.end.p0(i64 1, ptr nonnull %hashmap.value)
  ret i8 %6

lookup.throw:                                     ; preds = %entry
  call void @runtime.lookupPanic(ptr undef) #3
  unreachable
}

declare %runtime._string @runtime.stringFromBytes(ptr nocapture readonly dereferenceable_or_null(1), i32, i32, ptr) #0

attributes #0 = { "target-features"="+bulk-memory,+bulk-memory-opt,+call-indirect-overlong,+mutable-globals,+nontrapping-fptoint,+sign-ext,-multivalue,-reference-types" }
attributes #1 = { nounwind "target-features"="+bulk-memory,+bulk-memory-opt,+call-indirect-overlong,+mutable-globals,+nontrapping-fptoint,+sign-ext,-multivalue,-reference-types" }
attributes #2 = { nocallback nofree nosync nounwind willreturn memory(argmem: readwrite) }
attributes #3 = { nounwind }
