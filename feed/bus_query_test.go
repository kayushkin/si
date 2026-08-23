package feed

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	si "github.com/kayushkin/si"
)

// A bus token is a credential and is routinely base64, which means it can carry
// "+", "/" and "=". Concatenated into a query string those are not inert: a
// receiving server decodes "+" to a space before it compares the token, so a
// perfectly valid credential fails authentication with an error that names
// nothing. "&" is worse — it ends the parameter and starts another one.
// Every character here is LEGAL in a URL, which is the whole point: the request
// is sent successfully and the damage is silent. A probe carrying a space or a
// "#" reddens the pin too, but for the wrong reason — net/http refuses to parse
// the URL at all, so the failure reads "never reached the server" and a reader
// learns nothing about the value being mangled. The loud case is covered
// separately by TestASpaceInTheTokenIsNotWhatThisPinIsAbout.
const probeToken = "tok+en/with=all&injected=1"

const probeConsumer = "si&role=admin"

// requestURIRecorder answers every request and keeps the raw request line.
// The assertion below reads RequestURI rather than URL.Path or a re-encoded
// URL.String(): those are reconstructed from a parsed value and read the same
// whether or not the client escaped anything.
func requestURIRecorder(t *testing.T, seen chan<- string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case seen <- r.RequestURI:
		default:
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"items":[]}`))
	}))
}

// assertParameterRoundTrips is the property under test: the server must read
// back exactly the value the client sent, and no parameter the client never set
// may appear. The second half is what an unescaped "&" violates.
func assertParameterRoundTrips(t *testing.T, requestURI, parameter, want string, allowed ...string) {
	t.Helper()
	parsed, err := url.ParseRequestURI(requestURI)
	if err != nil {
		t.Fatalf("server could not parse the request line %q: %v", requestURI, err)
	}
	values := parsed.Query()
	if got := values.Get(parameter); got != want {
		t.Errorf("parameter %q round-tripped as %q, want %q\n  request line: %s",
			parameter, got, want, requestURI)
	}
	permitted := map[string]bool{parameter: true}
	for _, name := range allowed {
		permitted[name] = true
	}
	for name := range values {
		if !permitted[name] {
			t.Errorf("parameter %q appeared and the client never set it — a value carried a separator\n  request line: %s",
				name, requestURI)
		}
	}
	if parsed.Fragment != "" {
		t.Errorf("a value truncated the query into fragment %q\n  request line: %s", parsed.Fragment, requestURI)
	}
}

func TestPublishSendsTheTokenItWasGiven(t *testing.T) {
	seen := make(chan string, 4)
	server := requestURIRecorder(t, seen)
	defer server.Close()

	feed := NewBusFeed(BusFeedConfig{BusURL: server.URL, Token: probeToken})
	go feed.publishLoop()
	defer feed.Close()

	feed.outbound <- si.Message{Channel: "test", Author: "pin", Text: "hello"}

	select {
	case requestURI := <-seen:
		assertParameterRoundTrips(t, requestURI, "token", probeToken)
	case <-time.After(5 * time.Second):
		t.Fatal("publish never reached the server")
	}
}

func TestAcknowledgeSendsTheTokenItWasGiven(t *testing.T) {
	seen := make(chan string, 4)
	server := requestURIRecorder(t, seen)
	defer server.Close()

	feed := NewBusFeed(BusFeedConfig{BusURL: server.URL, Token: probeToken})
	defer feed.Close()

	feed.ack("outbound", 1)

	select {
	case requestURI := <-seen:
		assertParameterRoundTrips(t, requestURI, "token", probeToken)
	case <-time.After(5 * time.Second):
		t.Fatal("ack never reached the server")
	}
}

// The subscribe URL is dialled as a WebSocket, so the pin reads the request line
// off a plain HTTP server: the handshake is an HTTP GET and the dial failing
// afterwards is irrelevant to what was sent.
func TestSubscribeSendsTheConsumerAndTokenItWasGiven(t *testing.T) {
	seen := make(chan string, 4)
	server := requestURIRecorder(t, seen)
	defer server.Close()

	feed := NewBusFeed(BusFeedConfig{BusURL: server.URL, Token: probeToken, Consumer: probeConsumer})
	defer feed.Close()

	go feed.subscribe()

	select {
	case requestURI := <-seen:
		assertParameterRoundTrips(t, requestURI, "token", probeToken, "consumer", "topics")
		assertParameterRoundTrips(t, requestURI, "consumer", probeConsumer, "token", "topics")
		parsed, _ := url.ParseRequestURI(requestURI)
		if got := parsed.Query().Get("topics"); got != "outbound,events,gateway" {
			t.Errorf("topics round-tripped as %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("subscribe never reached the server")
	}
}

// The loud half of the same defect, pinned separately so the silent half above
// cannot be satisfied by it. A space cannot survive concatenation into a URL at
// all: net/http rejects the request before it is sent, so an operator sees a
// transport error rather than a rejected credential.
func TestASpaceInTheTokenIsNotWhatThisPinIsAbout(t *testing.T) {
	seen := make(chan string, 4)
	server := requestURIRecorder(t, seen)
	defer server.Close()

	feed := NewBusFeed(BusFeedConfig{BusURL: server.URL, Token: "tok en"})
	defer feed.Close()

	feed.ack("outbound", 1)

	select {
	case requestURI := <-seen:
		assertParameterRoundTrips(t, requestURI, "token", "tok en")
	case <-time.After(5 * time.Second):
		t.Fatal("a token with a space never reached the server")
	}
}
