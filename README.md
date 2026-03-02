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

## Status

Early development. Feed protocol TBD (depends on inber's session server implementation).
