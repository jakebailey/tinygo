#define GC_GENERATIONAL 0
#define GC_PARALLEL 0
#define GC_CONSERVATIVE_ROOTS 1
#define GC_CONSERVATIVE_TRACE 1
#define GC_HAS_IMMEDIATES 0
#define GC_ATTRS "mmc-attrs.h"

#include <stdint.h>
#include <stdlib.h>

#include "../../../lib/whippet/api/gc-allocate.h"
#include "../../../lib/whippet/api/gc-null-event-listener.h"
#include "tinygo-whippet-embedder.h"

struct tinygo_whippet_header {
  uintptr_t size;
  uintptr_t layout;
};

static struct gc_heap *tinygo_whippet_heap;
static struct gc_mutator *tinygo_whippet_mutator;
static struct gc_mutator_roots tinygo_whippet_mutator_roots;
static struct gc_heap_roots tinygo_whippet_heap_roots;
static uintptr_t tinygo_whippet_heap_bytes;
static uintptr_t tinygo_whippet_live_bytes;

static void (*tinygo_whippet_visit)(struct gc_edge, struct gc_heap *, void *);
static void (*tinygo_whippet_visit_ambiguous)(uintptr_t, uintptr_t, int,
                                               struct gc_heap *, void *);
static struct gc_heap *tinygo_whippet_trace_heap;
static void *tinygo_whippet_trace_data;

extern uintptr_t tinygo_whippet_trace_object(uintptr_t object);
extern void tinygo_whippet_trace_roots(void);

static void tinygo_whippet_heap_resized(void *data, size_t size) {
  tinygo_whippet_heap_bytes = size;
}

static void tinygo_whippet_live_data_size(void *data, size_t size) {
  tinygo_whippet_live_bytes = size;
}

size_t tinygo_whippet_embedder_trace_object(
    uintptr_t object,
    void (*visit)(struct gc_edge, struct gc_heap *, void *),
    struct gc_heap *heap, void *data) {
  if (!visit)
    return ((struct tinygo_whippet_header *)object)->size;
  tinygo_whippet_visit = visit;
  tinygo_whippet_trace_heap = heap;
  tinygo_whippet_trace_data = data;
  size_t size = tinygo_whippet_trace_object(object);
  tinygo_whippet_visit = NULL;
  return size;
}

void tinygo_whippet_embedder_trace_roots(
    void (*trace_ambiguous)(uintptr_t, uintptr_t, int, struct gc_heap *, void *),
    struct gc_heap *heap, void *data) {
  tinygo_whippet_visit_ambiguous = trace_ambiguous;
  tinygo_whippet_trace_heap = heap;
  tinygo_whippet_trace_data = data;
  tinygo_whippet_trace_roots();
  tinygo_whippet_visit_ambiguous = NULL;
}

void tinygo_whippet_init(void) {
  struct gc_options *options = gc_allocate_options();
  gc_options_set_int(options, GC_OPTION_HEAP_SIZE_POLICY,
                     GC_HEAP_SIZE_GROWABLE);
  gc_options_set_int(options, GC_OPTION_PARALLELISM, 1);

  struct gc_stack_addr stack = {0};
  struct gc_event_listener listener = GC_NULL_EVENT_LISTENER;
  listener.init = tinygo_whippet_heap_resized;
  listener.heap_resized = tinygo_whippet_heap_resized;
  listener.live_data_size = tinygo_whippet_live_data_size;
  if (!gc_init(options, stack, &tinygo_whippet_heap,
               &tinygo_whippet_mutator, listener, NULL))
    abort();
  gc_mutator_set_roots(tinygo_whippet_mutator,
                       &tinygo_whippet_mutator_roots);
  gc_heap_set_roots(tinygo_whippet_heap, &tinygo_whippet_heap_roots);
}

void *tinygo_whippet_alloc(uintptr_t size, uintptr_t layout, int kind) {
  if (kind == GC_ALLOCATION_TAGGED) {
    uintptr_t total = size + sizeof(struct tinygo_whippet_header);
    struct tinygo_whippet_header *header =
        gc_allocate(tinygo_whippet_mutator, total, kind);
    if (!header)
      return NULL;
    header->size = total;
    header->layout = layout;
    return header + 1;
  }
  return gc_allocate(tinygo_whippet_mutator, size, kind);
}

void tinygo_whippet_collect(void) {
  gc_collect(tinygo_whippet_mutator, GC_COLLECTION_ANY);
}

uintptr_t tinygo_whippet_allocation_counter(void) {
  return gc_allocation_counter(tinygo_whippet_heap);
}

uintptr_t tinygo_whippet_heap_size(void) {
  return tinygo_whippet_heap_bytes;
}

uintptr_t tinygo_whippet_live_size(void) {
  return tinygo_whippet_live_bytes;
}

void tinygo_whippet_trace_pointer(uintptr_t value) {
  if (!value)
    return;
  struct gc_ref ref = gc_resolve_conservative_ref(
      tinygo_whippet_trace_heap, gc_conservative_ref(value), 1);
  if (gc_ref_is_null(ref))
    return;
  tinygo_whippet_visit(gc_edge(&ref), tinygo_whippet_trace_heap,
                       tinygo_whippet_trace_data);
}

void tinygo_whippet_trace_range(uintptr_t start, uintptr_t end) {
  tinygo_whippet_visit_ambiguous(start, end, 1, tinygo_whippet_trace_heap,
                                 tinygo_whippet_trace_data);
}

uintptr_t tinygo_whippet_object_base(uintptr_t value) {
  struct gc_ref ref = gc_resolve_conservative_ref(
      tinygo_whippet_heap, gc_conservative_ref(value), 1);
  return gc_ref_value(ref);
}

uintptr_t tinygo_whippet_object_size(uintptr_t object) {
  return gc_heap_object_size(tinygo_whippet_heap, gc_ref(object));
}

void *tinygo_whippet_manual_alloc(uintptr_t size) {
  return calloc(1, size);
}

void tinygo_whippet_manual_free(void *ptr) {
  free(ptr);
}
