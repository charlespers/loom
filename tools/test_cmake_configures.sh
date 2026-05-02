#!/usr/bin/env bash
set -euo pipefail
rm -rf build
cmake -S . -B build -DCMAKE_BUILD_TYPE=Debug >/dev/null
test -f build/CMakeCache.txt
echo "OK: cmake configured"
