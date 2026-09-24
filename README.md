# Sí

*The doorways between worlds.*

Sí (shee) is the communications layer for [inber](https://github.com/kayushkin/inber). It routes messages between external platforms and inber's I/O feeds.

Sí does **not** manage or call inber. Inber runs its own sessions and exposes feeds. Sí connects those feeds to the outside world.

## Architecture

```
┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐
│ Discord  │  │ Telegram │  │  Claxon  │  │   TUI    │
│(matbridge)│  │(matbridge)│  │ Android  │  │          │
└────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘
     │              │             │              │
     └──────┬───────┘             │              │
            │                     │              │
     ┌──────▼──────┐      ┌──────▼──────┐       │
     │ matterbridge│      │  websocket  │       │
     │   adapter   │      │   adapter   │       │
     └──────┬──────┘      └──────┬──────┘       │
            │                    │               │
            └────────┬───────────┘───────────────┘
                     │
              ┌──────▼──────┐
              │    router   │
              └──────┬──────┘
                     │
              ┌──────▼──────┐
              │  inber feed │
              └─────────────┘
```

## Adapters

- **matterbridge** — Connects to matterbridge API for Discord, Telegram, WhatsApp, Slack, IRC, etc.
- **websocket** — WebSocket server for Claxon Android and other direct clients
- **tui** — Terminal UI for local interaction

## Configuration

si reads its configuration from the environment, and every variable it reads is declared once in `internal/config/settings.go` with llm-bridge's `servicesettings`. A set variable si owns (`SI_FEED`, `SI_WS_ADDR`, `SI_WS_TOKEN`, `SI_JWT_SECRET`, `SI_DISCORD_TOKEN`, `SI_DISCORD_CHANNEL`, or one of those names with more after it) that is not declared stops si at start, naming it. `NATS_URL` and `LOGSTACK_URL` are declared and not owned.

The websocket adapter serves the declarations at `GET /settings`, behind the same gate as `/ws`: secrets are shown only as set or unset. `TestEveryEnvironmentVariableTheServiceReadsIsDeclared` fails on any `os.Getenv` the declarations do not name.

## Status

Early development. Feed protocol TBD (depends on inber's session server implementation).
