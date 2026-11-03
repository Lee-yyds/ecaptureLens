# eCaptureLens Implementation Plan

Goal: deliver a runnable CLI/API and Flutter Web (PC)/Android product sharing the Go core. PC migration details: [web-plan.md](web-plan.md).

Architecture and contract: see design.md and contract.md.

- [x] Core: models, HTTP message parsing and transaction pairing; persistence limited to 1000; decoders, history, devices and operations. Tests first for behavior and failures.
- [x] CLI: all shared operations, serve, import/export/view/copy-compatible stdout, structured JSON and generic RPC. CLI tests and executable smoke.
- [x] Flutter: responsive workspace, bodies/decoders/history/devices/settings, Android local native bridge and remote connection. Original native PC runners are replaced by Flutter Web in the PC migration.
- [x] Gateway: typed eCaptureQ WebSocket ingestion, capture lifecycle, automation HTTP API/auth, native C ABI and build scripts. Integration tests for wire messages and HTTP plus capture errors.
- [x] Original integration: Go race/vet/build, Flutter analyze/8 tests, Android APK/signature/install, real eCapture capture and GUI/CLI shared-core flow, foreground API, stop-error cleanup, independent review and documented platform limits. See verification.md for dated PC Web migration results.

Review focus: HTTP/1 FIFO and interim responses; interleaved H2 streams/devices; corrupt history/import without data loss; process exit/disconnect/errors visible; native ABI and token-bound local server lifecycle.
