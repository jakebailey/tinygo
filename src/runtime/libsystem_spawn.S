//go:build none

// LibSystem stubs for spawn symbols absent from the minimal SDK.
.text
.macro stub symbol
    .globl _\symbol
    _\symbol:
    ret
.endmacro

stub posix_spawn
stub posix_spawnattr_init
stub posix_spawnattr_destroy
stub posix_spawnattr_setflags
stub posix_spawn_file_actions_init
stub posix_spawn_file_actions_destroy
stub posix_spawn_file_actions_adddup2
stub posix_spawn_file_actions_addinherit_np
stub posix_spawn_file_actions_addchdir_np
