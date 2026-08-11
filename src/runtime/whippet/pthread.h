#ifndef TINYGO_WHIPPET_PTHREAD_H
#define TINYGO_WHIPPET_PTHREAD_H

#include <time.h>

typedef int pthread_mutex_t;
typedef int pthread_cond_t;
typedef int pthread_t;

static inline int pthread_mutex_init(pthread_mutex_t *mutex, const void *attr) {
  *mutex = 0;
  return 0;
}

static inline int pthread_mutex_lock(pthread_mutex_t *mutex) {
  return 0;
}

static inline int pthread_mutex_trylock(pthread_mutex_t *mutex) {
  return 0;
}

static inline int pthread_mutex_unlock(pthread_mutex_t *mutex) {
  return 0;
}

static inline int pthread_cond_init(pthread_cond_t *cond, const void *attr) {
  *cond = 0;
  return 0;
}

static inline int pthread_cond_wait(pthread_cond_t *cond,
                                    pthread_mutex_t *mutex) {
  return 0;
}

static inline int pthread_cond_timedwait(pthread_cond_t *cond,
                                         pthread_mutex_t *mutex,
                                         const struct timespec *timeout) {
  return 0;
}

static inline int pthread_cond_signal(pthread_cond_t *cond) {
  return 0;
}

static inline int pthread_cond_broadcast(pthread_cond_t *cond) {
  return 0;
}

static inline int pthread_create(pthread_t *thread, const void *attr,
                                 void *(*start)(void *), void *arg) {
  *thread = 0;
  return 0;
}

static inline int pthread_join(pthread_t thread, void **result) {
  return 0;
}

#endif
