#ifndef SPIN_H
#define SPIN_H

static inline void yield_for_spin(size_t spin_count) {
  if (spin_count < 10) {
#if defined(__i386__) || defined(__x86_64__)
    __builtin_ia32_pause();
#endif
  }
}

#endif // SPIN_H
