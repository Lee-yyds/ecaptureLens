#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
flutter_bin="${FLUTTER:-flutter}"
target="${1:-web}"
case "$target" in
  web)
    (cd ui && "$flutter_bin" --no-version-check pub get && "$flutter_bin" --no-version-check build web --release --no-web-resources-cdn)
    mkdir -p build/desktop/web
    cp -R ui/build/web/. build/desktop/web/
    executable=build/desktop/ecapturelens
    if [ "$(go env GOOS)" = windows ]; then executable="$executable.exe"; fi
    CGO_ENABLED=0 go build -trimpath -o "$executable" ./cmd/ecapturelens
    if [ "$(uname -s)" = Darwin ] && [ "$(go env GOOS)" = darwin ]; then
      codesign --force --sign - "$executable"
    fi
    echo "Desktop bundle: build/desktop (run $executable web)"
    ;;
  android)
    scripts/build-native.sh android arm64-v8a
    cd ui
    "$flutter_bin" --no-version-check pub get
    "$flutter_bin" --no-version-check build apk --release --target-platform android-arm64
    ;;
  *) echo 'Usage: FLUTTER=/path/to/flutter scripts/build-ui.sh [web|android]' >&2; exit 1 ;;
esac
