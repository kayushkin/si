# SI Architecture

SI is the **single user-facing interface** for the entire system.

## Role

- The only thing Slava interacts with directly
- Per-agent chat tabs (claxon, oisin, brigid, etc.)
- Spawn/workspace/deploy visibility via event feed
- Subscribes to bus for all message flow

## Connections

```
Slava ↔ SI (HTTP/WebSocket)
SI ↔ Bus (HTTP publish + WS subscribe)
```

SI does NOT talk to inber directly. All messages flow through bus.

## Message Flow

### Inbound (user → agent)
1. Slava types in an agent's chat tab
2. SI publishes to bus topic `inbound` with `{agent: "<name>", text: "..."}` metadata
3. Bus queues it; inber server (subscribed to `inbound`) picks it up and routes to the agent

### Outbound (agent → user)
1. Agent responds → inber publishes to bus topic `outbound`
2. SI subscribes to `outbound`, filters by agent, renders in correct tab

### Events (system activity)
1. Inber publishes spawn/deploy/status events to bus topic `events`
2. SI subscribes to `events`, renders in orchestrator/activity feed

## What Changes

- **Remove**: `feed/inber_direct.go` — no more direct CLI calls
- **Keep**: `feed/bus.go` — this is the only feed, rename to just the bus connection
- **Add**: Chat UI with per-agent tabs
- **Add**: Subscribe to `outbound` + `events` topics (currently only `outbound` + `gateway`)
- **Keep**: Adapter interface — Discord/Signal/etc. adapters still publish through SI's router to bus

## Interfaces

### Feed (bus connection)
```go
// Feed connects SI to the bus. Publish inbound, subscribe outbound+events.
type Feed interface {
    Publish(msg Message) error        // → bus topic "inbound"
    Subscribe() <-chan BusEvent       // ← bus topics "outbound", "events"
    Close() error
}
```

### Adapter (external platforms)
```go
// Adapter bridges external platforms (Discord, Signal, etc.) to the router.
// Unchanged — adapters publish through SI which publishes to bus.
type Adapter interface {
    Name() string
    Start(ctx context.Context) error
    Send(msg Message) error
    Receive() <-chan Message
}
```
