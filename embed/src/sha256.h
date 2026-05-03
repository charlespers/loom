// embed/src/sha256.h — SHA-256 (FIPS 180-4) for the audit chain.
// Vendored implementation; no external crypto dependency.
#pragma once

#include <cstddef>
#include <cstdint>
#include <string>

namespace loom::detail {

// Compute SHA-256 over `data[0..len)`, write 32 bytes to `out`.
void sha256(const void* data, size_t len, uint8_t out[32]);

// Lowercase hex (64 chars, no separators, no prefix).
std::string sha256_hex(const void* data, size_t len);

// Streaming variant for chained hashing without joining buffers.
class Sha256 {
 public:
  Sha256();
  void update(const void* data, size_t len);
  void finalize(uint8_t out[32]);
  std::string finalize_hex();
 private:
  uint32_t h_[8];
  uint8_t  buf_[64];
  size_t   buf_len_ = 0;
  uint64_t bit_len_ = 0;
};

}  // namespace loom::detail
