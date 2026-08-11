#ifndef TINYGO_WHIPPET_EMBEDDER_H
#define TINYGO_WHIPPET_EMBEDDER_H

#include <stdint.h>

#include "../../../lib/whippet/api/gc-api.h"
#include "../../../lib/whippet/api/gc-embedder-api.h"

#define GC_EMBEDDER_EPHEMERON_HEADER uintptr_t tinygo_header;
#define GC_EMBEDDER_FINALIZER_HEADER uintptr_t tinygo_header;

struct gc_mutator_roots {
  uintptr_t unused;
};

struct gc_heap_roots {
  uintptr_t unused;
};

extern size_t tinygo_whippet_embedder_trace_object(
    uintptr_t object,
    void (*visit)(struct gc_edge, struct gc_heap *, void *),
    struct gc_heap *heap, void *data);
extern void tinygo_whippet_embedder_trace_roots(
    void (*trace_ambiguous)(uintptr_t, uintptr_t, int, struct gc_heap *, void *),
    struct gc_heap *heap, void *data);

static inline size_t gc_finalizer_priority_count(void) {
  return 1;
}

static inline int
gc_is_valid_conservative_ref_displacement(uintptr_t displacement) {
  return 1;
}

static inline int gc_extern_space_visit(struct gc_extern_space *space,
                                        struct gc_ref ref) {
  return 0;
}

static inline void gc_extern_space_start_gc(struct gc_extern_space *space,
                                             int is_minor_gc) {
}

static inline void gc_extern_space_finish_gc(struct gc_extern_space *space,
                                              int is_minor_gc) {
}

static inline size_t gc_trace_object(
    struct gc_ref ref,
    void (*visit)(struct gc_edge, struct gc_heap *, void *),
    struct gc_heap *heap, void *data) {
  return tinygo_whippet_embedder_trace_object(gc_ref_value(ref), visit, heap,
                                               data);
}

static inline void gc_trace_mutator_roots(
    struct gc_mutator_roots *roots,
    void (*trace_edge)(struct gc_edge, struct gc_heap *, void *),
    struct gc_heap *heap, void *data) {
}

static inline void gc_trace_heap_roots(
    struct gc_heap_roots *roots,
    void (*trace_edge)(struct gc_edge, struct gc_heap *, void *),
    struct gc_heap *heap, void *data) {
}

static inline void gc_trace_mutator_pinned_roots(
    struct gc_mutator_roots *roots,
    void (*trace_pinned)(struct gc_ref, struct gc_heap *, void *),
    void (*trace_ambiguous)(uintptr_t, uintptr_t, int, struct gc_heap *, void *),
    struct gc_heap *heap, void *data) {
  tinygo_whippet_embedder_trace_roots(trace_ambiguous, heap, data);
}

static inline void gc_trace_heap_pinned_roots(
    struct gc_heap_roots *roots,
    void (*trace_pinned)(struct gc_ref, struct gc_heap *, void *),
    void (*trace_ambiguous)(uintptr_t, uintptr_t, int, struct gc_heap *, void *),
    struct gc_heap *heap, void *data) {
}

static inline uintptr_t gc_object_forwarded_nonatomic(struct gc_ref ref) {
  GC_CRASH();
}

static inline void gc_object_forward_nonatomic(struct gc_ref ref,
                                                struct gc_ref new_ref) {
  GC_CRASH();
}

static inline struct gc_atomic_forward
gc_atomic_forward_begin(struct gc_ref ref) {
  GC_CRASH();
}

static inline void gc_atomic_forward_acquire(struct gc_atomic_forward *fwd) {
  GC_CRASH();
}

static inline int
gc_atomic_forward_retry_busy(struct gc_atomic_forward *fwd) {
  GC_CRASH();
}

static inline void gc_atomic_forward_abort(struct gc_atomic_forward *fwd) {
  GC_CRASH();
}

static inline size_t
gc_atomic_forward_object_size(struct gc_atomic_forward *fwd) {
  GC_CRASH();
}

static inline void gc_atomic_forward_commit(struct gc_atomic_forward *fwd,
                                             struct gc_ref new_ref) {
  GC_CRASH();
}

static inline uintptr_t
gc_atomic_forward_address(struct gc_atomic_forward *fwd) {
  GC_CRASH();
}

#endif
