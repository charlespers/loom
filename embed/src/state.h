// embed/src/state.h — internal-only state.
#pragma once
#include <atomic>

namespace loom::detail {
inline std::atomic<bool> g_active{false};
}
