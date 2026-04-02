package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// ---------------------------------------------------------------------------
// auth.go — Wrap
// ---------------------------------------------------------------------------

func TestWrap_NilAuthFunc_PassesThrough(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	handler := Wrap(inner, nil)
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("expected body 'ok', got %q", rec.Body.String())
	}
}

func TestWrap_AuthReturnsTrue_AllowsRequest(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("allowed"))
	})
	allow := func(w http.ResponseWriter, r *http.Request) bool { return true }
	handler := Wrap(inner, allow)

	req := httptest.NewRequest("GET", "/page", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "allowed" {
		t.Fatalf("expected body 'allowed', got %q", rec.Body.String())
	}
}

func TestWrap_AuthReturnsFalse_Returns401(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("inner handler should not be called")
	})
	deny := func(w http.ResponseWriter, r *http.Request) bool { return false }
	handler := Wrap(inner, deny)

	req := httptest.NewRequest("GET", "/secret", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestWrap_TokenQueryParam_RedirectsToStripToken(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("inner handler should not be called on redirect")
	})
	// Auth succeeds but token is in query — should redirect to strip it.
	allow := func(w http.ResponseWriter, r *http.Request) bool { return true }
	handler := Wrap(inner, allow)

	req := httptest.NewRequest("GET", "/dashboard?token=abc123", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if loc != "/dashboard" {
		t.Fatalf("expected redirect to '/dashboard', got %q", loc)
	}
}

func TestWrap_NoTokenParam_PassesThroughAfterAuth(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("served"))
	})
	allow := func(w http.ResponseWriter, r *http.Request) bool { return true }
	handler := Wrap(inner, allow)

	req := httptest.NewRequest("GET", "/page?other=value", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "served" {
		t.Fatalf("expected body 'served', got %q", rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// token_auth.go — TokenAuth, generateToken
// ---------------------------------------------------------------------------

func TestTokenAuth_ReturnsNonEmptyTokenAndNonNilFunc(t *testing.T) {
	token, fn := TokenAuth()
	if token == "" {
		t.Fatal("token should not be empty")
	}
	if fn == nil {
		t.Fatal("AuthFunc should not be nil")
	}
}

func TestGenerateToken_Is32HexChars(t *testing.T) {
	token := generateToken()
	if len(token) != 32 {
		t.Fatalf("expected 32 chars, got %d (%q)", len(token), token)
	}
	for _, c := range token {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Fatalf("token contains non-hex char %q in %q", string(c), token)
		}
	}
}

func TestTokenAuth_TwoCallsProduceDifferentTokens(t *testing.T) {
	t1, _ := TokenAuth()
	t2, _ := TokenAuth()
	if t1 == t2 {
		t.Fatalf("two TokenAuth calls produced the same token: %s", t1)
	}
}

func TestTokenAuthFunc_RejectsNoToken(t *testing.T) {
	_, fn := TokenAuth()
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	if fn(rec, req) {
		t.Fatal("expected auth to reject request with no token")
	}
}

func TestTokenAuthFunc_AcceptsValidQueryParam_SetsCookie(t *testing.T) {
	token, fn := TokenAuth()
	req := httptest.NewRequest("GET", "/?token="+token, nil)
	rec := httptest.NewRecorder()

	if !fn(rec, req) {
		t.Fatal("expected auth to accept valid query param token")
	}

	// Verify cookie was set.
	cookies := rec.Result().Cookies()
	var found *http.Cookie
	for _, c := range cookies {
		if c.Name == "godom_token" {
			found = c
			break
		}
	}
	if found == nil {
		t.Fatal("expected godom_token cookie to be set")
	}
	if found.Value != token {
		t.Fatalf("cookie value %q != token %q", found.Value, token)
	}
}

func TestTokenAuthFunc_AcceptsValidCookie(t *testing.T) {
	token, fn := TokenAuth()
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "godom_token", Value: token})
	rec := httptest.NewRecorder()

	if !fn(rec, req) {
		t.Fatal("expected auth to accept valid cookie")
	}
}

func TestTokenAuthFunc_RejectsWrongQueryParam(t *testing.T) {
	_, fn := TokenAuth()
	req := httptest.NewRequest("GET", "/?token=wrongvalue", nil)
	rec := httptest.NewRecorder()

	if fn(rec, req) {
		t.Fatal("expected auth to reject wrong query param token")
	}
}

func TestTokenAuthFunc_RejectsWrongCookie(t *testing.T) {
	_, fn := TokenAuth()
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "godom_token", Value: "badtoken"})
	rec := httptest.NewRecorder()

	if fn(rec, req) {
		t.Fatal("expected auth to reject wrong cookie")
	}
}

func TestTokenAuthFunc_CookieProperties(t *testing.T) {
	token, fn := TokenAuth()
	req := httptest.NewRequest("GET", "/?token="+token, nil)
	rec := httptest.NewRecorder()
	fn(rec, req)

	cookies := rec.Result().Cookies()
	var found *http.Cookie
	for _, c := range cookies {
		if c.Name == "godom_token" {
			found = c
			break
		}
	}
	if found == nil {
		t.Fatal("expected godom_token cookie to be set")
	}
	if !found.HttpOnly {
		t.Error("cookie should be HttpOnly")
	}
	if found.SameSite != http.SameSiteStrictMode {
		t.Errorf("cookie SameSite should be Strict, got %v", found.SameSite)
	}
	if found.Path != "/" {
		t.Errorf("cookie Path should be '/', got %q", found.Path)
	}
}

// [COVERAGE GAP] generateToken's error path (rand.Read failure) calls log.Fatalf,
// which exits the process. Testing this would require replacing crypto/rand or
// intercepting os.Exit, which is fragile. The error path is not covered.
