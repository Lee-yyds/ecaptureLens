# Flutter workspace

One Flutter codebase provides PC Web and native Android views. All analysis operations are sent to the shared Go gateway. PC uses its serving Go host; Android keeps its embedded `LensStart` FFI core, foreground service and remote connection settings.

```sh
flutter pub get
flutter analyze
flutter test
```

From the project root:

```sh
scripts/build-ui.sh web
build/desktop/ecapturelens web
scripts/build-ui.sh android
```

Set `FLUTTER=/absolute/path/to/flutter` if Flutter is not on PATH. The Web build bundles its renderer locally with `--no-web-resources-cdn`. The desktop bundle contains the host Go executable beside `web/`; distribute both together. PC needs a browser and the Go executable, with no Xcode, Flutter runtime or native shared library. `ecapturelens web --web-dir PATH` selects a different asset directory.

Open the URL printed by `web`. Its token fragment is removed after reading and the token remains in the tab's sessionStorage for reloads. **Change service** clears the session and returns to token entry. The browser always calls its serving origin with Bearer authentication. To access another Go host, open its Web URL. **API → Copy CLI connection** exposes the service credentials for agents. Import uses browser file selection; exports download files. Browser clipboard access requires a secure context (localhost or HTTPS).

Android builds target arm64 by default and bundle `libecapturelens.so` in `jniLibs`. For an x86_64 emulator, build that ABI with `scripts/build-native.sh android x86_64` and select the matching Flutter target. Android requires SDK/NDK and Java. It permits HTTP for the embedded loopback API and configured remote services. The release APK uses the local test signing key; publishing requires a release signing identity.

Use **Settings → Devices** to configure the WebSocket endpoint and explicit shell commands. Commands run on the Go host: PC commands can use ADB; Android local commands can invoke an installed `su`. Root and eCapture installation remain prerequisites. Android local mode starts a visible foreground service so the core continues while using the target app. **Change service** shuts down that local core and service. Force stop or reboot requires reopening; history persists. Store distribution must declare the service's special-use purpose.

Android can still use **Change service** to switch between its embedded core and a remote API, including a PC Web service. Its connection settings and remote token remain in app-private data. JSON/HAR import/export uses the platform document picker, and copy uses the system clipboard.

Native PC runners are removed. API-only `serve` retains its browser-Origin rejection; use `web` for the browser interface. The current Web CLI serves HTTP and defaults to loopback; use a trusted local connection or SSH tunnel for remote access. The gateway checks the request's scheme and Host and does not trust forwarded headers, so it does not support browser HTTPS termination in front of its plain-HTTP origin check.
