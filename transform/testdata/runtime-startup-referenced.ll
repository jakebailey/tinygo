@goroot = global ptr @runtime.GOROOT
@godebug = global ptr @"internal/godebug.setUpdate"

declare i1 @runtime.gorootEnvEnabled(ptr)
declare i1 @runtime.godebugEnvEnabled(ptr)
declare ptr @runtime.GOROOT(ptr)
declare void @"internal/godebug.setUpdate"(ptr, ptr)

define i1 @test.goroot() {
entry:
  %enabled = call i1 @runtime.gorootEnvEnabled(ptr undef)
  ret i1 %enabled
}

define i1 @test.godebug() {
entry:
  %enabled = call i1 @runtime.godebugEnvEnabled(ptr undef)
  ret i1 %enabled
}
