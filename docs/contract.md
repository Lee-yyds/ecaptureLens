# eCaptureLens shared contract

The CLI, HTTP API and embedded Flutter native bridge call the same Go gateway.
`gateway.Open(dir string) (*Service,error)`, `Call(ctx context.Context, method string, params json.RawMessage) (any,error)`, `Close() error`, `Handler(token string) http.Handler`, `WebHandler(token,directory string) (http.Handler,error)`.
Core: `lens.Open(dir string) (*Engine,error)`, with the same Call/Close signatures. Unknown method must return an error.

HTTP: POST /api, Authorization: Bearer TOKEN (when set), JSON `{ "method":"requests.list", "params":{} }`; response `{ "result": ... }` or `{ "error":"message" }`. GET /health returns JSON. No business logic in Flutter/CLI.

PC: `web` requires a token (generated when absent), serves Flutter assets and permits browser API calls only from the request's exact scheme/Host origin. No cross-origin CORS. API-only `Handler` still rejects browser Origins, including Android's embedded server. Non-browser CLI and Android HTTP clients can call either API with the configured token. Web token bootstrap uses a URL fragment, removes it from the address bar and stores credentials only for the tab session.

Core methods:
- `system.info` -> version, history_limit, supported methods.
- `events.ingest` params `{events:[Event]}` -> ingested count. Event: device_id, connection_id, stream_id (string), direction (request/response), timestamp (RFC3339Nano), protocol, src, dst, payload (base64 HTTP bytes), optional pid, process, sequence. HTTP/2 input uses upstream reconstructed complete HTTP messages and stream_id.
- `requests.list` params `{query?,device_id?,connection_id?,limit?,offset?}` -> `{items:[Transaction],total:N}` newest first.
- `requests.get` `{id}` -> Transaction.
- `requests.clear` `{}` -> result.
- `sessions.list` `{device_id?}` -> array of session summaries.
- `sessions.timeline` `{connection_id,device_id?}` -> array of transactions ordered ascending.
- `body.view` `{id,side:"request"|"response",format:"json"|"hex"|"text"|"protobuf"|"grpc"|"NAME",descriptor?:base64,message_type?:string}` -> `{text:string}`. Format supports a registered business decoder.
- `history.export` `{format:"json"|"har",ids?:[string]}` -> `{content:string,mime:string}`.
- `history.import` `{format:"json"|"har",content:string}` -> result. JSON is export transaction list; both preserve bodies.
- `devices.list` -> array of Device; `devices.get` `{id}` -> Device; `devices.put` Device -> Device; `devices.delete` `{id}` -> result. Device: id,name,endpoint,start_command,stop_command. Commands explicitly configured by user; executed on host that runs core (Android /system/bin/sh, desktop shell). ADB commands for PC are allowed.
- `decoders.list` -> array; `decoders.put` `{name,command}` -> result; `decoders.delete` `{name}` -> result. External command receives body bytes on stdin and UTF8 result on stdout. Errors propagate.

Transaction JSON: id,device_id,connection_id,stream_id,protocol,method,url,status(int),started_at(RFC3339),duration_ms(number),request (Message nullable),response(Message nullable), optional error. Message: headers (map string -> array string),body(base64),raw(base64),timestamp(RFC3339). Additional fields permitted. History retains latest 1000 transactions durably, pending pairs included. HTTP/1 FIFO within a connection; HTTP/2 pairs by stream ID. Device IDs scope all pairing keys.

Gateway extra methods:
- `capture.start` `{device_id}` -> status; executes configured start command then subscribes to device endpoint if nonempty. Empty command permits connect only. Do not reconnect silently.
- `capture.stop` `{device_id}` -> status; disconnect and configured stop command.
- `capture.status` `{device_id?}` -> array `{device_id,state,error?,logs?}`.
- `devices.discover` -> ADB device list; may error if adb unavailable.
- `api.describe` -> full methods list and help.

The eCaptureQ WebSocket sends binary Protobuf LogEntry messages. Its upstream server requires an Origin header. Event.timestamp is Unix seconds (not nanoseconds); payload is the raw reconstructed HTTP message in the v2.9.1 typed path. The formatted text sink is a separate output. `capture.start` initially returns starting; poll capture.status for running/error. Local CLI start stays foreground, while remote API start is asynchronous.

Native C ABI: `LensStart(const char *data_dir)` returns allocated UTF8 JSON `{url,token}` or `{error}`; `LensFree(char*)`; `LensStop()` returns allocated JSON. Starts loopback server on an ephemeral port with random token. Flutter uses same HTTP client for local embedded or configured remote service.
