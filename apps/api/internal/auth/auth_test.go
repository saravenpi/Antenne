package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestAuthorizeFromCookie(t *testing.T) {
	s := New("a-sufficiently-long-test-secret")
	id := uuid.New()
	tok, err := s.Issue(id)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/api/chat/ws", nil)
	r.AddCookie(&http.Cookie{Name: CookieName, Value: tok})

	got, err := s.Authorize(r)
	if err != nil {
		t.Fatalf("authorize from cookie: %v", err)
	}
	if got != id {
		t.Fatalf("got %v, want %v", got, id)
	}
}

func TestAuthorizeFromBearer(t *testing.T) {
	s := New("a-sufficiently-long-test-secret")
	id := uuid.New()
	tok, _ := s.Issue(id)

	r := httptest.NewRequest(http.MethodGet, "/api/tracks", nil)
	r.Header.Set("Authorization", "Bearer "+tok)

	got, err := s.Authorize(r)
	if err != nil || got != id {
		t.Fatalf("authorize from bearer: got %v err %v", got, err)
	}
}

func TestAuthorizeIgnoresQueryToken(t *testing.T) {
	s := New("a-sufficiently-long-test-secret")
	id := uuid.New()
	tok, _ := s.Issue(id)

	// A token in the URL must no longer authenticate (it must not leak via logs).
	r := httptest.NewRequest(http.MethodGet, "/api/chat/ws?token="+tok, nil)
	if _, err := s.Authorize(r); err == nil {
		t.Fatal("expected query-param token to be rejected")
	}
}

func TestSetCookieAttributes(t *testing.T) {
	s := New("a-sufficiently-long-test-secret")
	rec := httptest.NewRecorder()
	s.SetCookie(rec, "tok", true)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	c := cookies[0]
	if c.Name != CookieName || c.Value != "tok" {
		t.Fatalf("unexpected cookie name/value: %q=%q", c.Name, c.Value)
	}
	if !c.HttpOnly {
		t.Error("cookie must be HttpOnly")
	}
	if !c.Secure {
		t.Error("cookie must be Secure when secure=true")
	}
	if c.SameSite != http.SameSiteStrictMode {
		t.Error("cookie must be SameSite=Strict")
	}
}

func TestClearCookieExpires(t *testing.T) {
	s := New("a-sufficiently-long-test-secret")
	rec := httptest.NewRecorder()
	s.ClearCookie(rec, false)
	c := rec.Result().Cookies()[0]
	if c.MaxAge >= 0 {
		t.Fatalf("expected negative MaxAge to expire cookie, got %d", c.MaxAge)
	}
}
