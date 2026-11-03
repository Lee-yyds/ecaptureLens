# Verification record

2026-11-02, macOS arm64, Go 1.26.0.

- `go test -race ./...`: PASS across core, CLI and gateway. `go vet ./...`: PASS. `gofmt -l lens gateway cmd`: empty.
- Native host `c-shared` library builds; Python ctypes exercised LensStart, random-token authenticated system.info, LensStop and LensFree.
- Android arm64 `c-shared` library built with NDK 28.2.13676358.
- Android arm64 CLI built and executed on Pixel6: system.info and devices.put both succeeded. Linux amd64 and Windows amd64 CLI cross-compilation succeeded (not runtime-tested).
- CLI runtime flow: devices.put, ingest request+response, requests.list paired status200, body.view JSON, export HAR, clear, import HAR and restored transaction.
- Real Android: Pixel 6, Android15, eCapture2.9.1. Test C client dlopen'd system BoringSSL, connected through ADB reverse to a local TLS test server. Capture restricted to shell UID2000, loopback interface and TCP18443. Typed Protobuf frames entered gateway via ADB forward; request/response paired and response JSON exactly `{"lens":"android","ok":true}`. Capture exited and test forwarding rules removed. Test artifacts remain outside repo in `/tmp/ecapturelens-smoke/`.
- This exposed the upstream WebSocket Origin requirement, now covered by a regression test. Epoch seconds and explicit startup command failures are also covered.
- Flutter3.47.9/Dart3.13.7: `flutter analyze`: no issues; `flutter test`: 8 passed. Covers API errors/auth headers, desktop inspector, mobile navigation, mobile pending-response refresh, deleted devices/decoders, CLI connection credentials, and visible endpoint tuples when upstream authority is absent.
- Widget renderer visual check: desktop1440x960, phone390x844 list and detail, no overflow/layout exceptions. [Preview provenance](screenshots/README.md) distinguishes these synthetic-fixture screenshots from device screenshots.
- Impeccable mechanical UI detector returned an empty findings array. Shell build scripts passed `bash -n`.
- Android release arm64 APK: built successfully (12.4 MB), APK v2 signature verified; installed and launched on Pixel6. Embedded Go core opened without a desktop service. Device service report confirms foreground special-use service active; the token-authenticated API remained reachable after pressing Home.
- Desktop CLI connected through ADB forwarding to the GUI's embedded core, system.info and history.import succeeded. The controlled real eCapture transaction survived APK update and was shown in the native list/detail; response Body was exactly `{"lens":"android","ok":true}`.
- Stop error cleanup: configured a managed `sleep 300` capture with `exit 7` stop command; deleted its temporary device entry while running, then used Change service. The original stop binding still ran; error `exit status 7` was visible, managed process ended, and the Android foreground service was removed despite the Go stop error. Reopened workspace successfully; temporary device configuration removed.
- APK SHA256: `b68b83f012f01f408812f6e433da7cd7a4528a5bdd738764907d4e2f01859ec9`. This is a local test-signed APK; distribution signing is not configured.
- macOS repeatedly rebuilding and launching newly signed CLI binaries from a batch/Python subprocess occasionally received SIGKILL9 without a diagnostic report. Separate direct CLI invocations, including local history workflow and remote Android API calls, succeeded. No retries or fallback were added to product code based on this host behavior.

At the original release, the macOS native Flutter build could not run without full Xcode; Windows/Linux native GUI builds were also unverified. Those PC native runners were replaced by Flutter Web on 2026-11-03. The file_picker dependency emitted a future Kotlin-plugin migration warning; the pinned Android build succeeded.
