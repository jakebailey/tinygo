//go:build none

// Thread support for the workers scheduler on Windows. This file is always
// linked on Windows, unused functions are removed by the linker.

#include <windows.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

// The worker structure and thread handle of the current thread.
static __thread void *current_worker;
static __thread HANDLE current_thread;

void *tinygo_worker_current(void) {
    return current_worker;
}

// Implemented in Go. Weak, so other schedulers can link this file without it.
__attribute__((weak)) void tinygo_worker_start(void *worker);

// Initialize the main thread as the first worker. Returns the number of CPUs
// this process may run on.
int tinygo_worker_init(void *worker, uintptr_t *mainStackSize) {
    current_worker = worker;

    ULONG_PTR low, high;
    GetCurrentThreadStackLimits(&low, &high);
    *mainStackSize = high - low;

    DWORD_PTR processMask, systemMask;
    if (GetProcessAffinityMask(GetCurrentProcess(), &processMask, &systemMask)) {
        int n = __builtin_popcountll(processMask);
        if (n > 0) {
            return n;
        }
    }
    SYSTEM_INFO info;
    GetSystemInfo(&info);
    if (info.dwNumberOfProcessors > 0) {
        return info.dwNumberOfProcessors;
    }
    return 1;
}

static DWORD WINAPI start_wrapper(LPVOID worker) {
    current_worker = worker;
    tinygo_worker_start(worker);
    return 0;
}

// Start a new thread for the given worker. Returns 0 on success.
int tinygo_worker_spawn(void *worker, uintptr_t stackSize) {
    HANDLE thread = CreateThread(NULL, stackSize, start_wrapper, worker, STACK_SIZE_PARAM_IS_A_RESERVATION, NULL);
    if (thread == NULL) {
        return -1;
    }
    CloseHandle(thread);
    return 0;
}

// Return a handle to the current thread that other threads can use. It is
// closed when the worker exits.
uintptr_t tinygo_worker_self(void) {
    HANDLE process = GetCurrentProcess();
    DuplicateHandle(process, GetCurrentThread(), process, &current_thread, 0, FALSE, DUPLICATE_SAME_ACCESS);
    return (uintptr_t)current_thread;
}

// Stop a worker for the GC and store its general purpose registers in regs.
// Returns the stack pointer of the worker, or 0 on failure.
uintptr_t tinygo_worker_suspend(uintptr_t thread, uintptr_t *regs) {
    HANDLE handle = (HANDLE)thread;
    if (SuspendThread(handle) == (DWORD)-1) {
        return 0;
    }
    // SuspendThread is asynchronous. GetThreadContext waits until the thread
    // has stopped.
    // https://devblogs.microsoft.com/oldnewthing/20150205-00/?p=44743
    CONTEXT context;
    context.ContextFlags = CONTEXT_INTEGER | CONTEXT_CONTROL;
    if (!GetThreadContext(handle, &context)) {
        return 0;
    }
#if defined(__x86_64__)
    // Rax through R15 are stored next to each other.
    memcpy(regs, &context.Rax, 16 * sizeof(uintptr_t));
    return context.Rsp;
#elif defined(__aarch64__)
    memcpy(regs, context.X, 31 * sizeof(uintptr_t));
    return context.Sp;
#else
#error unknown architecture
#endif
}

void tinygo_worker_resume(uintptr_t thread) {
    ResumeThread((HANDLE)thread);
}

void *tinygo_worker_calloc(uintptr_t size) {
    return calloc(1, size);
}

// End the current worker thread. The worker is no longer in allWorkers.
void tinygo_worker_exit(void *worker) {
    free(worker);
    CloseHandle(current_thread);
    ExitThread(0);
}
