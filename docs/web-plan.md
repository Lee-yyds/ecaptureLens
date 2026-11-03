# PC Web migration

2026-11-03. Requested architecture: PC uses Flutter Web + Go; Android keeps its embedded Go core and foreground service.

The existing Go gateway remains the sole business entry for CLI, browser and Android. `ecapturelens web` serves the compiled Flutter assets and authenticated `/api` on one origin. It prints a browser URL with a token fragment; the browser removes that fragment and retains the token only for the tab session. API-only `serve` keeps its existing behavior. Capture commands and business decoders run on the Go host.

Distribution is a Go executable beside `web/` assets. This avoids a desktop native runner, C bridge and Xcode requirement on PC. Android continues to package its Go shared library. Import/export uses browser file selection/download on Web and the document picker on Android.

- [ ] Go: Web entry, static assets, same-origin authenticated API; verify with gateway and CLI tests.
- [ ] Flutter: conditional platform boundaries, browser session/token bootstrap, Web imports/downloads; verify analyze, widgets and Web release build.
- [ ] Packaging: Web + Go desktop bundle, Android target unchanged; update product instructions and remove obsolete native PC runners.
- [ ] Integration: real browser list/detail/decoding/import/export/settings/API, CLI against the same service; rebuild Android and check local core startup.

No new protocol or storage implementation is needed. Existing parsing, pairing and retention tests remain the regression checks for shared capabilities.
