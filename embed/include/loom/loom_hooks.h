// loom/loom_hooks.h — weak-symbol hooks. Standalone; no dependency on
// <loom.h>. Consumers that cannot or should not unconditionally link
// liblooom.a include this header and guard call sites with the LOOM_HOOK
// macro.
//
// When liblooom.a IS linked, the strong symbols defined by the embed lib
// override these weak ones and forward into the full implementation.
//
// When liblooom.a is NOT linked, the unreferenced weak symbols resolve to
// address zero. The LOOM_HOOK macro guards call sites with a null check
// that branch-predicts to "not taken" on the off-path build, achieving the
// ±0.5% byte-and-perf identity that Bedrock's contract requires.
//
// Platform note (macOS): Apple's linker rejects undefined weak symbols by
// default. Consumers that include this header without linking liblooom must
// pass `-Wl,-undefined,dynamic_lookup` (or equivalent CMake
// `target_link_options`). On Linux and Jetson Linux/aarch64 no extra flags
// are needed.
#pragma once

#ifdef __cplusplus
extern "C" {
#endif

#include <stdint.h>

#if defined(__GNUC__) || defined(__clang__)
  #define LOOM_WEAK __attribute__((weak))
#else
  #error "loom_hooks.h requires gcc or clang weak-symbol support"
#endif

LOOM_WEAK void loom_hook_span_begin(const char* name);
LOOM_WEAK void loom_hook_span_end  (const char* name);

// loom_hook_span_emit — emit a span with caller-supplied duration in
// nanoseconds. For CUDA kernels: record cudaEvent_t markers around
// the launch, drain at a sync point via cudaEventElapsedTime, and
// LOOM_HOOK(loom_hook_span_emit, name, ns) the result. Records mix
// freely with RAII spans in the manifest's percentile table.
LOOM_WEAK void loom_hook_span_emit (const char* name, uint64_t dur_ns);

// loom_hook_device_span_emit — same shape, but lands as cat="device_span"
// with backend + queue_id propagated into the event's attrs. Use this
// when the duration was measured on an asynchronous accelerator
// (GPU/NPU/DSP) so `loom show` can distinguish device-execution time
// from CPU-side dispatch / RAII span time for the same span name.
LOOM_WEAK void loom_hook_device_span_emit(const char* name,
                                          uint64_t    dur_ns,
                                          const char* backend,
                                          const char* queue_id);

LOOM_WEAK void loom_hook_metric_f64(const char* name, double v);
LOOM_WEAK void loom_hook_error     (const char* name, const char* message);
LOOM_WEAK void loom_hook_audit     (const char* name,
                                    const char* attrs_json,
                                    int sync);
LOOM_WEAK void loom_hook_lifecycle (const char* marker);

// Convenience macro: if the symbol resolves, call it; otherwise, no-op.
#define LOOM_HOOK(fn, ...) do { if (fn) { fn(__VA_ARGS__); } } while (0)

#ifdef __cplusplus
}
#endif
