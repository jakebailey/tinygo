// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !baremetal && !tinygo.wasm && !nintendoswitch

package os

import (
	"syscall"
	"unsafe"
)

// blockUntilWaitable waits for the child to exit but leaves it for wait4 to reap.
func (p *Process) blockUntilWaitable() error {
	const _P_PID = 1
	var siginfo [16]uint64
	for {
		_, _, errno := syscall.Syscall6(syscall.SYS_WAITID, _P_PID, uintptr(p.Pid), uintptr(unsafe.Pointer(&siginfo[0])), syscall.WEXITED|syscall.WNOWAIT, 0, 0)
		switch errno {
		case 0:
			return nil
		case syscall.EINTR:
			continue
		default:
			return NewSyscallError("waitid", errno)
		}
	}
}

func dupCloexec(fd, min int) (int, error) {
	nfd, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), syscall.F_DUPFD_CLOEXEC, uintptr(min))
	if errno != 0 {
		return 0, errno
	}
	return int(nfd), nil
}

// spawnFileActions has the layout of posix_spawn_file_actions_t in musl and glibc.
type spawnFileActions struct {
	_       [2]int32
	actions unsafe.Pointer
	_       [16]int32
}

func spawnProcess(pid *int32, path *byte, fa *spawnFileActions, argv, envp **byte) int32 {
	return libc_posix_spawn(pid, path, fa, nil, argv, envp)
}

//export posix_spawn
func libc_posix_spawn(pid *int32, path *byte, fa *spawnFileActions, attrp unsafe.Pointer, argv, envp **byte) int32

//export posix_spawn_file_actions_addchdir_np
func spawnChdir(fa *spawnFileActions, path *byte) int32

//export posix_spawn_file_actions_addclose
func spawnClose(fa *spawnFileActions, fd int32) int32

func spawnDup(fa *spawnFileActions, fd, newfd int32) int32 {
	return libc_posix_spawn_file_actions_adddup2(fa, fd, newfd)
}
