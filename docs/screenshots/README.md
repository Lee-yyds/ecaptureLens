# Fixture previews

The `fixture-*.png` files were rendered on 2026-11-02 by Flutter's widget-test renderer using the explicit synthetic `TestApi` fixture in `ui/test/workspace_test.dart`. Each includes a fixture-preview label. They show layout and available controls.

- `fixture-desktop.png`: 1440 × 960, desktop request list and response inspector.
- `fixture-phone-list.png`: 390 × 844, compact request list.
- `fixture-phone-detail.png`: 390 × 844, compact response inspector.

The temporary rendering test loaded Flutter's bundled Roboto and Material Icons plus macOS Monaco for readable text, then captured a RepaintBoundary. Both rendering tests passed without Flutter layout exceptions. The temporary test was removed after visual inspection; production code and existing tests were unchanged.

`android-device-list.png` and `android-device-detail.png` are actual screenshots of the release APK on a Pixel6. The single record is a controlled BoringSSL HTTPS request captured by real Android eCapture2.9.1, then imported through the desktop CLI into the app's embedded core. These show native startup, persisted history and real response Body; they do not claim that the screenshot was taken during a live capture subscription inside the APK.
