package main

const helpText = `ecapturelens — eCapture traffic analysis for humans and agents

Usage: ecapturelens [global flags] COMMAND [flags]
All operations share the same Go core as the graphical application.

Global flags (accepted anywhere):
  --data-dir DIR     Local durable history/config directory
                    default: OS user config directory/ecapturelens
  --server URL       Call an existing service instead of opening local state
  --token TOKEN      HTTP bearer token (also ECAPTURELENS_TOKEN)
  --timeout 30s      Remote HTTP timeout
  --json            Emit structured JSON, including body/export envelopes
Environment: ECAPTURELENS_DATA_DIR, ECAPTURELENS_SERVER, ECAPTURELENS_TOKEN

Commands:
  web [--listen 127.0.0.1:8787] [--web-dir DIR]
                                      Browser UI and authenticated API
                                      default DIR: web beside the executable
  serve [--listen 127.0.0.1:8787]      Persistent analysis/capture service
  version | system info | api describe | help

  requests list [--query TEXT] [--device ID] [--connection ID]
                [--limit 1000] [--offset 0]
  requests get ID
  requests clear
  body view ID [--side request|response] [--format FORMAT] [--output FILE]
               [--descriptor FILE] [--message-type package.Message]
    FORMAT: text, json, hex, protobuf, grpc, or a registered decoder name
    Body output can be piped to pbcopy/xclip or redirected to a file.

  sessions list [--device ID]
  sessions timeline CONNECTION_ID [--device ID]
  history export [--format json|har] [--ids ID,ID] [--output FILE|-]
  history import [--format json|har] [--file FILE|-]
  ingest [--file FILE|-]              Events JSON, array, envelope or JSONL

  devices list | devices discover
  devices get ID | devices delete ID
  devices put ID [--name NAME] [--endpoint ws://HOST:PORT]
                 [--start-command SHELL] [--stop-command SHELL]
  capture start DEVICE_ID | capture stop DEVICE_ID
  capture status [DEVICE_ID]
  decoders list
  decoders put NAME --command SHELL
  decoders delete NAME
  rpc METHOD [--params '{"key":"value"}']

Examples:
  ecapturelens web
  ecapturelens serve --token my-local-token
  ecapturelens --server http://127.0.0.1:8787 --token my-local-token requests list
  ecapturelens devices put phone --endpoint ws://127.0.0.1:28257
  ecapturelens --server http://127.0.0.1:8787 capture start phone
  ecapturelens body view REQUEST_ID --side response --format json
  ecapturelens body view REQUEST_ID --format protobuf --descriptor api.pb \
      --message-type example.Response
  ecapturelens history export --format har --output capture.har
  ecapturelens rpc requests.list --params '{"query":"login","limit":10}'

Local capture start stays in the foreground until Ctrl+C, then stops capture.
For agent-driven capture, use serve and --server to start/stop across CLI calls.
Start/stop and custom decoder commands execute on the machine running the core.
Android local commands use its shell; PC commands may invoke adb explicitly.
Network listening requires a token. Web defaults to loopback HTTP.
Web generates a token unless provided; open the printed URL to connect.
History retains the latest 1000 transactions, including request/response bodies.
eCapture must provide reconstructed text events for HTTP/2 stream pairing.
Failures go to stderr and exit nonzero; stdout contains only requested data.
`
