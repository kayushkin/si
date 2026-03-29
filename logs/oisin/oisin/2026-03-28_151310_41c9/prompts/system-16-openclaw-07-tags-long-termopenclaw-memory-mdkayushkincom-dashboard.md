# openclaw (0.7, tags: long-term,openclaw-memory-md,kayushkincom-dashboard)

*~266 tokens*

- Repo: github.com/kayushkin/si — stateless protocol adapter for inber
- **Stateless** — no history, no session tracking, no agent routing. Just protocol translation.
- Translates platform formats (Discord, WebSocket) ↔ bus messages
- Publishes all adapter messages to `inbound` topic with channel metadata
- Subscribes to `outbound` topic, routes responses to matching adapter by channel
- **NO INBER ON SERVER** — inber only runs locally on WSL, accessed via bus
- Binary: `~/bin/si`
- Feed modes: `SI_FEED=bus` (default), `SI_FEED=echo` (test)
- WebSocket adapter on :8090 — broadcasts events to all WS clients
- REST API: `GET /api/status`
- All routing decisions happen in inber (via bus-agent), not si
- Session tracking belongs in inber, channel→agent mapping belongs in inber

