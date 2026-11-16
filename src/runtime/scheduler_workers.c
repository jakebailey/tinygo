//go:build none

// Thread support for the workers scheduler on Linux and macOS. This file is
// always linked on these systems, unused functions are removed by the linker.

#define _GNU_SOURCE
#ifdef __linux__
#include <sched.h>
#endif
#include <errno.h>
#include <pthread.h>
#include <signal.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <sys/resource.h>
#include <unistd.h>

#ifdef __linux__
// BDWGC and the threads scheduler use the same signal.
#define workerGCSignal (SIGRTMIN + 6)
#else
// macOS has no real-time signals. The threads scheduler uses SIGIO too.
#define workerGCSignal SIGIO
#endif

// The worker structure of the current thread.
static __thread void *current_worker;

void *tinygo_worker_current(void) {
    return current_worker;
}

// Implemented in Go.
void tinygo_worker_gc_pause(void);
void tinygo_worker_start(void *worker);

static void gc_signal_handler(int sig) {
    // The handler interrupts arbitrary code, including code that is about to
    // read errno.
    int saved = errno;
    tinygo_worker_gc_pause();
    errno = saved;
}

// Install the signal handler that stops workers for the GC.
void tinygo_worker_gc_init(void) {
    struct sigaction act = { 0 };
    act.sa_handler = gc_signal_handler;
    act.sa_flags = SA_RESTART;
    sigfillset(&act.sa_mask);
    sigaction(workerGCSignal, &act, NULL);
}

// Initialize the main thread as the first worker. Returns the number of CPUs
// this process may run on.
int tinygo_worker_init(void *worker, uintptr_t *mainStackSize) {
    current_worker = worker;

    // The main thread stack grows on demand up to the stack limit.
    struct rlimit limit;
    uintptr_t size = 1 << 30;
    if (getrlimit(RLIMIT_STACK, &limit) == 0 && limit.rlim_cur != RLIM_INFINITY && limit.rlim_cur < size) {
        size = limit.rlim_cur;
    }
    *mainStackSize = size;

#ifdef __linux__
    cpu_set_t set;
    if (sched_getaffinity(0, sizeof(set), &set) == 0) {
        int n = CPU_COUNT(&set);
        if (n > 0) {
            return n;
        }
    }
#endif
    long n = sysconf(_SC_NPROCESSORS_ONLN);
    if (n > 0) {
        return n;
    }
    return 1;
}

static void *start_wrapper(void *worker) {
    current_worker = worker;
    tinygo_worker_start(worker);
    return NULL;
}

// Start a new thread for the given worker. Returns 0 on success.
int tinygo_worker_spawn(void *worker, uintptr_t stackSize) {
    pthread_attr_t attrs;
    pthread_attr_init(&attrs);
    pthread_attr_setdetachstate(&attrs, PTHREAD_CREATE_DETACHED);
    pthread_attr_setstacksize(&attrs, stackSize);

    pthread_t thread;
    int result = pthread_create(&thread, &attrs, start_wrapper, worker);
    pthread_attr_destroy(&attrs);
    return result;
}

pthread_t tinygo_worker_self(void) {
    return pthread_self();
}

// Ask a worker to stop for the GC.
void tinygo_worker_send_gc_signal(pthread_t thread) {
    pthread_kill(thread, workerGCSignal);
}

void *tinygo_worker_calloc(uintptr_t size) {
    return calloc(1, size);
}

// End the current worker thread. The worker is no longer in allWorkers.
void tinygo_worker_exit(void *worker) {
    free(worker);
    pthread_exit(NULL);
}
