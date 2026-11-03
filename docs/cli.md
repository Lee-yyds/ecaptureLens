# ecapturelens CLI

Build from the repository root:

```sh
go build -o bin/ecapturelens ./cmd/ecapturelens
bin/ecapturelens help
```

The CLI calls the same Go operation entry as Flutter. Data commands emit JSON.
`body view` emits decoded text, and `history export` emits file content; add
`--json` to receive their structured result envelopes on stdout; an explicit
`--output FILE` still writes the decoded/exported content to that file. Errors go to stderr and
return exit code 1. No status messages are mixed into data stdout.

## Persistent service and agents

For the PC browser interface, build `scripts/build-ui.sh web` and run:

```sh
build/desktop/ecapturelens --data-dir ./capture-data web
```

`web` serves adjacent `web/` assets and the same JSON API. Open the printed
`url`, including `#token=...`. A random token is generated when `--token` /
`ECAPTURELENS_TOKEN` is not supplied; generated tokens change on each start.
The fragment is not sent in HTTP requests, and the UI removes it and stores
the token only in the tab session. **API → Copy CLI connection** copies the
URL and token for agents. CLI and Android clients can use the same API.
`web --listen ADDRESS --web-dir PATH` changes the listener/assets. Keep the
executable and `web/` together when distributing; no Xcode is required.

`serve` remains the API-only entry:

Start one owner of a history directory, then use `--server` for every operation:

```sh
ecapturelens --data-dir ./capture-data serve --listen 127.0.0.1:8787 --token local-secret
export ECAPTURELENS_SERVER=http://127.0.0.1:8787
export ECAPTURELENS_TOKEN=local-secret
ecapturelens api describe
ecapturelens devices list
ecapturelens requests list --query login --limit 20
```

`serve` stays in the foreground and shuts down on SIGINT/SIGTERM. Non-loopback
binding requires a token. Remote plain HTTP does not encrypt payloads or tokens;
use a trusted local connection, tunnel or HTTPS termination for remote access.
The CLI accepts `http://` and `https://` service base URLs. `--timeout 30s`
controls remote request timeout. `--data-dir`, `--server`, `--token` and `--json`
can appear before or after subcommands.

Without `--server`, commands open local state directly. A running service or
embedded GUI owns its directory exclusively: use its API rather than opening
the same directory from a second process. `ECAPTURELENS_DATA_DIR` overrides the
default OS user configuration directory's `ecapturelens` subdirectory.

## Devices and capture

Register the device's reachable eCaptureQ WebSocket endpoint. Start/stop commands
are explicit operator configuration and run on the machine hosting the core.
For a desktop core they may invoke `adb`; on an Android embedded core they run
on that phone, where privileged capture requires the operator's root setup.

```sh
ecapturelens devices discover
ecapturelens devices put phone --name 'Android phone' --endpoint ws://127.0.0.1:28257
ecapturelens devices get phone
ecapturelens capture start phone
ecapturelens capture status phone
ecapturelens capture stop phone
```

To configure commands, replace the entire device configuration with
`devices put ID --name NAME --endpoint URL --start-command 'COMMAND' --stop-command 'COMMAND'`.
An empty start command allows connecting to a separately started eCapture.
`devices delete ID` removes a saved device.

When `capture start` runs without `--server`, it holds the Go core in the
foreground until Ctrl+C and then stops capture. Agent workflows should use
`serve` and remote CLI calls so start, inspect and stop can run in separate
commands. `devices discover` requires `adb` on the core's host; saved-device
operations do not.

HTTP/2 pairing expects complete HTTP messages reconstructed by upstream
eCapture's `--text-source pcapng` mode, with its stream identifiers retained.
Lens does not independently decrypt encrypted TLS packet captures.

## Inspect, decode and copy

```sh
ecapturelens requests list --device phone --limit 100 --offset 0
ecapturelens requests get REQUEST_ID
ecapturelens body view REQUEST_ID --side request --format json
ecapturelens body view REQUEST_ID --side response --format hex
ecapturelens body view REQUEST_ID --format json | pbcopy
ecapturelens body view REQUEST_ID --format text --output response.txt
ecapturelens body view REQUEST_ID --format protobuf --descriptor api.pb --message-type example.Response
ecapturelens body view REQUEST_ID --format grpc --descriptor api.pb --message-type example.Response
ecapturelens sessions list --device phone
ecapturelens sessions timeline CONNECTION_ID --device phone
```

The descriptor is a binary protobuf `FileDescriptorSet`, for example produced
by `protoc --include_imports --descriptor_set_out=api.pb api.proto`. Without a
descriptor, Protobuf inspection displays wire fields rather than claiming to
know field names. Body output can be piped to the platform clipboard tool
(`pbcopy`, `xclip`, or `clip.exe`).

A registered business decoder receives body bytes on stdin and writes decoded
UTF-8 text to stdout. Nonzero exit status and stderr are reported as errors:

```sh
ecapturelens decoders put business-json --command 'python3 /absolute/path/decode.py'
ecapturelens decoders list
ecapturelens body view REQUEST_ID --format business-json
ecapturelens decoders delete business-json
```

## History and event import

The core keeps the most recent 1000 transactions, including bodies, across
restarts. Export a session before that retention window passes if it must be
kept longer:

```sh
ecapturelens history export --format har --output capture.har
ecapturelens history export --format json --ids REQUEST_1,REQUEST_2 --output selection.json
ecapturelens history import --format har --file capture.har
ecapturelens history import --format json --file selection.json
ecapturelens requests clear
```

`history import` expects Lens transaction JSON or HAR. `ingest` is for captured
events; it accepts a single JSON object, JSON array, `{ "events": [...] }`
envelope, or newline-delimited JSON objects. Payload bytes use base64:

```json
{"device_id":"phone","connection_id":"connection-1","stream_id":"1","direction":"request","timestamp":"2026-11-02T13:00:00Z","protocol":"HTTP/2.0","src":"10.0.0.2:43100","dst":"example.test:443","payload":"R0VUIC8gSFRUUC8yLjANCkhvc3Q6IGV4YW1wbGUudGVzdA0KDQo="}
```

Use `ecapturelens ingest --file events.jsonl` or pipe the events to
`ecapturelens ingest`. Files and stdin are limited to 128 MiB per invocation.
The remote HTTP API caps each encoded request at 64 MiB, so remote imports
must also fit that limit after JSON encoding.

## Generic automation interface

Every gateway operation is accessible without waiting for a CLI flag addition:

```sh
ecapturelens rpc requests.list --params '{"query":"login","limit":10}'
ecapturelens rpc capture.status --params '{}'
ecapturelens rpc events.ingest --params '{"events":[]}'
```

The equivalent HTTP request is:

```sh
curl -H 'Authorization: Bearer local-secret' -H 'Content-Type: application/json' \
  -d '{"method":"requests.list","params":{"limit":10}}' \
  http://127.0.0.1:8787/api
```

The API returns `{ "result": ... }` or `{ "error": "message" }`. The CLI
unwraps successful `result` for data stdout and propagates API errors even if
the HTTP response incorrectly uses a successful status code.
