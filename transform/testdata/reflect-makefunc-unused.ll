@"internal/reflectlite.makeFuncTypes" = global ptr null
@"internal/reflectlite.makeFuncAdapters" = global ptr null
@"internal/reflectlite.makeFuncLinksLen" = global i32 0
@"reflect/types.type:named:unusedMakeFuncType" = internal constant { i8 } zeroinitializer
@"reflect/makefunc.link:func:{}{}" = internal constant ptr @"reflect/makefunc:func:{}{}"
@"reflect/dynamicmethod.link:func:{}{}" = internal constant ptr @"reflect/makefunc:func:{}{}"

define internal void @unused() #0 {
entry:
  ret void
}

define weak_odr void @"reflect/makefunc:func:{}{}"() {
entry:
  call void @"internal/reflectlite.makeFuncCall"()
  ret void
}

define internal void @"internal/reflectlite.makeFuncCall"() {
entry:
  %value = load i8, ptr @"reflect/types.type:named:unusedMakeFuncType"
  ret void
}

attributes #0 = { "tinygo-reflect-makefunc"="" }
