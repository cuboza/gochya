#!/usr/bin/env bash
set -euo pipefail

# Cross-compiles the Shared Core for the Android ABIs the companion ships and
# drops each .so into jniLibs, where `DynamicLibrary.open('libgochya_core.so')`
# finds it at runtime. Requires ANDROID_NDK_HOME and the Rust Android targets:
#
#   rustup target add aarch64-linux-android armv7-linux-androideabi \
#                     x86_64-linux-android
#
# 32-bit x86 (`i686-linux-android`, ABI `x86`) is deliberately absent. The Core
# pins every FFI struct to an exact size, and those sizes assume u64 aligns to
# 8; the i386 SysV ABI aligns it to 4, so `GochyaNeedsStateV1` is 52 bytes
# there and the Core's own `const` assertion refuses to compile. That ABI is
# emulator-only legacy and has never been a supported target.

project_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$project_root"

if [[ -z "${ANDROID_NDK_HOME:-}" ]]; then
  echo "ANDROID_NDK_HOME is not set" >&2
  exit 1
fi

api_level="${ANDROID_API_LEVEL:-24}"
host_tag="linux-x86_64"
if [[ "$(uname -s)" == "Darwin" ]]; then
  host_tag="darwin-x86_64"
fi
toolchain="${ANDROID_NDK_HOME}/toolchains/llvm/prebuilt/${host_tag}/bin"
if [[ ! -d "$toolchain" ]]; then
  echo "NDK toolchain not found at ${toolchain}" >&2
  exit 1
fi

jni_root="clients/companion/android/app/src/main/jniLibs"

# target triple : jniLibs ABI directory : clang target prefix
targets=(
  "aarch64-linux-android:arm64-v8a:aarch64-linux-android"
  "armv7-linux-androideabi:armeabi-v7a:armv7a-linux-androideabi"
  "x86_64-linux-android:x86_64:x86_64-linux-android"
)

for entry in "${targets[@]}"; do
  IFS=":" read -r triple abi clang_prefix <<<"$entry"

  linker="${toolchain}/${clang_prefix}${api_level}-clang"
  if [[ ! -x "$linker" ]]; then
    echo "missing linker ${linker}" >&2
    exit 1
  fi

  # Cargo reads the linker from an env var keyed by the upper-cased triple.
  linker_var="CARGO_TARGET_$(echo "$triple" | tr '[:lower:]-' '[:upper:]_')_LINKER"
  env "${linker_var}=${linker}" \
    "AR=${toolchain}/llvm-ar" \
    cargo build --release -p gochya-core --target "$triple"

  mkdir -p "${jni_root}/${abi}"
  cp "target/${triple}/release/libgochya_core.so" "${jni_root}/${abi}/"
  echo "${jni_root}/${abi}/libgochya_core.so"
done
