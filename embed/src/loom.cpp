#include "loom/loom_c.h"
#include "state.h"

#include <cstdlib>

extern "C" int loom_init(void) {
  const char* run_id = std::getenv("LOOM_RUN_ID");
  loom::detail::g_active.store(run_id != nullptr && run_id[0] != '\0',
                               std::memory_order_release);
  return 0;
}

extern "C" void loom_shutdown(void) {
  loom::detail::g_active.store(false, std::memory_order_release);
}

extern "C" int loom_active(void) {
  return loom::detail::g_active.load(std::memory_order_acquire) ? 1 : 0;
}

extern "C" uint64_t loom_span_begin(const char*, size_t,
                                    const uint8_t*, size_t) { return 0; }
extern "C" void     loom_span_end(uint64_t) {}
extern "C" void     loom_span_annotate(uint64_t,
                                       const char*, size_t,
                                       const uint8_t*, size_t) {}

extern "C" void loom_metric_i64 (const char*, size_t, int64_t) {}
extern "C" void loom_metric_f64 (const char*, size_t, double)  {}
extern "C" void loom_counter_inc(const char*, size_t, int64_t) {}

extern "C" void loom_error(const char*, size_t,
                           const char*, size_t,
                           int,
                           const uint8_t*, size_t) {}

extern "C" void loom_audit(const char*, size_t,
                           const uint8_t*, size_t,
                           int) {}

extern "C" void loom_lifecycle(const char*, size_t,
                               const uint8_t*, size_t) {}

extern "C" size_t loom_attrs_begin(uint8_t*, size_t)         { return 0; }
extern "C" size_t loom_attrs_i64  (uint8_t*, size_t, size_t,
                                   const char*, size_t, int64_t) { return 0; }
extern "C" size_t loom_attrs_f64  (uint8_t*, size_t, size_t,
                                   const char*, size_t, double)  { return 0; }
extern "C" size_t loom_attrs_str  (uint8_t*, size_t, size_t,
                                   const char*, size_t,
                                   const char*, size_t)         { return 0; }
extern "C" size_t loom_attrs_bool (uint8_t*, size_t, size_t,
                                   const char*, size_t, int)    { return 0; }
