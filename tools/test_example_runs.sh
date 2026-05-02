#!/usr/bin/env bash
set -euo pipefail
cmake --build build --target cpp_minimal -j >/dev/null
./build/examples/cpp_minimal/cpp_minimal
echo "OK: example ran"
