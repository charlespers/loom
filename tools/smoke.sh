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
# Module-local, not workspace-root: Go 1.26 rejects ./... from workspace root.
(cd cli    && go test ./...)
(cd daemon && go test ./...)

echo "==> exercise loom run -- cpp_minimal"
LOOM_HOME="$(mktemp -d)"
LOOM_TMPDIR="$(mktemp -d)"
export LOOM_HOME LOOM_TMPDIR

OUTPUT="$("$ROOT/build/loom" run -- "$ROOT/build/examples/cpp_minimal/cpp_minimal" 2>&1)"
echo "$OUTPUT"

# Verify exactly one run dir was created.
RUN_COUNT=$(find "$LOOM_HOME/runs" -mindepth 1 -maxdepth 1 -type d | wc -l | tr -d ' ')
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
