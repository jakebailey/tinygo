//go:build none

// This file is manually included, to avoid CGo which would cause a circular
// import.

#include <windows.h>
#include <stdint.h>

void tinygo_futex_wait(uint32_t *addr, uint32_t cmp) {
    WaitOnAddress(addr, &cmp, sizeof(cmp), INFINITE);
}

void tinygo_futex_wait_timeout(uint32_t *addr, uint32_t cmp, uint64_t timeout) {
    if (timeout == 0) {
        return;
    }
    // Round up to whole milliseconds, and stay below INFINITE.
    uint64_t ms = (timeout + 999999) / 1000000;
    if (ms >= INFINITE) {
        ms = INFINITE - 1;
    }
    WaitOnAddress(addr, &cmp, sizeof(cmp), (DWORD)ms);
}

void tinygo_futex_wake(uint32_t *addr) {
    WakeByAddressSingle(addr);
}

void tinygo_futex_wake_all(uint32_t *addr) {
    WakeByAddressAll(addr);
}
