// loom/loom.h — public C++ API (C++17). Inline shims over the canonical
// C ABI in loom_c.h. The C++ wrappers exist for ergonomic call sites in
// the C++/CUDA consumer (Bedrock, the multimodal SDK, etc.) — there is
// no separate implementation; every wrapper packs an attribute buffer
// and dispatches to the corresponding loom_* function.
#pragma once
#include "loom/loom_c.h"
#include "loom/version.h"

#include <cstdint>
#include <cstring>
#include <initializer_list>
#include <string_view>

namespace loom {

// Attribute value variant. Held as a tagged union for trivial copy.
struct AttrValue {
  enum class Tag : uint8_t { I64, F64, Str, Bool, Bytes };
  Tag tag;
  union U {
    int64_t i64;
    double  f64;
    bool    b;
    struct { const char*    data; size_t len; } str;
    struct { const uint8_t* data; size_t len; } bytes;
  } u;

  AttrValue(int64_t v)              noexcept : tag(Tag::I64) { u.i64 = v; }
  AttrValue(int v)                  noexcept : tag(Tag::I64) { u.i64 = v; }
  AttrValue(double v)               noexcept : tag(Tag::F64) { u.f64 = v; }
  AttrValue(bool v)                 noexcept : tag(Tag::Bool){ u.b   = v; }
  AttrValue(std::string_view v)     noexcept : tag(Tag::Str) {
    u.str.data = v.data();
    u.str.len  = v.size();
  }
  AttrValue(const char* v)          noexcept : tag(Tag::Str) {
    u.str.data = v;
    u.str.len  = std::strlen(v);
  }
  static AttrValue from_bytes(const uint8_t* data, size_t len) noexcept {
    AttrValue v(int64_t{0});
    v.tag = Tag::Bytes;
    v.u.bytes.data = data;
    v.u.bytes.len  = len;
    return v;
  }
};

struct Attr {
  std::string_view key;
  AttrValue        value;
};

inline int  init()       noexcept { return loom_init(); }
inline void shutdown()   noexcept {        loom_shutdown(); }
inline bool active()     noexcept { return loom_active() != 0; }

// Attribute encoding into a stack buffer. The buffer is large enough for
// typical use; producers that need more should split into multiple events
// or call the C ABI directly with a heap buffer.
namespace detail {
constexpr size_t kAttrBufCap = 1024;
struct AttrBuf {
  uint8_t bytes[kAttrBufCap];
  size_t  len = 0;
};

inline AttrBuf encode(std::initializer_list<Attr> attrs) noexcept {
  AttrBuf b;
  b.len = loom_attrs_begin(b.bytes, kAttrBufCap);
  for (const Attr& a : attrs) {
    switch (a.value.tag) {
      case AttrValue::Tag::I64:
        b.len = loom_attrs_i64(b.bytes, kAttrBufCap, b.len,
                               a.key.data(), a.key.size(),
                               a.value.u.i64);
        break;
      case AttrValue::Tag::F64:
        b.len = loom_attrs_f64(b.bytes, kAttrBufCap, b.len,
                               a.key.data(), a.key.size(),
                               a.value.u.f64);
        break;
      case AttrValue::Tag::Str:
        b.len = loom_attrs_str(b.bytes, kAttrBufCap, b.len,
                               a.key.data(), a.key.size(),
                               a.value.u.str.data,
                               a.value.u.str.len);
        break;
      case AttrValue::Tag::Bool:
        b.len = loom_attrs_bool(b.bytes, kAttrBufCap, b.len,
                                a.key.data(), a.key.size(),
                                a.value.u.b ? 1 : 0);
        break;
      case AttrValue::Tag::Bytes:
        b.len = loom_attrs_bytes(b.bytes, kAttrBufCap, b.len,
                                 a.key.data(), a.key.size(),
                                 a.value.u.bytes.data,
                                 a.value.u.bytes.len);
        break;
    }
  }
  return b;
}
}  // namespace detail

// ── Spans ─────────────────────────────────────────────────────────────────
// RAII timer. Construction emits a span_begin record; destruction emits a
// matching span_end with dur_ns set. Parents are tracked per-thread.
class Span {
 public:
  explicit Span(std::string_view name) noexcept {
    handle_ = loom_span_begin(name.data(), name.size(), nullptr, 0);
  }
  Span(std::string_view name, std::initializer_list<Attr> attrs) noexcept {
    auto b  = detail::encode(attrs);
    handle_ = loom_span_begin(name.data(), name.size(), b.bytes, b.len);
  }
  ~Span() noexcept {
    if (handle_) loom_span_end(handle_);
  }
  Span(const Span&)            = delete;
  Span& operator=(const Span&) = delete;
  Span(Span&& o) noexcept : handle_(o.handle_) { o.handle_ = 0; }
  Span& operator=(Span&& o) noexcept {
    if (this != &o) {
      if (handle_) loom_span_end(handle_);
      handle_   = o.handle_;
      o.handle_ = 0;
    }
    return *this;
  }
  // Attach additional attributes to a span already in flight.
  void annotate(std::initializer_list<Attr> attrs) noexcept {
    if (!handle_) return;
    auto b = detail::encode(attrs);
    loom_span_annotate(handle_, b.bytes, b.len);
  }
 private:
  uint64_t handle_ = 0;
};

// Emit a span with an externally-supplied duration. For work whose
// real cost cannot be measured by RAII bracketing — most importantly
// CUDA kernels whose `<<<...>>>` launch returns long before the GPU
// actually executes them. Pair with cudaEvent_t markers and call this
// at a deferred drain after `cudaEventElapsedTime`.
inline void span_emit(std::string_view name, uint64_t dur_ns) noexcept {
  loom_span_emit(name.data(), name.size(), dur_ns, nullptr, 0);
}
inline void span_emit(std::string_view name, uint64_t dur_ns,
                      std::initializer_list<Attr> attrs) noexcept {
  auto b = detail::encode(attrs);
  loom_span_emit(name.data(), name.size(), dur_ns, b.bytes, b.len);
}

// ── Metrics ───────────────────────────────────────────────────────────────
inline void metric_i64(std::string_view name, int64_t value,
                       std::initializer_list<Attr> attrs = {}) noexcept {
  auto b = detail::encode(attrs);
  loom_metric_i64(name.data(), name.size(), value, b.bytes, b.len);
}
inline void metric_f64(std::string_view name, double value,
                       std::initializer_list<Attr> attrs = {}) noexcept {
  auto b = detail::encode(attrs);
  loom_metric_f64(name.data(), name.size(), value, b.bytes, b.len);
}
inline void counter_inc(std::string_view name, int64_t delta = 1,
                        std::initializer_list<Attr> attrs = {}) noexcept {
  auto b = detail::encode(attrs);
  loom_counter_inc(name.data(), name.size(), delta, b.bytes, b.len);
}

// ── Errors ────────────────────────────────────────────────────────────────
enum class Severity : int { Warn = 0, Error = 1, Fatal = 2 };

inline void error(std::string_view name,
                  std::string_view message,
                  Severity sev = Severity::Error,
                  std::initializer_list<Attr> attrs = {}) noexcept {
  auto b = detail::encode(attrs);
  loom_error(name.data(),    name.size(),
             message.data(), message.size(),
             static_cast<int>(sev),
             b.bytes, b.len);
}

// ── Audit ─────────────────────────────────────────────────────────────────
struct AuditOptions { bool async = false; };
inline void audit(std::string_view name,
                  std::initializer_list<Attr> attrs,
                  AuditOptions opts = {}) noexcept {
  auto b = detail::encode(attrs);
  loom_audit(name.data(), name.size(), b.bytes, b.len, opts.async ? 0 : 1);
}

// ── Lifecycle ─────────────────────────────────────────────────────────────
inline void lifecycle(std::string_view marker,
                      std::initializer_list<Attr> attrs = {}) noexcept {
  auto b = detail::encode(attrs);
  loom_lifecycle(marker.data(), marker.size(), b.bytes, b.len);
}

}  // namespace loom
