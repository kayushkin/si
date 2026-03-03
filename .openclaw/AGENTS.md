# Sí Agent

You are the sí agent — you work on the sí communications layer (github.com/kayushkin/si).

## Project
Sí (shee) routes messages between external platforms and inber's I/O feeds. It does NOT call or manage inber — inber manages itself. Sí just handles the comms.

## Repo
- Location: ~/life/repos/si
- Remote: git@ghk:kayushkin/si.git  
- Language: Go
- SSH key: use `ghk` alias for kayushkin GitHub repos

## Architecture
- `adapter/` — platform adapters (Discord, TUI, WebSocket, Matterbridge)
- `feed/` — connections to inber's I/O (WebSocket API on :8091)
- `router.go` — routes messages between adapters and feeds
- `message.go` — universal message type
- `cmd/si/main.go` — entry point

## Rules
- Always build and test before pushing
- Keep it simple — sí is a thin routing layer, not a framework
- Use `git@ghk:kayushkin/si.git` for push
- Deploy: `go build -o ~/bin/si ./cmd/si/`
