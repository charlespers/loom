# Loom M1 — Skeleton Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up the Loom repo skeleton so `loom run -- echo hi` works end-to-end on a clean checkout, the public C++/C header API surface from the spec is pinned as compilable headers, and CI builds successfully on Linux + macOS.

**Architecture:** Three-component layout (C++ embed lib, Go daemon, Go CLI) wired into a single repo. Embed lib is a static archive built by CMake with skeleton no-op bodies. Daemon and CLI live in a Go workspace. Cobra drives CLI subcommand routing; only `loom run` and `loom version` have real bodies in M1 — every other subcommand is stubbed with a clear "not implemented in M1" message. Run-id allocation, artifact-dir creation, and child-process exec are implemented; ring buffer, daemon launch, and event emission are deferred to M2. End-to-end smoke test in CI is the acceptance gate.

**Tech Stack:** C++17, CMake ≥ 3.20, Go 1.21, [Cobra](https://github.com/spf13/cobra) for CLI, [oklog/ulid/v2](https://github.com/oklog/ulid) for run-ids, GoogleTest (added but unused in M1), GitHub Actions CI on `ubuntu-22.04` + `macos-14`.

**Reference spec:** `docs/design/2026-05-02-loom-design.md` (see § 5 for embed lib API, § 7 for CLI surface, § 10 for repo layout).

---

## File Structure

Files created or modified by this plan:

| Path | Purpose |
|---|---|
| `LICENSE` | Apache-2.0 license text. |
| `.gitignore` | Build artifacts, OS junk, IDE folders. |
| `README.md` | Designed README — vision, quickstart, install, status badge. M1 ships a minimal but designed version; later milestones expand. |
| `CHANGELOG.md` | Standard "Keep a Changelog" header; M1 entry. |
| `CMakeLists.txt` | Top-level CMake. Defines project, C++17, sub-includes `embed/`. |
| `embed/CMakeLists.txt` | Builds static archive `liblooom.a` from `embed/src/*.cpp`. |
| `embed/include/loom/loom.h` | Public C++ API (skeleton, declarations only matching spec § 5.1). |
| `embed/include/loom/loom_c.h` | C ABI mirror (declarations only). |
| `embed/include/loom/loom_hooks.h` | Weak-symbol no-op header for non-linking consumers. |
| `embed/include/loom/version.h` | Generated header with `LOOM_VERSION_*` macros. |
| `embed/src/loom.cpp` | Implements the C ABI as no-ops; the C++ wrappers (in `loom.h`) inline-call into these. |
| `embed/tests/CMakeLists.txt` | GoogleTest harness wired up; one smoke test that links against `liblooom.a`. |
| `embed/tests/test_smoke.cpp` | Single test: `loom::active()` returns false when env vars unset. |
| `go.work` | Workspace pulling `daemon/` and `cli/` modules. |
| `daemon/go.mod` | Daemon Go module. |
| `daemon/cmd/loom-daemon/main.go` | Daemon entry; in M1, parses `--run-id` + `--ring-path`, prints a banner, sleeps until SIGTERM. |
| `cli/go.mod` | CLI Go module. |
| `cli/cmd/loom/main.go` | CLI entry; wires up Cobra root command. |
| `cli/internal/version/version.go` | Version constants and `loom version` body. |
| `cli/internal/run/run.go` | `loom run` body: allocates run-id (ULID), creates artifact dir, execs child. M2 will add daemon launch. |
| `cli/internal/stub/stub.go` | Helper for the "not implemented in M1" stub bodies of `watch`, `view`, `report`, `verify`, `redact`, `doctor`. |
| `examples/cpp_minimal/CMakeLists.txt` | Builds a tiny program that links `liblooom.a` and calls `loom::init()` then `loom::Span("hi"); loom::shutdown()`. |
| `examples/cpp_minimal/main.cpp` | The example. |
| `tools/smoke.sh` | End-to-end smoke test invoked by CI: configures, builds, runs `loom run -- ./cpp_minimal`, asserts run dir exists with the expected files. |
| `.github/workflows/ci.yml` | Matrix CI (ubuntu-22.04, macos-14) running CMake build + Go build + smoke. |
| `Makefile` | Convenience targets: `make build`, `make test`, `make smoke`, `make clean`. |

Notes on decomposition:

- `loom.h` is the C++ surface; `loom_c.h` is the canonical implementation surface (per spec § 5.2 self-review fix). The C++ wrappers in `loom.h` are inline shims that build attribute payloads and call the C entry points in `loom.cpp`. M1 ships the C ABI as no-ops; M2 implements the ring writer behind them.
- `cli/internal/run/run.go` is intentionally separated from `main.go` so M2 can extend it (daemon fork, ring setup) without touching the Cobra wiring.
- `examples/cpp_minimal/` exists in M1 specifically so the smoke test has something real to run; in later milestones it doubles as user-facing example code.

---

## Tasks

### Task 1: Bootstrap repo with LICENSE, .gitignore, CHANGELOG

**Files:**
- Create: `LICENSE`
- Create: `.gitignore`
- Create: `CHANGELOG.md`

- [ ] **Step 1: Create the Apache-2.0 LICENSE file**

Write the standard Apache License 2.0 text (year 2026, copyright holder "Charles Mahoney") to `LICENSE`. Source the canonical text from `https://www.apache.org/licenses/LICENSE-2.0.txt`. Replace `[yyyy]` with `2026` and `[name of copyright owner]` with `Charles Mahoney`.

- [ ] **Step 2: Create `.gitignore`**

```gitignore
# Build artifacts
build/
build-*/
cmake-build-*/
*.o
*.a
*.so
*.dylib

# Go
/daemon/loom-daemon
/cli/loom

# Examples
/examples/cpp_minimal/build/
/examples/cpp_minimal/cpp_minimal

# Loom runtime artifacts
/tmp/loom/
~/.loom/runs/

# OS / editor
.DS_Store
.idea/
.vscode/
*.swp

# CMake
CMakeCache.txt
CMakeFiles/
cmake_install.cmake
Makefile
!Makefile

# Coverage
*.gcda
*.gcno
coverage/
```

- [ ] **Step 3: Create `CHANGELOG.md`**

```markdown
# Changelog

All notable changes to Loom are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/). This project
follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- M1 skeleton: repo layout, public C++/C header API surface (declarations
  only, no-op bodies), Go workspace with daemon and CLI skeletons, end-to-end
  smoke test, CI on Linux and macOS.
```

- [ ] **Step 4: Commit**

```bash
git add LICENSE .gitignore CHANGELOG.md
git commit -m "chore: license, gitignore, changelog"
```

---

### Task 2: Top-level CMakeLists.txt + embed lib skeleton

**Files:**
- Create: `CMakeLists.txt`
- Create: `embed/CMakeLists.txt`
- Create: `embed/src/loom.cpp` (placeholder; replaced in Task 4)

- [ ] **Step 1: Write the failing test (CMake configure smoke)**

Create `tools/test_cmake_configures.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
rm -rf build
cmake -S . -B build -DCMAKE_BUILD_TYPE=Debug >/dev/null
test -f build/CMakeCache.txt
echo "OK: cmake configured"
```

Make it executable: `chmod +x tools/test_cmake_configures.sh`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `./tools/test_cmake_configures.sh`
Expected: FAIL — `CMakeLists.txt` does not exist.

- [ ] **Step 3: Write top-level `CMakeLists.txt`**

```cmake
cmake_minimum_required(VERSION 3.20)
project(loom
        VERSION 0.1.0
        DESCRIPTION "Observability harness for local-compute AI systems"
        LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)
set(CMAKE_POSITION_INDEPENDENT_CODE ON)

if(NOT CMAKE_BUILD_TYPE)
  set(CMAKE_BUILD_TYPE Release)
endif()

option(LOOM_BUILD_TESTS "Build embed-lib unit tests" ON)
option(LOOM_BUILD_EXAMPLES "Build example programs" ON)

add_subdirectory(embed)

if(LOOM_BUILD_EXAMPLES)
  add_subdirectory(examples/cpp_minimal)
endif()
```

- [ ] **Step 4: Write `embed/CMakeLists.txt`**

```cmake
add_library(loom STATIC
  src/loom.cpp
)

target_include_directories(loom
  PUBLIC
    $<BUILD_INTERFACE:${CMAKE_CURRENT_SOURCE_DIR}/include>
    $<INSTALL_INTERFACE:include>
)

target_compile_options(loom PRIVATE
  -Wall -Wextra -Wpedantic -Wshadow -Wconversion
)

set_target_properties(loom PROPERTIES
  OUTPUT_NAME "loom"
)

if(LOOM_BUILD_TESTS)
  enable_testing()
  add_subdirectory(tests)
endif()
```

- [ ] **Step 5: Create placeholder `embed/src/loom.cpp`**

```cpp
// Placeholder body. Replaced in Task 4.
extern "C" int loom_init(void) { return 0; }
```

- [ ] **Step 6: Run the test to verify it passes**

Run: `./tools/test_cmake_configures.sh`
Expected: PASS — `OK: cmake configured`.

- [ ] **Step 7: Verify the embed lib actually builds**

Run: `cmake --build build --target loom -j`
Expected: builds `build/embed/libloom.a` (or equivalent for the platform).

- [ ] **Step 8: Commit**

```bash
git add CMakeLists.txt embed/CMakeLists.txt embed/src/loom.cpp tools/test_cmake_configures.sh
git commit -m "build(cmake): top-level + embed lib skeleton"
```

---

### Task 3: Embed lib public headers (declarations only)

**Files:**
- Create: `embed/include/loom/version.h`
- Create: `embed/include/loom/loom_c.h`
- Create: `embed/include/loom/loom.h`
- Create: `embed/include/loom/loom_hooks.h`

- [ ] **Step 1: Write `embed/include/loom/version.h`**

```cpp
// loom/version.h — version macros for the embed lib.
#pragma once

#define LOOM_VERSION_MAJOR 0
#define LOOM_VERSION_MINOR 1
#define LOOM_VERSION_PATCH 0
#define LOOM_VERSION_STRING "0.1.0"

// Wire schema version emitted by this embed lib.
#define LOOM_WIRE_SCHEMA "loom.event.v1"
```

- [ ] **Step 2: Write `embed/include/loom/loom_c.h`**

This is the canonical C ABI per spec § 5.2. Every category has a flat C entry
point; the C++ wrappers in `loom.h` will inline-dispatch to these.

```c
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
```

- [ ] **Step 3: Write `embed/include/loom/loom.h`**

```cpp
// loom/loom.h — public C++ API (C++17). Inline shims over loom_c.h.
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
  enum class Tag : uint8_t { I64, F64, Str, Bool };
  Tag tag;
  union U {
    int64_t i64;
    double  f64;
    bool    b;
    struct { const char* data; size_t len; } str;
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
};

struct Attr {
  std::string_view key;
  AttrValue        value;
};

inline int  init()       noexcept { return loom_init(); }
inline void shutdown()   noexcept {        loom_shutdown(); }
inline bool active()     noexcept { return loom_active() != 0; }

// Attribute encoding into a stack buffer. M1: stub; encoding pinned in M2.
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
    }
  }
  return b;
}
}  // namespace detail

class Span {
 public:
  explicit Span(std::string_view name) noexcept {
    handle_ = loom_span_begin(name.data(), name.size(), nullptr, 0);
  }
  Span(std::string_view name, std::initializer_list<Attr> attrs) noexcept {
    auto b  = detail::encode(attrs);
    handle_ = loom_span_begin(name.data(), name.size(), b.bytes, b.len);
  }
  ~Span() {
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
  void annotate(std::string_view key, AttrValue value) noexcept {
    Attr a{key, value};
    auto b = detail::encode({a});
    loom_span_annotate(handle_, key.data(), key.size(), b.bytes, b.len);
  }
 private:
  uint64_t handle_ = 0;
};

inline void metric_i64(std::string_view name, int64_t value) noexcept {
  loom_metric_i64(name.data(), name.size(), value);
}
inline void metric_f64(std::string_view name, double value) noexcept {
  loom_metric_f64(name.data(), name.size(), value);
}
inline void counter_inc(std::string_view name, int64_t delta = 1) noexcept {
  loom_counter_inc(name.data(), name.size(), delta);
}

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

struct AuditOptions { bool async = false; };
inline void audit(std::string_view name,
                  std::initializer_list<Attr> attrs,
                  AuditOptions opts = {}) noexcept {
  auto b = detail::encode(attrs);
  loom_audit(name.data(), name.size(), b.bytes, b.len, opts.async ? 0 : 1);
}

inline void lifecycle(std::string_view marker,
                      std::initializer_list<Attr> attrs = {}) noexcept {
  auto b = detail::encode(attrs);
  loom_lifecycle(marker.data(), marker.size(), b.bytes, b.len);
}

}  // namespace loom
```

- [ ] **Step 4: Write `embed/include/loom/loom_hooks.h`**

```c
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
#pragma once

#ifdef __cplusplus
extern "C" {
#endif

#if defined(__GNUC__) || defined(__clang__)
  #define LOOM_WEAK __attribute__((weak))
#else
  #error "loom_hooks.h requires gcc or clang weak-symbol support"
#endif

LOOM_WEAK void loom_hook_span_begin(const char* name);
LOOM_WEAK void loom_hook_span_end  (const char* name);
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
```

- [ ] **Step 5: Verify headers compile in isolation**

Create a one-liner `tools/test_headers_compile.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
TMP="$(mktemp -d)"
cat > "$TMP/probe.cpp" <<'EOF'
#include <loom/loom.h>
#include <loom/loom_hooks.h>
int main() {
  loom::init();
  { loom::Span s("probe", {{"k", 1}}); }
  loom::metric_f64("rate", 0.5);
  loom::error("oops", "boom", loom::Severity::Warn);
  loom::audit("file.read", {{"path", std::string_view("/x")}});
  loom::lifecycle("run.start");
  LOOM_HOOK(loom_hook_span_begin, "test");
  loom::shutdown();
  return 0;
}
EOF
c++ -std=c++17 -Wall -Wextra -Iembed/include -c -o "$TMP/probe.o" "$TMP/probe.cpp"
echo "OK: headers compile"
```

Run: `chmod +x tools/test_headers_compile.sh && ./tools/test_headers_compile.sh`
Expected: PASS — `OK: headers compile`. (Linking is not attempted; that comes in Task 4.)

- [ ] **Step 6: Commit**

```bash
git add embed/include/loom/version.h embed/include/loom/loom_c.h \
        embed/include/loom/loom.h embed/include/loom/loom_hooks.h \
        tools/test_headers_compile.sh
git commit -m "embed: pin public C/C++ header API surface"
```

---

### Task 4: Embed lib stub bodies

**Files:**
- Modify: `embed/src/loom.cpp` (replace placeholder)
- Create: `embed/src/state.h` (internal-only header)

The C ABI gets real (no-op) bodies that compile and link. M2 will replace the
no-op bodies with ring-writer implementations; for M1, every entry point
returns immediately and the global "active" flag is always `false` until
`LOOM_RUN_ID` env var is observed at `loom_init`.

- [ ] **Step 1: Write `embed/src/state.h`**

```cpp
// embed/src/state.h — internal-only state.
#pragma once
#include <atomic>

namespace loom::detail {
inline std::atomic<bool> g_active{false};
}
```

- [ ] **Step 2: Write `embed/src/loom.cpp`**

```cpp
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
```

- [ ] **Step 3: Build the embed lib**

Run: `cmake --build build --target loom -j`
Expected: builds `build/embed/libloom.a` without warnings.

- [ ] **Step 4: Commit**

```bash
git add embed/src/loom.cpp embed/src/state.h
git commit -m "embed: stub bodies; init/active/shutdown observe LOOM_RUN_ID"
```

---

### Task 5: Embed lib smoke test (GoogleTest)

**Files:**
- Create: `embed/tests/CMakeLists.txt`
- Create: `embed/tests/test_smoke.cpp`

- [ ] **Step 1: Write the failing test `embed/tests/test_smoke.cpp`**

```cpp
#include <loom/loom.h>
#include <gtest/gtest.h>
#include <cstdlib>

TEST(Smoke, InactiveByDefault) {
  unsetenv("LOOM_RUN_ID");
  EXPECT_EQ(loom::init(), 0);
  EXPECT_FALSE(loom::active());
  loom::shutdown();
}

TEST(Smoke, ActivatesWithEnvVar) {
  setenv("LOOM_RUN_ID", "01J3KTV6S5ABCDEFGHJKMNPQRS", 1);
  EXPECT_EQ(loom::init(), 0);
  EXPECT_TRUE(loom::active());
  loom::shutdown();
  EXPECT_FALSE(loom::active());
  unsetenv("LOOM_RUN_ID");
}

TEST(Smoke, CallsAreHarmlessWhenInactive) {
  unsetenv("LOOM_RUN_ID");
  loom::init();
  ASSERT_FALSE(loom::active());
  // None of these may crash; all are no-ops.
  { loom::Span s("probe", {{"k", 1}}); }
  loom::metric_f64("rate", 0.5);
  loom::counter_inc("hits");
  loom::error("oops", "boom", loom::Severity::Warn);
  loom::audit("file.read", {{"path", std::string_view("/x")}});
  loom::lifecycle("run.start");
  loom::shutdown();
  SUCCEED();
}
```

- [ ] **Step 2: Write `embed/tests/CMakeLists.txt`**

```cmake
include(FetchContent)
FetchContent_Declare(
  googletest
  GIT_REPOSITORY https://github.com/google/googletest.git
  GIT_TAG        v1.14.0
)
set(BUILD_GMOCK OFF CACHE BOOL "" FORCE)
set(INSTALL_GTEST OFF CACHE BOOL "" FORCE)
FetchContent_MakeAvailable(googletest)

add_executable(loom_test_smoke test_smoke.cpp)
target_link_libraries(loom_test_smoke
  PRIVATE loom GTest::gtest_main
)

include(GoogleTest)
gtest_discover_tests(loom_test_smoke)
```

- [ ] **Step 3: Configure and build**

Run: `cmake --build build -j`
Expected: builds `build/embed/tests/loom_test_smoke` (FetchContent will pull GoogleTest on first run).

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd build && ctest --output-on-failure && cd ..`
Expected: 3 tests pass.

- [ ] **Step 5: Commit**

```bash
git add embed/tests/CMakeLists.txt embed/tests/test_smoke.cpp
git commit -m "embed(tests): smoke test — inactive by default, activates on env"
```

---

### Task 6: Example consumer `cpp_minimal`

**Files:**
- Create: `examples/cpp_minimal/CMakeLists.txt`
- Create: `examples/cpp_minimal/main.cpp`

- [ ] **Step 1: Write the failing test (smoke run)**

The acceptance test for this task is just running the example and getting a
zero exit code. Create `tools/test_example_runs.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
cmake --build build --target cpp_minimal -j >/dev/null
./build/examples/cpp_minimal/cpp_minimal
echo "OK: example ran"
```

Make it executable and run it:
`chmod +x tools/test_example_runs.sh && ./tools/test_example_runs.sh`
Expected (now): FAIL — target does not exist.

- [ ] **Step 2: Write `examples/cpp_minimal/CMakeLists.txt`**

```cmake
add_executable(cpp_minimal main.cpp)
target_link_libraries(cpp_minimal PRIVATE loom)
```

- [ ] **Step 3: Write `examples/cpp_minimal/main.cpp`**

```cpp
#include <loom/loom.h>
#include <cstdio>

int main(int argc, char** argv) {
  (void)argc; (void)argv;
  loom::init();
  std::printf("loom active=%d\n", loom::active() ? 1 : 0);
  {
    loom::Span s("example.run", {{"step", 1}});
    loom::metric_f64("rate", 0.5);
    loom::lifecycle("done");
  }
  loom::shutdown();
  return 0;
}
```

- [ ] **Step 4: Re-run the smoke test**

Run: `./tools/test_example_runs.sh`
Expected: PASS — `loom active=0` then `OK: example ran`.

- [ ] **Step 5: Commit**

```bash
git add examples/cpp_minimal/ tools/test_example_runs.sh
git commit -m "examples: cpp_minimal — links loom and emits one of each category"
```

---

### Task 7: Go workspace + daemon module

**Files:**
- Create: `go.work`
- Create: `daemon/go.mod`
- Create: `daemon/cmd/loom-daemon/main.go`

- [ ] **Step 1: Initialize the daemon Go module**

```bash
mkdir -p daemon/cmd/loom-daemon
cd daemon
go mod init github.com/charlespers/loom/daemon
cd ..
```

- [ ] **Step 2: Write `daemon/cmd/loom-daemon/main.go`**

```go
// loom-daemon — M1 skeleton. In M1 the daemon is launched but does no
// real work; it parses --run-id and --ring-path, prints a banner on
// stderr, and waits for SIGTERM. M2 implements ring drain and artifact
// writing.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

const Version = "0.1.0"

func main() {
	runID := flag.String("run-id", "", "run id (ULID)")
	ringPath := flag.String("ring-path", "", "path to mmap'd ring file")
	flag.Parse()

	if *runID == "" {
		fmt.Fprintln(os.Stderr, "loom-daemon: --run-id is required")
		os.Exit(2)
	}

	fmt.Fprintf(os.Stderr,
		"loom-daemon v%s · run-id=%s · ring=%s · waiting for SIGTERM\n",
		Version, *runID, *ringPath)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	<-sig

	fmt.Fprintln(os.Stderr, "loom-daemon: shutting down")
}
```

- [ ] **Step 3: Build and smoke-test the daemon**

```bash
cd daemon && go build -o loom-daemon ./cmd/loom-daemon && cd ..
./daemon/loom-daemon --run-id 01J3KTV6S5ABCDEFGHJKMNPQRS --ring-path /tmp/x &
PID=$!
sleep 0.5
kill -TERM "$PID"
wait "$PID"
```

Expected: prints the banner, then "shutting down". Exits 0.

- [ ] **Step 4: Commit (just the daemon piece)**

```bash
git add daemon/
git commit -m "daemon: M1 skeleton — flags, banner, SIGTERM wait"
```

---

### Task 8: CLI module + Cobra wiring

**Files:**
- Create: `cli/go.mod`
- Create: `cli/cmd/loom/main.go`
- Create: `cli/internal/version/version.go`
- Create: `cli/internal/stub/stub.go`

- [ ] **Step 1: Initialize the CLI module**

```bash
mkdir -p cli/cmd/loom cli/internal/version cli/internal/stub
cd cli
go mod init github.com/charlespers/loom/cli
go get github.com/spf13/cobra@v1.8.0
cd ..
```

- [ ] **Step 2: Write `cli/internal/version/version.go`**

```go
// Package version exposes the CLI version constants and the body of the
// `loom version` subcommand.
package version

import (
	"fmt"
	"io"
)

// CLIVersion is the user-facing CLI version. Bumped via release tags.
const CLIVersion = "0.1.0"

// WireSchema is the wire format version this CLI knows how to read.
const WireSchema = "loom.event.v1"

// Print writes a multi-line version block to w.
func Print(w io.Writer) {
	fmt.Fprintf(w, "loom         %s\n", CLIVersion)
	fmt.Fprintf(w, "wire schema  %s\n", WireSchema)
	fmt.Fprintf(w, "daemon       (run `loom doctor` to verify discoverable)\n")
}
```

- [ ] **Step 3: Write `cli/internal/stub/stub.go`**

```go
// Package stub provides standardized "not implemented in M1" handlers for
// CLI subcommands whose real bodies arrive in later milestones.
package stub

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NotImplementedYet returns a RunE that prints a clear deferral notice and
// exits non-zero. The milestone tag (e.g. "M3") tells users when the
// behavior is planned.
func NotImplementedYet(milestone string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		return fmt.Errorf(
			"%s is not implemented in M1 (planned for %s); see docs/design/2026-05-02-loom-design.md § 17",
			cmd.Name(), milestone)
	}
}
```

- [ ] **Step 4: Write `cli/cmd/loom/main.go`**

```go
// loom — M1 CLI. `loom run` and `loom version` are real; every other
// subcommand is a stub that exits non-zero with a deferral message.
package main

import (
	"fmt"
	"os"

	"github.com/charlespers/loom/cli/internal/run"
	"github.com/charlespers/loom/cli/internal/stub"
	"github.com/charlespers/loom/cli/internal/version"
	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "loom",
		Short: "Observability harness for local-compute AI systems",
		Long: "Loom instruments local AI workloads. M1 ships skeleton " +
			"plumbing only; ring buffer and artifacts arrive in M2.",
		SilenceUsage: true,
	}

	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, _ []string) {
			version.Print(cmd.OutOrStdout())
		},
	})

	runCmd := &cobra.Command{
		Use:   "run -- <command> [args...]",
		Short: "Launch a command under the harness",
		Args:  cobra.MinimumNArgs(1),
		RunE:  run.RunE,
	}
	runCmd.Flags().Bool("quiet", false, "suppress the run-id banner")
	root.AddCommand(runCmd)

	root.AddCommand(&cobra.Command{Use: "watch",  Short: "Live TUI",       RunE: stub.NotImplementedYet("M4")})
	root.AddCommand(&cobra.Command{Use: "view",   Short: "Static TUI",     RunE: stub.NotImplementedYet("M4")})
	root.AddCommand(&cobra.Command{Use: "report", Short: "Render report",  RunE: stub.NotImplementedYet("M5")})
	root.AddCommand(&cobra.Command{Use: "verify", Short: "Verify chain",   RunE: stub.NotImplementedYet("M3")})
	root.AddCommand(&cobra.Command{Use: "redact", Short: "Re-run pipe",    RunE: stub.NotImplementedYet("M3")})
	root.AddCommand(&cobra.Command{Use: "doctor", Short: "Env diagnostic", RunE: stub.NotImplementedYet("M7")})

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

- [ ] **Step 5: Write `go.work` at repo root**

```
go 1.21

use (
	./daemon
	./cli
)
```

- [ ] **Step 6: Create empty `cli/internal/run/run.go` placeholder so the import compiles**

```go
package run

import "github.com/spf13/cobra"

// RunE is the body of `loom run`. Real implementation lands in Task 9.
func RunE(_ *cobra.Command, _ []string) error {
	panic("not implemented; replaced in Task 9")
}
```

- [ ] **Step 7: Verify the workspace builds**

Run: `go build ./...`
Expected: builds without errors. Two binaries: `daemon/loom-daemon` and `cli/loom`.

- [ ] **Step 8: Verify `loom version` works**

```bash
./cli/loom version
```

Expected output:
```
loom         0.1.0
wire schema  loom.event.v1
daemon       (run `loom doctor` to verify discoverable)
```

- [ ] **Step 9: Commit**

```bash
git add go.work cli/
git commit -m "cli: cobra root, version, stub subcommands for M3-M7"
```

---

### Task 9: `loom run` minimal implementation

**Files:**
- Modify: `cli/internal/run/run.go` (replace placeholder)

The M1 implementation allocates a run-id (ULID), creates the artifact dir,
sets `LOOM_RUN_ID` and `LOOM_RING_PATH` in the child env, and execs the
child. Daemon launch and ring setup are deferred to M2; in M1 the env vars
are set but unused by the daemon (which is not started yet).

- [ ] **Step 1: Add the ulid dependency**

```bash
cd cli
go get github.com/oklog/ulid/v2@v2.1.0
cd ..
```

- [ ] **Step 2: Write the failing test `cli/internal/run/run_test.go`**

```go
package run

import (
	"os"
	"path/filepath"
	"testing"
)

func TestArtifactDirCreated(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LOOM_HOME", tmp)

	id, dir, err := allocateRun()
	if err != nil {
		t.Fatalf("allocateRun: %v", err)
	}
	if len(id) != 26 {
		t.Fatalf("expected 26-char ULID, got %q (len %d)", id, len(id))
	}
	if filepath.Dir(dir) != filepath.Join(tmp, "runs") {
		t.Fatalf("run dir not under LOOM_HOME/runs: %s", dir)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("run dir not created: %v", err)
	}
}

func TestRingPathDirCreated(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LOOM_TMPDIR", tmp)

	id := "01J3KTV6S5ABCDEFGHJKMNPQRS"
	ring, err := ensureRingDir(id)
	if err != nil {
		t.Fatalf("ensureRingDir: %v", err)
	}
	if filepath.Dir(ring) != filepath.Join(tmp, "loom", id) {
		t.Fatalf("unexpected ring path %s", ring)
	}
	if _, err := os.Stat(filepath.Dir(ring)); err != nil {
		t.Fatalf("ring dir not created: %v", err)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd cli && go test ./internal/run/... -v && cd ..`
Expected: FAIL — `allocateRun`/`ensureRingDir` undefined.

- [ ] **Step 4: Implement `cli/internal/run/run.go`**

```go
// Package run implements `loom run` — allocate a run-id, create the
// artifact directory, set env vars, exec the child. M1 does not launch
// the daemon; that arrives in M2.
package run

import (
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/spf13/cobra"
)

// loomHome returns the artifact root, honoring $LOOM_HOME with a fallback
// to ~/.loom.
func loomHome() (string, error) {
	if h := os.Getenv("LOOM_HOME"); h != "" {
		return h, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".loom"), nil
}

// loomTmpDir returns the ring-file root, honoring $LOOM_TMPDIR with a
// fallback to /tmp.
func loomTmpDir() string {
	if t := os.Getenv("LOOM_TMPDIR"); t != "" {
		return t
	}
	return "/tmp"
}

// allocateRun creates a fresh ULID-named run directory under LOOM_HOME/runs
// and returns the id and the directory path.
func allocateRun() (string, string, error) {
	home, err := loomHome()
	if err != nil {
		return "", "", err
	}
	id := ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader).String()
	dir := filepath.Join(home, "runs", id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", err
	}
	return id, dir, nil
}

// ensureRingDir creates the per-run ring directory and returns the
// expected ring file path. The file itself is not created in M1; M2 will
// mmap it.
func ensureRingDir(runID string) (string, error) {
	dir := filepath.Join(loomTmpDir(), "loom", runID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "ring"), nil
}

// RunE is the body of `loom run`.
func RunE(cmd *cobra.Command, args []string) error {
	quiet, _ := cmd.Flags().GetBool("quiet")

	id, dir, err := allocateRun()
	if err != nil {
		return fmt.Errorf("allocate run dir: %w", err)
	}
	ring, err := ensureRingDir(id)
	if err != nil {
		return fmt.Errorf("ensure ring dir: %w", err)
	}

	if !quiet {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"loom · run %s · artifacts %s\n", id, dir)
	}

	child := exec.Command(args[0], args[1:]...)
	child.Stdin  = os.Stdin
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	child.Env = append(os.Environ(),
		"LOOM_RUN_ID="+id,
		"LOOM_RING_PATH="+ring,
	)

	if err := child.Run(); err != nil {
		// Forward the exit code if the child exited.
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("exec child: %w", err)
	}
	return nil
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `cd cli && go test ./internal/run/... -v && cd ..`
Expected: 2 tests PASS.

- [ ] **Step 6: Manual end-to-end check**

```bash
go build -o cli/loom ./cli/cmd/loom
LOOM_HOME=/tmp/loom-test ./cli/loom run -- echo hello
ls /tmp/loom-test/runs/
```

Expected: prints the run banner + `hello`, exits 0, run dir exists with one ULID-named subdirectory.

- [ ] **Step 7: Commit**

```bash
git add cli/internal/run/ cli/go.mod cli/go.sum
git commit -m "cli(run): allocate ULID, create artifact dir, exec child"
```

---

### Task 10: End-to-end smoke test

**Files:**
- Create: `tools/smoke.sh`

This script is what CI runs and what local devs run before pushing. It builds
everything from scratch and exercises the M1 happy path.

- [ ] **Step 1: Write `tools/smoke.sh`**

```bash
#!/usr/bin/env bash
# tools/smoke.sh — end-to-end M1 acceptance test.
# Builds the embed lib, daemon, CLI; runs `loom run -- ./cpp_minimal`;
# verifies the artifact directory exists and contains the expected env-var
# evidence.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "==> configure cmake"
cmake -S . -B build -DCMAKE_BUILD_TYPE=Debug >/dev/null

echo "==> build C++ targets"
cmake --build build -j

echo "==> run embed-lib unit tests"
(cd build && ctest --output-on-failure)

echo "==> build Go workspace"
go build -o build/loom-daemon ./daemon/cmd/loom-daemon
go build -o build/loom         ./cli/cmd/loom

echo "==> run Go unit tests"
go test ./...

echo "==> exercise loom run -- cpp_minimal"
LOOM_HOME="$(mktemp -d)"
LOOM_TMPDIR="$(mktemp -d)"
export LOOM_HOME LOOM_TMPDIR

OUTPUT="$("$ROOT/build/loom" run -- "$ROOT/build/examples/cpp_minimal/cpp_minimal" 2>&1)"
echo "$OUTPUT"

# Verify a run dir was created.
RUN_COUNT=$(find "$LOOM_HOME/runs" -mindepth 1 -maxdepth 1 -type d | wc -l)
if [ "$RUN_COUNT" -ne 1 ]; then
  echo "FAIL: expected exactly one run dir, found $RUN_COUNT"
  exit 1
fi

# Verify the example saw LOOM_RUN_ID set (loom active=1 in the print).
if ! grep -q "loom active=1" <<<"$OUTPUT"; then
  echo "FAIL: example did not see LOOM_RUN_ID; output was:"
  echo "$OUTPUT"
  exit 1
fi

echo "OK: M1 smoke test passed"
```

Make it executable: `chmod +x tools/smoke.sh`.

- [ ] **Step 2: Run it**

Run: `./tools/smoke.sh`
Expected: terminates with `OK: M1 smoke test passed`.

- [ ] **Step 3: Commit**

```bash
git add tools/smoke.sh
git commit -m "tools: end-to-end M1 smoke (cmake + go + run + verify)"
```

---

### Task 11: Convenience Makefile

**Files:**
- Create: `Makefile`

- [ ] **Step 1: Write the Makefile**

```makefile
# Makefile — Loom developer convenience targets.
# CI does not depend on this; it calls tools/smoke.sh directly.

.PHONY: build test smoke clean configure help

BUILD ?= build

help:
	@echo "Targets:"
	@echo "  configure   cmake configure into ./$(BUILD)"
	@echo "  build       cmake + go build everything"
	@echo "  test        run all unit tests (ctest + go test)"
	@echo "  smoke       end-to-end smoke test (tools/smoke.sh)"
	@echo "  clean       remove ./$(BUILD) and Go binaries"

configure:
	cmake -S . -B $(BUILD) -DCMAKE_BUILD_TYPE=Debug

build: configure
	cmake --build $(BUILD) -j
	go build -o $(BUILD)/loom-daemon ./daemon/cmd/loom-daemon
	go build -o $(BUILD)/loom         ./cli/cmd/loom

test: build
	cd $(BUILD) && ctest --output-on-failure
	go test ./...

smoke:
	./tools/smoke.sh

clean:
	rm -rf $(BUILD) cli/loom daemon/loom-daemon
```

- [ ] **Step 2: Verify each target**

```bash
make clean
make configure
make build
make test
make smoke
```

Expected: each target succeeds; `make smoke` prints `OK: M1 smoke test passed`.

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "build: Makefile convenience targets (configure/build/test/smoke)"
```

---

### Task 12: GitHub Actions CI

**Files:**
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Write `.github/workflows/ci.yml`**

```yaml
name: ci
on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  build-and-smoke:
    strategy:
      fail-fast: false
      matrix:
        os: [ubuntu-22.04, macos-14]
    runs-on: ${{ matrix.os }}

    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: "1.21"
          cache: true
          cache-dependency-path: |
            cli/go.sum
            daemon/go.sum

      - name: Set up CMake
        uses: lukka/get-cmake@latest
        with:
          cmakeVersion: "3.27.x"

      - name: Show toolchain versions
        run: |
          cmake --version
          go version
          c++ --version

      - name: Configure
        run: cmake -S . -B build -DCMAKE_BUILD_TYPE=Debug

      - name: Build C++ targets
        run: cmake --build build -j

      - name: ctest
        working-directory: build
        run: ctest --output-on-failure

      - name: Build Go workspace
        run: |
          go build -o build/loom-daemon ./daemon/cmd/loom-daemon
          go build -o build/loom         ./cli/cmd/loom

      - name: go test
        run: go test ./...

      - name: End-to-end smoke
        run: ./tools/smoke.sh
```

- [ ] **Step 2: Validate the workflow YAML locally**

If `actionlint` is installed: `actionlint .github/workflows/ci.yml`.
Otherwise visual inspection — confirm both matrix entries name a real runner
label and that `tools/smoke.sh` is in the workflow.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: build + smoke on ubuntu-22.04 and macos-14"
```

---

### Task 13: Designed README

**Files:**
- Create: `README.md`

The README is itself part of the showroom; it gets typographic care, not
boilerplate. M1 ships the version below; subsequent milestones expand the
"What's there" matrix.

- [ ] **Step 1: Write `README.md`**

```markdown
# Loom

> Observability harness for local-compute AI systems.

Loom is the harness you wrap around a local-AI workload — an inference
runtime, an agent, a small-team app — to make it observable, auditable,
and trustworthy on the device that runs it.

It is the deliberate counterpoint to the dark-factory style of the SDKs it
instruments. The SDKs are terse, agent-readable, optimized for the model.
Loom is the room people sit in: a polished C++ embed library, a tiny Go
daemon, a designed terminal UI, and a single-file HTML report you can hand
to an auditor.

## Status

Loom is under active development. Today's checkpoint:

| Milestone | Scope                                                         | Status |
|-----------|---------------------------------------------------------------|--------|
| M1        | Skeleton: repo, public API headers, CLI/daemon stubs, CI      | ✓      |
| M2        | Ring buffer, embed-lib bodies, daemon drain, `events.jsonl`   | next   |
| M3        | Audit + redaction + hash chain, `loom verify`                 | —      |
| M4        | Designed TUI (`loom watch`, `loom view`)                      | —      |
| M5        | Single-file `report.html` with in-browser chain verifier      | —      |
| M6        | Bedrock W4A16 inference-runtime integration                   | —      |
| M7        | Python / Go bindings, `loom doctor`, release pipeline         | —      |

## Quickstart

Requires CMake ≥ 3.20, a C++17 compiler, and Go 1.21.

```bash
git clone https://github.com/charlespers/loom.git
cd loom
make build
./build/loom version
./build/loom run -- echo "hello from a future-loom-instrumented process"
```

## Design

The full design is in [`docs/design/2026-05-02-loom-design.md`](docs/design/2026-05-02-loom-design.md).
Highlights:

- Three components, three processes, one mmap'd shared region.
- Embed lib hot path: < 300 ns per `span` enter/exit, no allocation.
- Audit events are sync-by-default and hash-chained; tampering breaks the chain.
- Two parallel artifacts per run — `audit.jsonl` (private, mode 0600) and
  `audit.public.jsonl` (redacted, safe to share).
- Single-host v1; cross-process trace propagation reserved for v2.

## License

Apache-2.0. See [`LICENSE`](LICENSE).
```

- [ ] **Step 2: Visual inspection**

Render the README in a Markdown viewer (or push to GitHub and view there).
Confirm: the status table renders cleanly, the code block runs as advertised,
the link to the design doc resolves.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: designed M1 README"
```

---

## Self-Review

Spec coverage:

| Spec section | Task(s) |
|---|---|
| § 1 motivation, § 2 aesthetic | Task 13 (README narrative) |
| § 3 architecture (3-component) | Tasks 4, 7, 8 (embed lib + daemon + CLI) |
| § 4 wire format | Pinned in headers (Task 3); bodies in M2 |
| § 5.1 C++ API | Task 3 (header) + Task 4 (no-op bodies) |
| § 5.2 C ABI canonical | Task 3 (`loom_c.h`) + Task 4 (impl) |
| § 5.3 weak-symbol hooks | Task 3 (`loom_hooks.h`) |
| § 6 daemon | Task 7 (skeleton, full body in M2) |
| § 7 CLI surface | Task 8 (cobra wiring + stubs) + Task 9 (`run` real body) |
| § 9 perf budget | Microbench scaffolding deferred to M2 |
| § 10 repo layout | Tasks 1–13 collectively; deferred dirs (`bindings/`, `tools/replay/`) created in their respective milestones |
| § 17 milestones | This plan delivers M1 in full |

No spec gaps. M2–M7 are correctly deferred and explicitly listed in the
README status table so users can see what is and isn't there.

Type-consistency check: the C ABI types declared in `loom_c.h` (Task 3) match
what `embed/src/loom.cpp` defines (Task 4) and what the `loom.h` C++ inline
shims call (Task 3). The `Severity` enum maps to ints 0/1/2 in both the header
(`enum class Severity : int { Warn = 0, Error = 1, Fatal = 2 }`) and the C ABI
(`int severity`).

Placeholder scan: no "TBD"/"TODO"/"implement later" tokens remain. Every code
block contains the actual code an engineer should paste; every shell command
shows expected output where it has output.

## Acceptance criteria for M1

A reviewer should be able to:

1. `git clone` the repo on a fresh macOS or Ubuntu 22.04 machine.
2. Run `make smoke` and see `OK: M1 smoke test passed`.
3. Run `./build/loom version` and see CLI 0.1.0 + wire schema `loom.event.v1`.
4. Run `./build/loom watch` and get a clear "not implemented in M1; planned for M4" message and a non-zero exit.
5. See CI green on both matrix runners.

If any of these fail, M1 is not complete.
