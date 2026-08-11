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

extern void tinygo_whippet_trace_roots(void);

static void tinygo_whippet_heap_resized(void *data, size_t size) {
  tinygo_whippet_heap_bytes = size;
}

static void tinygo_whippet_live_data_size(void *data, size_t size) {
  tinygo_whippet_live_bytes = size;
}

static void tinygo_whippet_visit_pointer(
    uintptr_t value,
    void (*visit)(struct gc_edge, struct gc_heap *, void *),
    struct gc_heap *heap, void *data) {
  if (!value)
    return;
  struct gc_ref ref =
      gc_resolve_conservative_ref(heap, gc_conservative_ref(value), 1);
  if (!gc_ref_is_null(ref))
    visit(gc_edge(&ref), heap, data);
}

static void tinygo_whippet_trace_mask(
    uintptr_t start, uintptr_t mask,
    void (*visit)(struct gc_edge, struct gc_heap *, void *),
    struct gc_heap *heap, void *data) {
  while (mask) {
      unsigned skip = __builtin_ctzll((unsigned long long)mask);
    uintptr_t addr = start + skip * sizeof(uintptr_t);
    tinygo_whippet_visit_pointer(*(uintptr_t *)addr, visit, heap, data);
    mask &= mask - 1;
  }
}

static uintptr_t tinygo_whippet_load_bitmap_tail(const uint8_t *bitmap,
                                                  uintptr_t size) {
  uintptr_t mask = 0;
  for (uintptr_t i = 0; i < size; i++)
    mask |= (uintptr_t)bitmap[i] << (i * 8);
  return mask;
}

size_t tinygo_whippet_embedder_trace_object(
    uintptr_t object,
    void (*visit)(struct gc_edge, struct gc_heap *, void *),
    struct gc_heap *heap, void *data) {
  struct tinygo_whippet_header *header =
      (struct tinygo_whippet_header *)object;
  if (!visit)
    return header->size;

  uintptr_t start = (uintptr_t)(header + 1);
  uintptr_t remaining = header->size - sizeof(*header);
  uintptr_t layout = header->layout;
  const uintptr_t size_bits = 4 + sizeof(uintptr_t) / 4;

  if (layout & 1) {
    uintptr_t words = (layout >> 1) & (((uintptr_t)1 << size_bits) - 1);
    uintptr_t element_size = words * sizeof(uintptr_t);
    uintptr_t mask = layout >> (size_bits + 1);
    while (remaining >= element_size) {
      tinygo_whippet_trace_mask(start, mask, visit, heap, data);
      start += element_size;
      remaining -= element_size;
    }
  } else {
    uintptr_t words = *(uintptr_t *)layout;
    uintptr_t element_size = words * sizeof(uintptr_t);
    const uint8_t *bitmap =
        (const uint8_t *)(layout + sizeof(uintptr_t));
    uintptr_t bitmap_size = (words + 7) / 8;
    while (remaining >= element_size) {
      uintptr_t i = 0;
      for (; i + sizeof(uintptr_t) <= bitmap_size; i += sizeof(uintptr_t)) {
        uintptr_t mask = *(const uintptr_t *)(const void *)(bitmap + i);
        tinygo_whippet_trace_mask(start + i * 8 * sizeof(uintptr_t),
                                  mask, visit, heap, data);
      }
      if (i < bitmap_size) {
        uintptr_t mask =
            tinygo_whippet_load_bitmap_tail(bitmap + i, bitmap_size - i);
        tinygo_whippet_trace_mask(start + i * 8 * sizeof(uintptr_t),
                                  mask, visit, heap, data);
      }
      start += element_size;
      remaining -= element_size;
    }
  }
  return header->size;
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
  tinygo_whippet_visit_pointer(value, tinygo_whippet_visit,
                               tinygo_whippet_trace_heap,
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
