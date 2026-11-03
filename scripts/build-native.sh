#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
target="${1:-host}"
case "$target" in
  host)
    case "$(go env GOOS)" in
      darwin) dest=build/native/macos/libecapturelens.dylib ;;
      linux) dest=build/native/linux/libecapturelens.so ;;
      windows) dest=build/native/windows/ecapturelens.dll ;;
      *) echo 'Unsupported host' >&2; exit 1 ;;
    esac
    mkdir -p "$(dirname "$dest")"
    CGO_ENABLED=1 go build -trimpath -buildmode=c-shared -o "$dest" ./cmd/lensbridge
    ;;
  android)
    sdk="${ANDROID_SDK_ROOT:-${ANDROID_HOME:-$HOME/Library/Android/sdk}}"
    ndk="${ANDROID_NDK_HOME:-$sdk/ndk/28.2.13676358}"
    case "$(uname -s)" in Darwin) ndk_host=darwin-x86_64 ;; Linux) ndk_host=linux-x86_64 ;; *) echo 'Set up an NDK build on Linux/macOS' >&2;exit 1 ;; esac
    abi="${2:-arm64-v8a}"
    case "$abi" in
      arm64-v8a) arch=arm64; compiler=aarch64-linux-android24-clang ;;
      x86_64) arch=amd64; compiler=x86_64-linux-android24-clang ;;
      *) echo 'Supported Android ABIs: arm64-v8a, x86_64' >&2;exit 1 ;;
    esac
    dest="ui/android/app/src/main/jniLibs/$abi/libecapturelens.so"
    mkdir -p "$(dirname "$dest")"
    CC="$ndk/toolchains/llvm/prebuilt/$ndk_host/bin/$compiler" CGO_ENABLED=1 GOOS=android GOARCH="$arch" go build -trimpath -buildmode=c-shared -o "$dest" ./cmd/lensbridge
    ;;
  *) echo 'Usage: scripts/build-native.sh [host|android [arm64-v8a|x86_64]]' >&2;exit 1 ;;
esac
echo "$dest"
