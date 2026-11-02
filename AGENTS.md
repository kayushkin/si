# About si

## What it owns

Communications layer routing messages between external platforms and inber I/O feeds. Adapters for matterbridge (Discord, Telegram, Slack), WebSocket, and TUI. Listens on `:8090` (`SI_WS_ADDR`; the unit binds loopback). Every environment variable it reads is declared in `internal/config/settings.go` and served at `GET /settings`; a new `os.Getenv` fails `TestEveryEnvironmentVariableTheServiceReadsIsDeclared` until it is declared there. ⚠️ No matterbridge process or unit runs on this host (measured 2026-09-18), so the matterbridge adapter has nothing to talk to.

Messages to and from agents travel over NATS (`SI_FEED=nats`, the default; `bus` is an older name for the same feed, and `echo` is a test loop); `feed/` holds the three. si no longer talks to inber over a WebSocket. `router.go` routes between adapters and feeds, `message.go` is the shared message type, `cmd/si/main.go` is the entry point, and `adapter/` holds discord, matterbridge, tui and websocket.

## Where this prompt lives

These sections are stored in agent-store as a project prompt collection and rendered, with identical text, to `AGENTS.md` and `CLAUDE.md` at the root of this repo, so that whichever file a harness reads it gets the same thing. Edit them on dash `/files`, or edit either rendered file: the 15-minute scan carries the edit back into the sections and out to the other file. The host prompt keeps one row for this repo with only what an agent elsewhere needs.

# Working in this repo

## Dead code

`feed/inber_direct_deprecated.go.bak` is the old direct inber feed, kept only as a file.
