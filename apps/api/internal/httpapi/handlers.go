package httpapi

import (
	"net/http"

	"github.com/saravenpi/antenne/internal/auth"
	"github.com/saravenpi/antenne/internal/models"
	"golang.org/x/crypto/bcrypt"
)

// --- Auth ---

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := decode(w, r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	var admin models.Admin
	if err := s.db.Where("username = ?", req.Username).First(&admin).Error; err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(req.Password)) != nil {
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	token, err := s.auth.Issue(admin.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not issue token")
		return
	}
	// The browser authenticates via the HttpOnly cookie; the token is not exposed
	// to JavaScript (defeats XSS token theft) and never appears in a URL.
	s.auth.SetCookie(w, token, s.cfg.IsProd())
	writeJSON(w, http.StatusOK, map[string]string{"username": admin.Username})
}

// handleMe returns the authenticated admin's identity. The client uses it to
// discover login state, since the HttpOnly cookie is not readable from JS.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	id, ok := auth.AdminID(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var admin models.Admin
	if err := s.db.First(&admin, "id = ?", id).Error; err != nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"username": admin.Username})
}

// handleLogout clears the session cookie. It is public and idempotent: clearing
// a cookie needs no valid session.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.auth.ClearCookie(w, s.cfg.IsProd())
	w.WriteHeader(http.StatusNoContent)
}
