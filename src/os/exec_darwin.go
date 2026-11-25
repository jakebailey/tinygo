//go:build !baremetal && !tinygo.wasm && !nintendoswitch

package os

import (
	"syscall"
	"unsafe"
)

func (p *Process) blockUntilWaitable() error {
	if errno := syscall.Errno(darwinWaitUntilWaitable(int32(p.Pid))); errno != 0 {
		return NewSyscallError("waitid", errno)
	}
	return nil
}

func dupCloexec(fd, min int) (int, error) {
	var dup int32
	if errno := syscall.Errno(darwinDupCloexec(int32(fd), int32(min), &dup)); errno != 0 {
		return 0, errno
	}
	return int(dup), nil
}

// Darwin defines posix_spawn_file_actions_t as an opaque pointer in spawn.h.
type spawnFileActions unsafe.Pointer

func spawnClose(_ *spawnFileActions, _ int32) int32 {
	// POSIX_SPAWN_CLOEXEC_DEFAULT closes descriptors without file actions.
	return 0
}

func spawnDup(fa *spawnFileActions, fd, newfd int32) int32 {
	if fd == newfd {
		return darwinSpawnInherit(fa, fd)
	}
	return libc_posix_spawn_file_actions_adddup2(fa, fd, newfd)
}

//export tinygo_darwin_wait_until_waitable
func darwinWaitUntilWaitable(pid int32) int32

//export tinygo_darwin_dup_cloexec
func darwinDupCloexec(fd, min int32, dup *int32) int32

//export tinygo_darwin_spawn
func spawnProcess(pid *int32, path *byte, fa *spawnFileActions, argv, envp **byte) int32

//export tinygo_darwin_spawn_chdir
func spawnChdir(fa *spawnFileActions, path *byte) int32

//export posix_spawn_file_actions_addinherit_np
func darwinSpawnInherit(fa *spawnFileActions, fd int32) int32
