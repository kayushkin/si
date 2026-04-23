package websocket

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestAuthorizeLegacyBearer(t *testing.T) {
	t.Setenv("SI_WS_TOKEN", "shared-secret")
	t.Setenv("SI_JWT_SECRET", "")

	a := New(":0")
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
	t.Setenv("SI_WS_TOKEN", "shared-secret")
	t.Setenv("SI_JWT_SECRET", "")

	a := New(":0")
	req := httptest.NewRequest("GET", "/ws?token=shared-secret", nil)
	if !a.authorize(req) {
		t.Fatal("legacy query token should be accepted")
	}
}

func TestAuthorizeJWT(t *testing.T) {
	const secret = "jwt-secret"
	t.Setenv("SI_WS_TOKEN", "")
	t.Setenv("SI_JWT_SECRET", secret)

	a := New(":0")

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
	t.Setenv("SI_WS_TOKEN", "shared")
	t.Setenv("SI_JWT_SECRET", secret)

	a := New(":0")

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
	t.Setenv("SI_WS_TOKEN", "")
	t.Setenv("SI_JWT_SECRET", secret)

	a := New(":0")

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
	t.Setenv("SI_WS_TOKEN", "")
	t.Setenv("SI_JWT_SECRET", "")

	a := New(":0")
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
