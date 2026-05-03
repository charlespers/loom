// embed/src/attrs.h — internal helpers for building and decoding the
// binary attribute payload defined in loom_c.h.
//
// The encoder side (loom_attrs_*) is in attrs.cpp; the decoder side is
// also there because both sides share validation logic for the magic
// header and bounds checks.
#pragma once

#include <cstddef>
#include <cstdint>
#include <string>

namespace loom::detail {

// Magic at offset 0 of every well-formed attrs buffer.
constexpr uint32_t kAttrsMagic = 0xA77A1234u;

// Append the decoded contents of `(buf, len)` as JSON object members onto
// `out`. The output is *not* wrapped in braces — caller emits "{...}".
// Sample output (no leading or trailing braces):
//     "layer":8,"variant":"qkv_tri_fused","alpha":0.087,"is_fused":true
//
// On a malformed or empty buffer, leaves `out` unchanged. Returns the
// number of attributes actually decoded.
size_t append_attrs_json(const uint8_t* buf, size_t len, std::string& out);

// Like append_attrs_json, but emits only keys whose name ends in the
// "@public" suffix, with the suffix stripped. This is the redaction
// rule used by audit.public.jsonl: producers tag fields that are safe
// to share by suffixing the key name. A field both `path` and
// `path@public` may be present in the same call — `path` stays only
// in the private file, `path@public` becomes `path` in the public.
size_t append_attrs_json_public(const uint8_t* buf, size_t len, std::string& out);

// JSON-escape a string into `out`. Safe for arbitrary UTF-8.
void append_json_string(const char* s, size_t n, std::string& out);
void append_json_string(const std::string& s, std::string& out);

}  // namespace loom::detail
