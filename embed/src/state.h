// embed/src/state.h — internal-only state for the embed library.
//
// M2-partial: holds the active flag, sequence counter, events.jsonl FILE*,
// and write mutex. The proper M2 ring buffer + daemon supersede these in
// the next milestone; the global FILE* model only exists so iter26 can
// "see Loom in action" before the ring lands.
#pragma once
#include <atomic>
#include <cstdint>
#include <cstdio>
#include <mutex>

namespace loom::detail {
inline std::atomic<bool>     g_active{false};
inline std::atomic<uint64_t> g_seq{0};
inline std::FILE*            g_events_file = nullptr;
inline std::mutex            g_write_mutex;
}
