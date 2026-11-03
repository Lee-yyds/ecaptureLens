#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
target="${1:-host}"
case "$target" in
  host)
    mkdir -p bin
    go build -trimpath -o bin/ecapturelens ./cmd/ecapturelens
    if [ "$(go env GOOS)" = darwin ]; then codesign --force --sign - bin/ecapturelens; fi
    ;;
  android)
    sdk="${ANDROID_SDK_ROOT:-${ANDROID_HOME:-$HOME/Library/Android/sdk}}"
    ndk="${ANDROID_NDK_HOME:-$sdk/ndk/28.2.13676358}"
    case "$(uname -s)" in Darwin) ndk_host=darwin-x86_64 ;; Linux) ndk_host=linux-x86_64 ;; *) echo 'Use Linux/macOS with Android NDK' >&2;exit 1 ;; esac
    mkdir -p bin/android-arm64
    CC="$ndk/toolchains/llvm/prebuilt/$ndk_host/bin/aarch64-linux-android24-clang" CGO_ENABLED=1 GOOS=android GOARCH=arm64 go build -trimpath -o bin/android-arm64/ecapturelens ./cmd/ecapturelens
    ;;
  *) echo 'Usage: scripts/build-cli.sh [host|android]' >&2;exit 1 ;;
esac
