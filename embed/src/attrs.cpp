// embed/src/attrs.cpp — binary attribute encoder + JSON decoder.
//
// The on-the-wire format is documented in loom_c.h. This file implements
// both sides: the loom_attrs_* C entry points that producers call to build
// a buffer, and append_attrs_json() that the embed lib uses when writing
// canonical JSON to events.jsonl.
#include "loom/loom_c.h"
#include "attrs.h"

#include <cstdio>
#include <cstring>
#include <string>

namespace {

// Type tags — must match the C ABI documentation in loom_c.h.
constexpr uint8_t TAG_I64   = 0;
constexpr uint8_t TAG_F64   = 1;
constexpr uint8_t TAG_STR   = 2;
constexpr uint8_t TAG_BOOL  = 3;
constexpr uint8_t TAG_BYTES = 4;

inline void put_u32_le(uint8_t* p, uint32_t v) {
  p[0] = uint8_t(v);
  p[1] = uint8_t(v >> 8);
  p[2] = uint8_t(v >> 16);
  p[3] = uint8_t(v >> 24);
}
inline uint32_t get_u32_le(const uint8_t* p) {
  return uint32_t(p[0]) | (uint32_t(p[1]) << 8) |
         (uint32_t(p[2]) << 16) | (uint32_t(p[3]) << 24);
}
inline void put_u16_le(uint8_t* p, uint16_t v) {
  p[0] = uint8_t(v);
  p[1] = uint8_t(v >> 8);
}
inline uint16_t get_u16_le(const uint8_t* p) {
  return static_cast<uint16_t>(uint16_t(p[0]) |
                               static_cast<uint16_t>(uint16_t(p[1]) << 8));
}
inline void put_i64_le(uint8_t* p, int64_t v) {
  uint64_t u = static_cast<uint64_t>(v);
  for (int i = 0; i < 8; i++) p[i] = uint8_t(u >> (i * 8));
}
inline int64_t get_i64_le(const uint8_t* p) {
  uint64_t u = 0;
  for (int i = 0; i < 8; i++) u |= uint64_t(p[i]) << (i * 8);
  return static_cast<int64_t>(u);
}
inline void put_f64_le(uint8_t* p, double v) {
  uint64_t u; std::memcpy(&u, &v, 8);
  for (int i = 0; i < 8; i++) p[i] = uint8_t(u >> (i * 8));
}
inline double get_f64_le(const uint8_t* p) {
  uint64_t u = 0;
  for (int i = 0; i < 8; i++) u |= uint64_t(p[i]) << (i * 8);
  double v; std::memcpy(&v, &u, 8);
  return v;
}

// Increment the count at offset 4. Caller has already bumped offset for
// the new entry.
inline void inc_count(uint8_t* buf) {
  uint32_t c = get_u32_le(buf + 4);
  put_u32_le(buf + 4, c + 1);
}

}  // namespace

extern "C" size_t loom_attrs_begin(uint8_t* buf, size_t buf_cap) {
  if (buf_cap < 8) return 0;
  put_u32_le(buf,     loom::detail::kAttrsMagic);
  put_u32_le(buf + 4, 0u);
  return 8;
}

extern "C" size_t loom_attrs_i64(uint8_t* buf, size_t buf_cap, size_t off,
                                 const char* key, size_t key_len, int64_t v) {
  if (off < 8 || key_len > 0xFFFF) return off;
  size_t need = 2 + key_len + 1 + 8;
  if (off + need > buf_cap) return off;
  put_u16_le(buf + off, static_cast<uint16_t>(key_len));   off += 2;
  std::memcpy(buf + off, key, key_len);                    off += key_len;
  buf[off++] = TAG_I64;
  put_i64_le(buf + off, v);                                off += 8;
  inc_count(buf);
  return off;
}

extern "C" size_t loom_attrs_f64(uint8_t* buf, size_t buf_cap, size_t off,
                                 const char* key, size_t key_len, double v) {
  if (off < 8 || key_len > 0xFFFF) return off;
  size_t need = 2 + key_len + 1 + 8;
  if (off + need > buf_cap) return off;
  put_u16_le(buf + off, static_cast<uint16_t>(key_len));   off += 2;
  std::memcpy(buf + off, key, key_len);                    off += key_len;
  buf[off++] = TAG_F64;
  put_f64_le(buf + off, v);                                off += 8;
  inc_count(buf);
  return off;
}

extern "C" size_t loom_attrs_str(uint8_t* buf, size_t buf_cap, size_t off,
                                 const char* key, size_t key_len,
                                 const char* val, size_t val_len) {
  if (off < 8 || key_len > 0xFFFF || val_len > 0xFFFFFFFFu) return off;
  size_t need = 2 + key_len + 1 + 4 + val_len;
  if (off + need > buf_cap) return off;
  put_u16_le(buf + off, static_cast<uint16_t>(key_len));   off += 2;
  std::memcpy(buf + off, key, key_len);                    off += key_len;
  buf[off++] = TAG_STR;
  put_u32_le(buf + off, static_cast<uint32_t>(val_len));   off += 4;
  std::memcpy(buf + off, val, val_len);                    off += val_len;
  inc_count(buf);
  return off;
}

extern "C" size_t loom_attrs_bool(uint8_t* buf, size_t buf_cap, size_t off,
                                  const char* key, size_t key_len, int v) {
  if (off < 8 || key_len > 0xFFFF) return off;
  size_t need = 2 + key_len + 1 + 1;
  if (off + need > buf_cap) return off;
  put_u16_le(buf + off, static_cast<uint16_t>(key_len));   off += 2;
  std::memcpy(buf + off, key, key_len);                    off += key_len;
  buf[off++] = TAG_BOOL;
  buf[off++] = (v != 0) ? uint8_t{1} : uint8_t{0};
  inc_count(buf);
  return off;
}

extern "C" size_t loom_attrs_bytes(uint8_t* buf, size_t buf_cap, size_t off,
                                   const char* key, size_t key_len,
                                   const uint8_t* val, size_t val_len) {
  if (off < 8 || key_len > 0xFFFF || val_len > 0xFFFFFFFFu) return off;
  size_t need = 2 + key_len + 1 + 4 + val_len;
  if (off + need > buf_cap) return off;
  put_u16_le(buf + off, static_cast<uint16_t>(key_len));   off += 2;
  std::memcpy(buf + off, key, key_len);                    off += key_len;
  buf[off++] = TAG_BYTES;
  put_u32_le(buf + off, static_cast<uint32_t>(val_len));   off += 4;
  std::memcpy(buf + off, val, val_len);                    off += val_len;
  inc_count(buf);
  return off;
}

namespace loom::detail {

void append_json_string(const char* s, size_t n, std::string& out) {
  out.push_back('"');
  for (size_t i = 0; i < n; i++) {
    unsigned char c = static_cast<unsigned char>(s[i]);
    switch (c) {
      case '"':  out += "\\\""; break;
      case '\\': out += "\\\\"; break;
      case '\n': out += "\\n";  break;
      case '\r': out += "\\r";  break;
      case '\t': out += "\\t";  break;
      case '\b': out += "\\b";  break;
      case '\f': out += "\\f";  break;
      default:
        if (c < 0x20) {
          char buf[8];
          std::snprintf(buf, sizeof(buf), "\\u%04x", c);
          out += buf;
        } else {
          out.push_back(static_cast<char>(c));
        }
        break;
    }
  }
  out.push_back('"');
}

void append_json_string(const std::string& s, std::string& out) {
  append_json_string(s.data(), s.size(), out);
}

// Base64 (no-padding-saving variant; standard alphabet) for byte attrs.
namespace {
const char kB64[] = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
void append_base64(const uint8_t* data, size_t n, std::string& out) {
  for (size_t i = 0; i + 3 <= n; i += 3) {
    uint32_t v = (uint32_t(data[i]) << 16) |
                 (uint32_t(data[i + 1]) << 8) |
                 (uint32_t(data[i + 2]));
    out.push_back(kB64[(v >> 18) & 0x3F]);
    out.push_back(kB64[(v >> 12) & 0x3F]);
    out.push_back(kB64[(v >> 6)  & 0x3F]);
    out.push_back(kB64[v & 0x3F]);
  }
  size_t rem = n % 3;
  if (rem == 1) {
    uint32_t v = uint32_t(data[n - 1]) << 16;
    out.push_back(kB64[(v >> 18) & 0x3F]);
    out.push_back(kB64[(v >> 12) & 0x3F]);
    out += "==";
  } else if (rem == 2) {
    uint32_t v = (uint32_t(data[n - 2]) << 16) | (uint32_t(data[n - 1]) << 8);
    out.push_back(kB64[(v >> 18) & 0x3F]);
    out.push_back(kB64[(v >> 12) & 0x3F]);
    out.push_back(kB64[(v >> 6)  & 0x3F]);
    out += '=';
  }
}
}  // namespace

size_t append_attrs_json(const uint8_t* buf, size_t len, std::string& out) {
  if (buf == nullptr || len < 8) return 0;
  if (get_u32_le(buf) != kAttrsMagic) return 0;
  uint32_t count = get_u32_le(buf + 4);
  size_t off = 8;
  size_t emitted = 0;
  for (uint32_t i = 0; i < count; i++) {
    if (off + 3 > len) break;  // need at least key_len + tag
    uint16_t key_len = get_u16_le(buf + off); off += 2;
    if (off + key_len + 1 > len) break;
    const char* key = reinterpret_cast<const char*>(buf + off);
    off += key_len;
    uint8_t tag = buf[off++];

    if (emitted > 0) out.push_back(',');
    append_json_string(key, key_len, out);
    out.push_back(':');

    switch (tag) {
      case TAG_I64: {
        if (off + 8 > len) return emitted;
        int64_t v = get_i64_le(buf + off); off += 8;
        char tmp[32];
        std::snprintf(tmp, sizeof(tmp), "%lld", static_cast<long long>(v));
        out += tmp;
        break;
      }
      case TAG_F64: {
        if (off + 8 > len) return emitted;
        double v = get_f64_le(buf + off); off += 8;
        char tmp[32];
        // %.10g matches the convention pinned across the project, including
        // bedrock.bench.v1, so JSON parsers that already accept it work
        // unmodified.
        std::snprintf(tmp, sizeof(tmp), "%.10g", v);
        out += tmp;
        break;
      }
      case TAG_STR: {
        if (off + 4 > len) return emitted;
        uint32_t val_len = get_u32_le(buf + off); off += 4;
        if (off + val_len > len) return emitted;
        append_json_string(reinterpret_cast<const char*>(buf + off),
                           val_len, out);
        off += val_len;
        break;
      }
      case TAG_BOOL: {
        if (off + 1 > len) return emitted;
        uint8_t v = buf[off++];
        out += (v ? "true" : "false");
        break;
      }
      case TAG_BYTES: {
        if (off + 4 > len) return emitted;
        uint32_t val_len = get_u32_le(buf + off); off += 4;
        if (off + val_len > len) return emitted;
        out.push_back('"');
        append_base64(buf + off, val_len, out);
        out.push_back('"');
        off += val_len;
        break;
      }
      default:
        // Unknown tag — abort decoding.
        return emitted;
    }
    emitted++;
  }
  return emitted;
}

}  // namespace loom::detail
