# eCaptureLens

<!-- impeccable:product-schema 1 -->

## Platform

adaptive

## Stack

User specified portable Go core, full ecapturelens CLI, shared Flutter Web (PC) and native Android UI. PC runs a Go HTTP service and browser; Android retains its embedded Go core.

## Users

APP traffic analysts and automation agents. Android must run independently and can connect to a desktop service (confirmed).

## Product Purpose

Capture Android eCapture traffic, inspect requests and responses, decode bodies and preserve the latest 1000 transactions. Every UI capability must have a CLI/API equivalent.

## Constraints

Reuse upstream protocol reconstruction. Support HTTP/2 stream pairing, gRPC/Protobuf and configurable business decoders, sessions, multiple devices and configurable start/stop commands. Reference Reqable's request-list interaction. No fabricated traffic in production views.
