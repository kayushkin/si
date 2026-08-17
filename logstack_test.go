package si

import (
	"testing"
	"time"

	logstackmodels "github.com/kayushkin/logstack/models"
)

// healthcheckEvent rebuilds the message the feed makes out of a healthcheck
// status change, field for field: feed/nats.go's health.> subscription and
// feed/bus.go's "events" topic both construct exactly this — the raw payload as
// text, the "events" channel, and nothing else. The router then hands it to
// publish() as EventOutbound, because outbound is its direction of travel.
//
// It is the fixture that matters most here: as of 2026-08-17, 139,728 of the
// 147,204 entries si has filed as outbound are this shape -- 94.9%, and it
// read 138,441 of 145,813 for the same 94.9% when this was written. The
// share is the stable fact; the two absolutes only track how long the box
// has been running, so they are dated rather than corrected.
func healthcheckEvent() Event {
	return Event{
		Type: EventOutbound,
		Message: Message{
			Text:      `{"type":"healthcheck.status_change","service":"kayushkin","old_status":"degraded","new_status":"down"}`,
			Channel:   "events",
			Timestamp: time.Now(),
		},
	}
}

// A healthcheck ping is not a completed agent turn, and logstack's usage
// readers select TypeOutbound and nothing else. Filing plumbing there is how
// the outbound bucket came to be 94.9% noise.
func TestRepublishedHealthEventIsNotCountedAsATurn(t *testing.T) {
	got := entryType(healthcheckEvent())
	if got == logstackmodels.TypeOutbound {
		t.Fatalf("healthcheck event typed %q — Usage and MaxUsage read that bucket as billable conversation", got)
	}
	if got != logstackmodels.TypeRouting {
		t.Fatalf("healthcheck event typed %q, want %q", got, logstackmodels.TypeRouting)
	}
}

// The other half of the same predicate: a real turn must keep its bucket. A fix
// that silences the noise by dropping everything would report zero just as
// loudly as the noise did.
func TestAgentTurnIsStillCounted(t *testing.T) {
	e := Event{Type: EventOutbound, Message: Message{
		Text:         "the answer is 4",
		Agent:        "brigid",
		Orchestrator: "inber",
		Timestamp:    time.Now(),
	}}
	if got := entryType(e); got != logstackmodels.TypeOutbound {
		t.Fatalf("agent turn typed %q, want %q — a turn outside that bucket contributes no tokens and no dollars", got, logstackmodels.TypeOutbound)
	}
}

// A human message names its speaker with Author rather than Agent, and si has
// always resolved one to the other. The direction still decides the bucket.
func TestHumanMessageKeepsItsDirection(t *testing.T) {
	e := Event{Type: EventInbound, Message: Message{
		Text:      "hello",
		Author:    "nrmh",
		Channel:   "discord:143132977210195968",
		Timestamp: time.Now(),
	}}
	if got := entryType(e); got != logstackmodels.TypeInbound {
		t.Fatalf("human inbound typed %q, want %q", got, logstackmodels.TypeInbound)
	}
}

// EventGateway is si's own word and logstack has never had a type by that name,
// so a gateway event used to be filed under a string no reader recognises.
func TestGatewayEventUsesAKnownType(t *testing.T) {
	e := Event{Type: EventGateway, Message: Message{
		Text:      "session started",
		Agent:     "claxon",
		Timestamp: time.Now(),
	}}
	if got := entryType(e); got != logstackmodels.TypeLifecycle {
		t.Fatalf("gateway event typed %q, want %q", got, logstackmodels.TypeLifecycle)
	}
}

// The whole defect was one vocabulary's string landing in another's field, so
// pin that every answer entryType can give is a type logstack actually defines.
// A future edit that returns string(e.Type) again fails here.
func TestEveryTypeEntryTypeReturnsIsOneLogstackDefines(t *testing.T) {
	known := map[string]bool{
		logstackmodels.TypeInbound:    true,
		logstackmodels.TypeOutbound:   true,
		logstackmodels.TypeMessage:    true,
		logstackmodels.TypeToolCall:   true,
		logstackmodels.TypeToolResult: true,
		logstackmodels.TypeError:      true,
		logstackmodels.TypeMetrics:    true,
		logstackmodels.TypeLifecycle:  true,
		logstackmodels.TypeRouting:    true,
	}

	for _, direction := range []EventType{EventInbound, EventOutbound, EventGateway} {
		for _, m := range []Message{
			{},
			{Agent: "brigid"},
			{Author: "nrmh"},
			{Channel: "events", Text: "{}"},
			{Agent: "brigid", Author: "nrmh", Orchestrator: "inber", Channel: "discord:1"},
		} {
			got := entryType(Event{Type: direction, Message: m})
			if !known[got] {
				t.Fatalf("entryType(%s, %+v) = %q, which logstack does not define — no reader will ever select it", direction, m, got)
			}
		}
	}
}

// logstack files an entry under entry.Orchestrator and both usage readers group
// by it. si knew the orchestrator on 7,371 of its 7,388 attributed entries and
// sent it only inside Content, so every entry it ever wrote landed in
// unknown.jsonl and by_orchestrator came back empty for the whole box.
func TestEntryCarriesTheOrchestratorLogstackFilesItUnder(t *testing.T) {
	entry := buildEntry(Event{Type: EventOutbound, Message: Message{
		Text:         "done",
		Agent:        "brigid",
		Orchestrator: "inber",
		Timestamp:    time.Now(),
	}})

	if entry.Orchestrator != "inber" {
		t.Fatalf("entry.Orchestrator = %q, want %q — logstack names the file after this field and Usage groups by it", entry.Orchestrator, "inber")
	}
	if entry.Agent != "brigid" {
		t.Fatalf("entry.Agent = %q, want %q", entry.Agent, "brigid")
	}
}

// A message with no timestamp must still be datable, or it sorts to the epoch
// and drops out of every windowed query.
func TestUndatedMessageIsStamped(t *testing.T) {
	entry := buildEntry(Event{Type: EventOutbound, Message: Message{Text: "x", Agent: "brigid"}})
	if entry.Timestamp.IsZero() {
		t.Fatal("entry timestamp is zero — a windowed query would never see it")
	}
}
