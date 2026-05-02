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
