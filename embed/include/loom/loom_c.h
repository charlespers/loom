// loom/loom_c.h — flat C ABI. Canonical entry points for FFI consumers
// (Python ctypes, Rust bindgen, Go cgo) and the C++ wrappers in <loom.h>.
//
// Wire schema: loom.event.v1.
// Attribute payload format (binary, little-endian):
//
//     uint32  magic   = 0xA77A1234
//     uint32  count
//     repeated count times:
//       uint16  key_len
//       byte    key[key_len]
//       uint8   type    // 0=i64, 1=f64, 2=str, 3=bool, 4=bytes
//       payload:
//         i64:   int64
//         f64:   float64
//         str:   uint32 val_len; byte val[val_len]
//         bool:  uint8 (0|1)
//         bytes: uint32 val_len; byte val[val_len]
//
// Producers build the buffer with loom_attrs_* into a stack array; the
// embed lib decodes it once per emit call. The fixed magic is the
// no-op sentinel: a zero-initialized buffer fails parse cleanly.
#pragma once
#include <stdint.h>
#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

// ── Lifecycle of the embed lib in this process ──────────────────────────
int  loom_init(void);
void loom_shutdown(void);
int  loom_active(void);

// ── Spans ───────────────────────────────────────────────────────────────
// span_begin returns a handle (== span_id, opaque uint64; 0 means inactive).
// span_end pairs with begin via the handle and emits one event with
// dur_ns set; per-name span stats accumulate for the run summary.
// Parents are tracked per-thread: the parent of a new span is the most
// recent unbalanced span_begin on the same thread.
uint64_t loom_span_begin(const char* name, size_t name_len,
                         const uint8_t* attrs, size_t attrs_len);
void     loom_span_end  (uint64_t handle);
void     loom_span_annotate(uint64_t handle,
                            const uint8_t* attrs, size_t attrs_len);

// ── Metrics ─────────────────────────────────────────────────────────────
// Every metric carries a value AND optional attributes. value is emitted
// directly into the JSON event; attrs are decoded and merged.
void loom_metric_i64 (const char* name, size_t name_len, int64_t value,
                      const uint8_t* attrs, size_t attrs_len);
void loom_metric_f64 (const char* name, size_t name_len, double  value,
                      const uint8_t* attrs, size_t attrs_len);
void loom_counter_inc(const char* name, size_t name_len, int64_t delta,
                      const uint8_t* attrs, size_t attrs_len);

// ── Errors ──────────────────────────────────────────────────────────────
// severity: 0 = warn, 1 = error, 2 = fatal. Errors are kept in memory
// and rendered into the run summary in the order they arrived.
void loom_error(const char* name,    size_t name_len,
                const char* message, size_t message_len,
                int severity,
                const uint8_t* attrs, size_t attrs_len);

// ── Audit ───────────────────────────────────────────────────────────────
// Audit events go to two files: audit.jsonl (private, mode 0600, full
// attrs) and audit.public.jsonl (redacted in M2-partial: attrs are
// dropped; M3 introduces field-level @public). Both files carry a
// SHA-256 hash chain so any tampering breaks verification.
//
// sync != 0 ⇒ flush + fsync before returning.
void loom_audit(const char* name, size_t name_len,
                const uint8_t* attrs, size_t attrs_len,
                int sync);

// ── Lifecycle markers ───────────────────────────────────────────────────
// Sparse named anchors that drive the navigation outline of the run
// summary, the live TUI, and report.html.
void loom_lifecycle(const char* marker, size_t marker_len,
                    const uint8_t* attrs, size_t attrs_len);

// ── Attribute encoding helpers ──────────────────────────────────────────
// loom_attrs_begin writes the magic header into buf and returns the
// initial offset (8 bytes). Each loom_attrs_<type> appends one attribute
// at the given offset and returns the new offset. Producers chain calls,
// passing the previous return value. On overflow the helpers leave the
// buffer in its prior state and return the input offset unchanged.
//
// To attach the buffer to an event, pass `(buf, returned_offset)` as
// `(attrs, attrs_len)` to the appropriate emit function. Pass `(NULL, 0)`
// for an event with no attributes.
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
size_t loom_attrs_bytes(uint8_t* buf, size_t buf_cap, size_t off,
                        const char* key, size_t key_len,
                        const uint8_t* val, size_t val_len);

#ifdef __cplusplus
}
#endif
