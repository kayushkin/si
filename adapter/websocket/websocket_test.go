package websocket

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestAuthorizeLegacyBearer(t *testing.T) {

	a := New(":0", Credentials{LegacyBearerToken: "shared-secret", JWTSecret: ""})
	req := httptest.NewRequest("GET", "/ws", nil)
	req.Header.Set("Authorization", "Bearer shared-secret")
	if !a.authorize(req) {
		t.Fatal("legacy bearer should be accepted")
	}

	req.Header.Set("Authorization", "Bearer wrong")
	if a.authorize(req) {
		t.Fatal("wrong legacy bearer must be rejected")
	}
}

func TestAuthorizeLegacyQueryToken(t *testing.T) {

	a := New(":0", Credentials{LegacyBearerToken: "shared-secret", JWTSecret: ""})
	req := httptest.NewRequest("GET", "/ws?token=shared-secret", nil)
	if !a.authorize(req) {
		t.Fatal("legacy query token should be accepted")
	}
}

func TestAuthorizeJWT(t *testing.T) {
	const secret = "jwt-secret"

	a := New(":0", Credentials{LegacyBearerToken: "", JWTSecret: secret})

	// Valid JWT with aud=si.
	valid := mintToken(t, secret, "si", time.Hour)
	req := httptest.NewRequest("GET", "/ws", nil)
	req.Header.Set("Authorization", "Bearer "+valid)
	if !a.authorize(req) {
		t.Fatal("valid JWT with aud=si must be accepted")
	}

	// Wrong audience.
	wrongAud := mintToken(t, secret, "other", time.Hour)
	req.Header.Set("Authorization", "Bearer "+wrongAud)
	if a.authorize(req) {
		t.Fatal("JWT with aud=other must be rejected")
	}

	// Expired.
	expired := mintToken(t, secret, "si", -time.Hour)
	req.Header.Set("Authorization", "Bearer "+expired)
	if a.authorize(req) {
		t.Fatal("expired JWT must be rejected")
	}

	// Wrong secret.
	wrongSig := mintToken(t, "other-secret", "si", time.Hour)
	req.Header.Set("Authorization", "Bearer "+wrongSig)
	if a.authorize(req) {
		t.Fatal("JWT signed with wrong secret must be rejected")
	}

	// Garbage.
	req.Header.Set("Authorization", "Bearer not-a-jwt")
	if a.authorize(req) {
		t.Fatal("garbage token must be rejected")
	}
}

func TestAuthorizeBothEnabled(t *testing.T) {
	const secret = "jwt-secret"

	a := New(":0", Credentials{LegacyBearerToken: "shared", JWTSecret: secret})

	// Legacy still works.
	req := httptest.NewRequest("GET", "/ws", nil)
	req.Header.Set("Authorization", "Bearer shared")
	if !a.authorize(req) {
		t.Fatal("legacy bearer should still work when JWT is also configured")
	}

	// JWT also works.
	tok := mintToken(t, secret, "si", time.Hour)
	req.Header.Set("Authorization", "Bearer "+tok)
	if !a.authorize(req) {
		t.Fatal("JWT should be accepted when both configured")
	}

	// Wrong everything fails.
	req.Header.Set("Authorization", "Bearer nope")
	if a.authorize(req) {
		t.Fatal("unknown bearer must be rejected")
	}
}

// The kayushkin.com mint endpoint uses jwt.RegisteredClaims with an Audience
// field of type jwt.ClaimStrings, which marshals as a JSON array when it has
// a single entry. Verify si accepts both string and array forms.
func TestAuthorizeJWTArrayAudience(t *testing.T) {
	const secret = "jwt-secret"

	a := New(":0", Credentials{LegacyBearerToken: "", JWTSecret: secret})

	claims := jwt.RegisteredClaims{
		Issuer:    "kayushkin.com",
		Audience:  jwt.ClaimStrings{"si"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("mint: %v", err)
	}

	req := httptest.NewRequest("GET", "/ws", nil)
	req.Header.Set("Authorization", "Bearer "+s)
	if !a.authorize(req) {
		t.Fatal("JWT with array-form audience containing 'si' must be accepted")
	}

	// Array without si → reject.
	claims.Audience = jwt.ClaimStrings{"other", "not-si"}
	tok = jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, _ = tok.SignedString([]byte(secret))
	req.Header.Set("Authorization", "Bearer "+s)
	if a.authorize(req) {
		t.Fatal("JWT with array audience not containing 'si' must be rejected")
	}
}

func TestAuthorizeDevOpen(t *testing.T) {

	a := New(":0", Credentials{LegacyBearerToken: "", JWTSecret: ""})
	req := httptest.NewRequest("GET", "/ws", nil)
	if !a.authorize(req) {
		t.Fatal("dev-open mode must accept anything")
	}
}

func mintToken(t *testing.T, secret, aud string, ttl time.Duration) string {
	t.Helper()
	claims := jwt.MapClaims{
		"aud": aud,
		"exp": time.Now().Add(ttl).Unix(),
		"iat": time.Now().Unix(),
		"iss": "kayushkin.com",
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	return s
}

// GET /settings answers only a client /ws would admit, and PUT is not served.
func TestSettingsSitBehindTheWebsocketGate(t *testing.T) {
	settings := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("described")) })

	gated := New(":0", Credentials{LegacyBearerToken: "shared"})
	gated.SetSettingsHandler(settings)
	for _, c := range []struct {
		method, authorization string
		want                  int
	}{
		{http.MethodGet, "", http.StatusUnauthorized},
		{http.MethodGet, "Bearer wrong", http.StatusUnauthorized},
		{http.MethodGet, "Bearer shared", http.StatusOK},
		{http.MethodPut, "Bearer shared", http.StatusMethodNotAllowed},
	} {
		req := httptest.NewRequest(c.method, "/settings", nil)
		if c.authorization != "" {
			req.Header.Set("Authorization", c.authorization)
		}
		recorder := httptest.NewRecorder()
		gated.routes().ServeHTTP(recorder, req)
		if recorder.Code != c.want {
			t.Errorf("%s /settings with %q = %d, want %d", c.method, c.authorization, recorder.Code, c.want)
		}
	}

	open := New(":0", Credentials{})
	open.SetSettingsHandler(settings)
	recorder := httptest.NewRecorder()
	open.routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/settings", nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "described" {
		t.Errorf("GET /settings with no credentials configured = %d %q", recorder.Code, recorder.Body)
	}

	unset := New(":0", Credentials{})
	recorder = httptest.NewRecorder()
	unset.routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/settings", nil))
	if recorder.Code != http.StatusNotFound {
		t.Errorf("GET /settings with no handler set = %d, want 404", recorder.Code)
	}
}
