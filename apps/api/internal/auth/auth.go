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

// Service issues and verifies admin JWTs.
type Service struct {
	secret []byte
}

func New(secret string) *Service {
	return &Service{secret: []byte(secret)}
}

// Issue returns a signed JWT for the given admin, valid for 7 days.
func (s *Service) Issue(adminID uuid.UUID) (string, error) {
	claims := jwt.RegisteredClaims{
		Subject:   adminID.String(),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
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

// fromRequest extracts and validates the token from the Authorization header or,
// for WebSocket upgrades, the `token` query parameter.
func (s *Service) fromRequest(r *http.Request) (uuid.UUID, error) {
	raw := ""
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		raw = strings.TrimPrefix(h, "Bearer ")
	} else if q := r.URL.Query().Get("token"); q != "" {
		raw = q
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
