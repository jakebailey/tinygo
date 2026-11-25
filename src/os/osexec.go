//go:build (darwin || linux) && !baremetal && !tinygo.wasm && !nintendoswitch

package os

import (
	"runtime"
	"syscall"
)

// spawn starts path with argv and the attributes in attr and returns the pid of the new process.
// attr.Files[i] becomes file descriptor i in the new process, and a nil entry closes it.
func spawn(path string, argv []string, attr *ProcAttr) (int, error) {
	var fa spawnFileActions
	if errno := libc_posix_spawn_file_actions_init(&fa); errno != 0 {
		return 0, syscall.Errno(errno)
	}
	defer libc_posix_spawn_file_actions_destroy(&fa)

	if attr.Dir != "" {
		dirp := cstring(attr.Dir)
		if errno := spawnChdir(&fa, &dirp[0]); errno != 0 {
			return 0, syscall.Errno(errno)
		}
	}

	files := attr.Files
	for i, f := range files {
		var errno int32
		if f == nil {
			errno = spawnClose(&fa, int32(i))
		} else {
			fd := int(f.Fd())
			if fd < len(files) && fd != i {
				// An earlier action could replace fd, so pass a copy above the slots.
				nfd, err := dupCloexec(fd, len(files))
				if err != nil {
					return 0, err
				}
				defer syscall.Close(nfd)
				fd = nfd
			}
			errno = spawnDup(&fa, int32(fd), int32(i))
		}
		if errno != 0 {
			return 0, syscall.Errno(errno)
		}
	}

	pathp := cstring(path)
	argvp := cstrings(argv)
	envp := cstrings(attr.Env)
	var pid int32
	errno := spawnProcess(&pid, &pathp[0], &fa, &argvp[0], &envp[0])
	runtime.KeepAlive(files)
	if errno != 0 {
		return 0, syscall.Errno(errno)
	}
	return int(pid), nil
}

func cstring(s string) []byte {
	data := make([]byte, len(s)+1)
	copy(data, s)
	// final byte should be zero from the initial allocation
	return data
}

// cstrings returns ss as a nil terminated array of C strings.
func cstrings(ss []string) []*byte {
	p := make([]*byte, len(ss)+1)
	for i, s := range ss {
		p[i] = &cstring(s)[0]
	}
	return p
}

// The posix_spawn functions return an error number instead of setting errno, see
// https://pubs.opengroup.org/onlinepubs/9799919799/functions/posix_spawn.html
//
//export posix_spawn_file_actions_init
func libc_posix_spawn_file_actions_init(fa *spawnFileActions) int32

//export posix_spawn_file_actions_destroy
func libc_posix_spawn_file_actions_destroy(fa *spawnFileActions) int32

//export posix_spawn_file_actions_adddup2
func libc_posix_spawn_file_actions_adddup2(fa *spawnFileActions, fd, newfd int32) int32
