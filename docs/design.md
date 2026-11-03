# First release design

Go `lens` owns parsing, pairing, decoders and durable bounded history. `gateway` owns capture processes, eCaptureQ Protobuf WebSocket subscriptions and a common JSON operation entry. The CLI is a complete entry point with discoverable commands and generic RPC. On PC, `ecapturelens web` serves Flutter Web and its same-origin API. Android uses an embedded Go C shared library or connects to a remote service. Native desktop runners are replaced by the browser entry.

Upstream eCapture `tls --text-source pcapng` reconstructs HTTP/1 and HTTP/2; Lens consumes its typed events, retaining direction and stream IDs. No new TLS stack and no silently guessing encrypted PCAP content. Offline JSON/HAR import and export provide durable portable history. Raw pcapng remains an upstream capture artifact for later reprocessing.

History: atomic file persistence and cross-process ownership lock, globally latest 1000 transactions, scoped pairing by device and connection and stream. Core errors surface in CLI and GUI. JSON/hex/text/protobuf/gRPC and explicitly registered external business decoders share one body operation.

UI: dense neutral workspace with a device sidebar, capture toolbar, searchable request table and request/response inspector. Desktop split panes; mobile list/detail navigation. Configurable commands live in device editor. PC uses the serving Go host; a per-run token is passed in the URL fragment and retained in browser session storage. The Android embedded service remains default; remote endpoint/token can be selected. Root/eCapture binary installation is a real deployment prerequisite.

Acceptance: Go race tests for pairing (including interleaved H2 and device isolation), restart/1000 limit, malformed input and decoders; CLI and HTTP smoke flow; Flutter analyze/widget tests and build as tools permit; real-device verification only claimed when executed.
