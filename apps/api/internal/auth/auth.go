package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type ctxKey string

const adminIDKey ctxKey = "adminID"

// CookieName is the name of the HttpOnly cookie that carries the admin token.
const CookieName = "antenne_token"

// tokenTTL is how long an issued token (and its cookie) remains valid.
const tokenTTL = 7 * 24 * time.Hour

// Service issues and verifies admin JWTs.
type Service struct {
	secret []byte
}

func New(secret string) *Service {
	return &Service{secret: []byte(secret)}
}

// Issue returns a signed JWT for the given admin, valid for tokenTTL.
func (s *Service) Issue(adminID uuid.UUID) (string, error) {
	claims := jwt.RegisteredClaims{
		Subject:   adminID.String(),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(tokenTTL)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

// SetCookie writes the token as a hardened, HttpOnly session cookie. secure must
// be true in production (HTTPS) so the cookie is never sent over plain HTTP;
// SameSite=Strict blocks it from cross-site requests (CSRF / CSWSH defence).
func (s *Service) SetCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(tokenTTL.Seconds()),
	})
}

// ClearCookie expires the session cookie (logout).
func (s *Service) ClearCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

// Parse validates a token string and returns the admin ID.
func (s *Service) Parse(tokenStr string) (uuid.UUID, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &jwt.RegisteredClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return s.secret, nil
	})
	if err != nil {
		return uuid.Nil, err
	}
	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok || !token.Valid {
		return uuid.Nil, errors.New("invalid token")
	}
	return uuid.Parse(claims.Subject)
}

// Middleware rejects requests without a valid Bearer token and stores the admin
// ID in the request context.
func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := s.fromRequest(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), adminIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// fromRequest extracts and validates the token from the Authorization header
// (for API clients) or the HttpOnly session cookie (for the browser, including
// same-origin WebSocket handshakes). The token is never read from the URL, so it
// cannot leak via logs, history, or Referer.
func (s *Service) fromRequest(r *http.Request) (uuid.UUID, error) {
	raw := ""
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		raw = strings.TrimPrefix(h, "Bearer ")
	} else if c, err := r.Cookie(CookieName); err == nil {
		raw = c.Value
	}
	if raw == "" {
		return uuid.Nil, errors.New("missing token")
	}
	return s.Parse(raw)
}

// Authorize validates a request outside the middleware chain (e.g. WebSocket).
func (s *Service) Authorize(r *http.Request) (uuid.UUID, error) {
	return s.fromRequest(r)
}

// AdminID returns the authenticated admin ID from a request context.
func AdminID(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(adminIDKey).(uuid.UUID)
	return id, ok
}
