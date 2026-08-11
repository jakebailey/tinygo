#define GC_IMPL 1

#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

#ifdef _WIN32
#include <malloc.h>
#endif

#include "gc-platform.h"

void gc_platform_init(void) {
}

uintptr_t gc_platform_current_thread_stack_base(void) {
  return 0;
}

void gc_platform_visit_global_conservative_roots(
    void (*f)(uintptr_t, uintptr_t, struct gc_heap *, void *),
    struct gc_heap *heap, void *data) {
}

int gc_platform_processor_count(void) {
  return 1;
}

uint64_t gc_platform_monotonic_nanoseconds(void) {
  return 0;
}

size_t gc_platform_page_size(void) {
  return 4096;
}

static void *allocate_aligned(size_t size, size_t alignment) {
  if (!alignment)
    alignment = gc_platform_page_size();
#ifdef _WIN32
  return _aligned_malloc(size, alignment);
#else
  void *ret = NULL;
  if (posix_memalign(&ret, alignment, size))
    return NULL;
  return ret;
#endif
}

static void release_aligned(void *base) {
#ifdef _WIN32
  _aligned_free(base);
#else
  free(base);
#endif
}

struct gc_reservation gc_platform_reserve_memory(size_t size,
                                                  size_t alignment) {
  void *base = allocate_aligned(size, alignment);
  return (struct gc_reservation){(uintptr_t)base, base ? size : 0};
}

void *gc_platform_acquire_memory_from_reservation(
    struct gc_reservation reservation, size_t offset, size_t size) {
  if (offset > reservation.size || size > reservation.size - offset)
    return NULL;
  void *ret = (void *)(reservation.base + offset);
  memset(ret, 0, size);
  return ret;
}

void gc_platform_release_reservation(struct gc_reservation reservation) {
  if (reservation.base)
    release_aligned((void *)reservation.base);
}

void *gc_platform_acquire_memory(size_t size, size_t alignment) {
  void *ret = allocate_aligned(size, alignment);
  if (ret)
    memset(ret, 0, size);
  return ret;
}

void gc_platform_release_memory(void *base, size_t size) {
  release_aligned(base);
}

int gc_platform_populate_memory(void *addr, size_t size) {
  return 1;
}

int gc_platform_discard_memory(void *addr, size_t size) {
  memset(addr, 0, size);
  return 1;
}
