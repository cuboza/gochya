#!/usr/bin/env bash
set -euo pipefail

# Builds the Shared Core as a host cdylib so the Flutter client's FFI tests can
# exercise the real library instead of a mock. Device builds are separate:
# Android ships a per-ABI .so, iOS statically links the staticlib (audit T3).

project_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$project_root"

cargo build --release -p gochya-core

case "$(uname -s)" in
Darwin) library="libgochya_core.dylib" ;;
*) library="libgochya_core.so" ;;
esac

built="target/release/${library}"
if [[ ! -f "$built" ]]; then
  echo "expected ${built} after cargo build" >&2
  exit 1
fi

echo "$built"
