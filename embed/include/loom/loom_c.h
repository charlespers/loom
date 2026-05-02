// loom/loom_c.h — flat C ABI. Canonical entry points.
#pragma once
#include <stdint.h>
#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

// ── Lifecycle of the embed lib in this process ──────────────────────────
int  loom_init(void);
void loom_shutdown(void);
int  loom_active(void);   // 1 if active, 0 if no-op mode

// ── Spans ───────────────────────────────────────────────────────────────
// Returns a span handle (opaque uint64; 0 means inactive). Caller passes the
// same handle to span_end. Attribute payload is a length-prefixed
// flat-buffer-style blob produced by loom_attrs_*; see attrs section.
uint64_t loom_span_begin(const char* name, size_t name_len,
                         const uint8_t* attrs, size_t attrs_len);
void     loom_span_end  (uint64_t handle);
void     loom_span_annotate(uint64_t handle,
                            const char* key, size_t key_len,
                            const uint8_t* attrs, size_t attrs_len);

// ── Metrics ─────────────────────────────────────────────────────────────
void loom_metric_i64 (const char* name, size_t name_len, int64_t value);
void loom_metric_f64 (const char* name, size_t name_len, double  value);
void loom_counter_inc(const char* name, size_t name_len, int64_t delta);

// ── Errors ──────────────────────────────────────────────────────────────
// severity: 0 = warn, 1 = error, 2 = fatal.
void loom_error(const char* name,    size_t name_len,
                const char* message, size_t message_len,
                int severity,
                const uint8_t* attrs, size_t attrs_len);

// ── Audit ───────────────────────────────────────────────────────────────
// sync != 0 ⇒ block until daemon fsyncs the record.
void loom_audit(const char* name, size_t name_len,
                const uint8_t* attrs, size_t attrs_len,
                int sync);

// ── Lifecycle markers ───────────────────────────────────────────────────
void loom_lifecycle(const char* marker, size_t marker_len,
                    const uint8_t* attrs, size_t attrs_len);

// ── Attribute encoding helpers ──────────────────────────────────────────
// In M1 these are declared but not implemented; M2 will implement the
// flat encoding. The C++ wrapper still calls them so the API shape is
// pinned now.
size_t loom_attrs_begin(uint8_t* buf, size_t buf_cap);
size_t loom_attrs_i64  (uint8_t* buf, size_t buf_cap, size_t off,
                        const char* key, size_t key_len, int64_t v);
size_t loom_attrs_f64  (uint8_t* buf, size_t buf_cap, size_t off,
                        const char* key, size_t key_len, double  v);
size_t loom_attrs_str  (uint8_t* buf, size_t buf_cap, size_t off,
                        const char* key, size_t key_len,
                        const char* val, size_t val_len);
size_t loom_attrs_bool (uint8_t* buf, size_t buf_cap, size_t off,
                        const char* key, size_t key_len, int v);

#ifdef __cplusplus
}
#endif
