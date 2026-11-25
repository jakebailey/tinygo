//go:build none

// This file is included in the build, despite the //go:build line above.

#include <errno.h>
#include <fcntl.h>
#include <sys/wait.h>

// sys/ioctl.h is not in lib/macos-minimal-sdk. Declare ioctl here.
extern int ioctl(int fd, unsigned long request, ...);

int tinygo_darwin_wait_until_waitable(pid_t pid) {
    siginfo_t info;
    while (waitid(P_PID, pid, &info, WEXITED | WNOWAIT) < 0) {
        if (errno != EINTR) {
            return errno;
        }
    }
    return 0;
}

int tinygo_darwin_dup_cloexec(int fd, int min, int *result) {
    int dup = fcntl(fd, F_DUPFD_CLOEXEC, min);
    if (dup < 0) {
        return errno;
    }
    *result = dup;
    return 0;
}

// The minimal SDK lacks spawn.h. See xnu/libsyscall/wrappers/spawn/spawn.h
// and xnu/bsd/sys/spawn.h for the declarations and flag.
typedef void *spawn_actions;
typedef void *spawn_attrs;
extern int posix_spawn(pid_t *, const char *, const spawn_actions *, const spawn_attrs *, char *const *, char *const *);
extern int posix_spawnattr_init(spawn_attrs *);
extern int posix_spawnattr_destroy(spawn_attrs *);
extern int posix_spawnattr_setflags(spawn_attrs *, short);
extern int posix_spawn_file_actions_addchdir_np(spawn_actions *, const char *) __attribute__((weak_import));

int tinygo_darwin_spawn_chdir(spawn_actions *actions, const char *dir) {
    // This file action is available since macOS 10.15.
    int (*volatile addchdir)(spawn_actions *, const char *) = posix_spawn_file_actions_addchdir_np;
    if (!addchdir) {
        return ENOSYS;
    }
    return addchdir(actions, dir);
}

int tinygo_darwin_spawn(pid_t *pid, const char *path, spawn_actions *actions, char *const *argv, char *const *envp) {
    spawn_attrs attrs;
    int error = posix_spawnattr_init(&attrs);
    if (error) {
        return error;
    }
    const short POSIX_SPAWN_CLOEXEC_DEFAULT = 0x4000;
    error = posix_spawnattr_setflags(&attrs, POSIX_SPAWN_CLOEXEC_DEFAULT);
    if (!error) {
        error = posix_spawn(pid, path, actions, &attrs, argv, envp);
    }
    posix_spawnattr_destroy(&attrs);
    return error;
}

// Fixed-signature wrappers for the variadic libc imports. Variadic functions
// take stack arguments on darwin/arm64. See darwinVariadicImports in
// compiler/syscall.go.

int syscall_libc_open(uintptr_t pathname, uintptr_t flags, uintptr_t mode) {
    return open((const char *)pathname, (int)flags, (mode_t)mode);
}

int syscall_libc_ioctl(uintptr_t fd, uintptr_t request, uintptr_t arg) {
    return ioctl((int)fd, (unsigned long)request, (void *)arg);
}

// The third fcntl argument is an int for some commands and a pointer for
// other commands. The raw pointer-sized value is correct for both.
int syscall_libc_fcntl(uintptr_t fd, uintptr_t cmd, uintptr_t arg) {
    return fcntl((int)fd, (int)cmd, (void *)arg);
}

// x/sys calls openat through syscall6 with two trailing zero arguments. The
// two extra register arguments are harmless to a four-parameter callee.
int syscall_libc_openat(uintptr_t dirfd, uintptr_t pathname, uintptr_t flags, uintptr_t mode) {
    return openat((int)dirfd, (const char *)pathname, (int)flags, (mode_t)mode);
}

// The following functions are called by the runtime because Go can't call
// function pointers directly.

int tinygo_syscall(int (*fn)(uintptr_t a1, uintptr_t a2, uintptr_t a3), uintptr_t a1, uintptr_t a2, uintptr_t a3) {
    return fn(a1, a2, a3);
}

uintptr_t tinygo_syscallX(uintptr_t (*fn)(uintptr_t a1, uintptr_t a2, uintptr_t a3), uintptr_t a1, uintptr_t a2, uintptr_t a3) {
    return fn(a1, a2, a3);
}

int tinygo_syscall6(int (*fn)(uintptr_t a1, uintptr_t a2, uintptr_t a3, uintptr_t a4, uintptr_t a5, uintptr_t a6), uintptr_t a1, uintptr_t a2, uintptr_t a3, uintptr_t a4, uintptr_t a5, uintptr_t a6) {
    return fn(a1, a2, a3, a4, a5, a6);
}

uintptr_t tinygo_syscall6X(uintptr_t (*fn)(uintptr_t a1, uintptr_t a2, uintptr_t a3, uintptr_t a4, uintptr_t a5, uintptr_t a6), uintptr_t a1, uintptr_t a2, uintptr_t a3, uintptr_t a4, uintptr_t a5, uintptr_t a6) {
    return fn(a1, a2, a3, a4, a5, a6);
}
