#!/usr/bin/env bash
set -euo pipefail

project_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$project_root"

cargo fmt --all -- --check
cargo clippy --workspace --all-targets -- -D warnings
cargo test --workspace
cargo build --release -p gochya-core

cc \
  core/tests/abi_smoke.c \
  -Icore/ffi \
  target/release/libgochya_core.a \
  -lpthread \
  -ldl \
  -lm \
  -o target/abi-smoke

target/abi-smoke
node tools/check-markdown-links.mjs
