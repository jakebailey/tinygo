.globaltype __stack_pointer, i32

.functype jspi_pause (i32) -> ()
.import_module jspi_pause, gojs
.import_name jspi_pause, runtime.jspiPause
.functype jspi_exit () -> ()
.import_module jspi_exit, gojs
.import_name jspi_exit, runtime.jspiExit
.functype jspi_sleep_ticks (i64) -> ()
.import_module jspi_sleep_ticks, gojs
.import_name jspi_sleep_ticks, runtime.sleepTicks

.global tinygo_jspi_launch
.hidden tinygo_jspi_launch
.type tinygo_jspi_launch,@function
tinygo_jspi_launch:
    .functype tinygo_jspi_launch (i32) -> ()
    local.get 0
    i32.load 8
    global.set __stack_pointer
    local.get 0
    i32.load 4
    local.get 0
    i32.load 0
    call_indirect (i32) -> ()
    local.get 0
    i32.load 12
    global.set __stack_pointer
    return
    end_function

.global tinygo_jspi_pause
.hidden tinygo_jspi_pause
.type tinygo_jspi_pause,@function
tinygo_jspi_pause:
    .functype tinygo_jspi_pause (i32) -> ()
    local.get 0
    global.get __stack_pointer
    i32.store 8
    local.get 0
    i32.load 12
    global.set __stack_pointer
    local.get 0
    call jspi_pause
    local.get 0
    i32.load 8
    global.set __stack_pointer
    return
    end_function

.global tinygo_jspi_exit
.hidden tinygo_jspi_exit
.type tinygo_jspi_exit,@function
tinygo_jspi_exit:
    .functype tinygo_jspi_exit (i32) -> ()
    local.get 0
    i32.load 12
    global.set __stack_pointer
    call jspi_exit
    unreachable
    end_function

.global tinygo_jspi_sleepTicks
.hidden tinygo_jspi_sleepTicks
.type tinygo_jspi_sleepTicks,@function
tinygo_jspi_sleepTicks:
    .functype tinygo_jspi_sleepTicks (i64) -> ()
    .local i32
    global.get __stack_pointer
    local.set 1
    local.get 0
    call jspi_sleep_ticks
    local.get 1
    global.set __stack_pointer
    return
    end_function
