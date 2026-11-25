//go:build none

// Ignore the //go:build above. This file is manually included on Linux and
// MacOS to provide os/signal support.

#include <stdint.h>
#include <signal.h>
#include <time.h>
#include <unistd.h>

// Signal handler in the runtime.
void tinygo_signal_handler(int sig);

static int signal_owned_by_runtime(uint32_t sig) {
    struct sigaction cur = { 0 };
    if (sigaction(sig, NULL, &cur) != 0) {
        return 1;
    }
    return cur.sa_handler != SIG_DFL &&
           cur.sa_handler != SIG_IGN &&
           cur.sa_handler != &tinygo_signal_handler;
}

// Enable a signal from the runtime.
void tinygo_signal_enable(uint32_t sig) {
    if (signal_owned_by_runtime(sig)) {
        return;
    }
    struct sigaction act = { 0 };
    act.sa_handler = &tinygo_signal_handler;
    act.sa_flags = SA_RESTART;
    sigaction(sig, &act, NULL);
}

void tinygo_signal_ignore(uint32_t sig) {
    if (signal_owned_by_runtime(sig)) {
        return;
    }
    struct sigaction act = { 0 };
    act.sa_handler = SIG_IGN;
    sigaction(sig, &act, NULL);
}

void tinygo_signal_disable(uint32_t sig) {
    if (signal_owned_by_runtime(sig)) {
        return;
    }
    struct sigaction act = { 0 };
    act.sa_handler = SIG_DFL;
    sigaction(sig, &act, NULL);
}

int tinygo_signal_ignored(uint32_t sig) {
    struct sigaction act = { 0 };
    if (sigaction(sig, NULL, &act) != 0) {
        return 0;
    }
    return act.sa_handler == SIG_IGN;
}
