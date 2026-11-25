@callback.slot = global ptr null
@invoker.slot = global ptr null
@test.slot = global ptr @testFunction

define void @runtime.Callers() {
entry:
  ret void
}

define void @directCaller() {
entry:
  call void @runtime.Callers()
  ret void
}

define void @callback(i32 %value) alwaysinline {
entry:
  call void @runtime.Callers()
  ret void
}

define void @storeCallback() {
entry:
  store ptr @callback, ptr @callback.slot
  ret void
}

define void @indirectInvoker() {
entry:
  %fn = load ptr, ptr @callback.slot
  call void %fn(i32 0)
  ret void
}

define void @directParent() {
entry:
  call void @indirectInvoker()
  ret void
}

define void @storeInvoker() {
entry:
  store ptr @indirectInvoker, ptr @invoker.slot
  ret void
}

define void @secondLevelInvoker() {
entry:
  %fn = load ptr, ptr @invoker.slot
  call void %fn()
  ret void
}

define void @testFunction(ptr %value) {
entry:
  call void @runtime.Callers()
  ret void
}

define void @testHarness() {
entry:
  %fn = load ptr, ptr @test.slot
  call void %fn(ptr null)
  ret void
}

define void @unrelated() {
entry:
  ret void
}
