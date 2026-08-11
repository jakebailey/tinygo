#define GC_IMPL 1

#include <stdint.h>
#include <string.h>

#include "gc-stack.h"

void gc_stack_init(struct gc_stack *stack, struct gc_stack_addr base) {
  memset(stack, 0, sizeof(*stack));
}

void gc_stack_capture_hot(struct gc_stack *stack) {
}

void gc_stack_visit(struct gc_stack *stack,
                    void (*visit)(uintptr_t, uintptr_t, struct gc_heap *,
                                  void *),
                    struct gc_heap *heap, void *data) {
}
