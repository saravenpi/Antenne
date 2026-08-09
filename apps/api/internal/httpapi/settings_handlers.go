package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/saravenpi/poste/internal/db"
	"github.com/saravenpi/poste/internal/models"
)

// --- REST: settings ---

type socialLink struct {
	Platform string `json:"platform"`
	URL      string `json:"url"`
}

// parseSocials decodes the stored JSON list, always returning a non-nil slice so
// the API emits `[]` rather than `null`.
func parseSocials(raw string) []socialLink {
	out := []socialLink{}
	if raw == "" {
		return out
	}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

type settingsJSON struct {
	StationName string       `json:"stationName"`
	BannedWords []string     `json:"bannedWords"`
	SlowModeSec int          `json:"slowModeSec"`
	Background  string       `json:"background"`
	Logo        string       `json:"logo"`
	Socials     []socialLink `json:"socials"`
}

func toSettingsJSON(st models.Settings) settingsJSON {
	return settingsJSON{
		StationName: st.StationName,
		BannedWords: parseBannedWords(st.BannedWords),
		SlowModeSec: st.SlowModeSec,
		Background:  st.Background,
		Logo:        st.Logo,
		Socials:     parseSocials(st.Socials),
	}
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, toSettingsJSON(s.loadSettings()))
}

// handleAppearance is the PUBLIC subset of settings the listener page needs.
func (s *Server) handleAppearance(w http.ResponseWriter, r *http.Request) {
	st := s.loadSettings()
	writeJSON(w, http.StatusOK, map[string]any{
		"stationName": st.StationName,
		"background":  st.Background,
		"logo":        st.Logo,
		"socials":     parseSocials(st.Socials),
	})
}

type updateSettingsReq struct {
	StationName *string       `json:"stationName"`
	BannedWords *[]string     `json:"bannedWords"`
	SlowModeSec *int          `json:"slowModeSec"`
	Background  *string       `json:"background"`
	Logo        *string       `json:"logo"`
	Socials     *[]socialLink `json:"socials"`
}

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req updateSettingsReq
	if err := decode(w, r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	// Reject values that could turn admin-controlled appearance fields into an
	// injection vector: CSS that loads remote/script resources, and hrefs with
	// dangerous URL schemes. These are admin-only fields, so this is
	// defence-in-depth rather than a direct trust boundary.
	if req.Background != nil && !validBackground(*req.Background) {
		writeErr(w, http.StatusBadRequest, "invalid background value")
		return
	}
	if req.Logo != nil && !validImageRef(*req.Logo) {
		writeErr(w, http.StatusBadRequest, "invalid logo value")
		return
	}
	if req.Socials != nil {
		for _, l := range *req.Socials {
			if l.URL != "" && !validLinkURL(l.URL) {
				writeErr(w, http.StatusBadRequest, "invalid social link URL")
				return
			}
		}
	}

	st := s.loadSettings()
	if st.ID == 0 {
		if err := s.db.First(&st).Error; err != nil && !db.IsNotFound(err) {
			writeErr(w, http.StatusInternalServerError, "db error")
			return
		}
	}
	if req.StationName != nil {
		st.StationName = *req.StationName
	}
	if req.BannedWords != nil {
		st.BannedWords = strings.Join(*req.BannedWords, "\n")
	}
	if req.SlowModeSec != nil {
		st.SlowModeSec = *req.SlowModeSec
	}
	if req.Background != nil {
		st.Background = *req.Background
	}
	if req.Logo != nil {
		st.Logo = *req.Logo
	}
	if req.Socials != nil {
		b, _ := json.Marshal(*req.Socials)
		st.Socials = string(b)
	}
	if err := s.db.Save(&st).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, toSettingsJSON(st))
}

// --- appearance value validation ---

// dangerousCSS are substrings that must never appear in the inline `background`
// CSS the listener page renders (style="background: <value>").
var dangerousCSS = []string{"javascript:", "expression(", "@import", "</", "<", "\\"}

// validBackground allows plain colours/gradients and inline data:image
// backgrounds, but rejects anything that would break out of the style attribute
// or load a remote/script resource. An empty value clears the background.
func validBackground(s string) bool {
	l := strings.ToLower(strings.TrimSpace(s))
	if l == "" {
		return true
	}
	for _, bad := range dangerousCSS {
		if strings.Contains(l, bad) {
			return false
		}
	}
	// Every url(...) must reference an inline data:image, never a remote origin.
	rest := l
	for {
		idx := strings.Index(rest, "url(")
		if idx < 0 {
			break
		}
		arg := strings.TrimLeft(rest[idx+4:], " '\"")
		if !strings.HasPrefix(arg, "data:image/") {
			return false
		}
		rest = rest[idx+4:]
	}
	return true
}

// validImageRef allows an empty value, an inline data:image, or an http(s) URL —
// used for the logo, which is rendered as an <img> source.
func validImageRef(s string) bool {
	l := strings.ToLower(strings.TrimSpace(s))
	return l == "" ||
		strings.HasPrefix(l, "data:image/") ||
		strings.HasPrefix(l, "https://") ||
		strings.HasPrefix(l, "http://")
}

// validLinkURL restricts social link hrefs to safe schemes, blocking
// javascript:/data: and other script-bearing URLs.
func validLinkURL(s string) bool {
	l := strings.ToLower(strings.TrimSpace(s))
	return strings.HasPrefix(l, "https://") ||
		strings.HasPrefix(l, "http://") ||
		strings.HasPrefix(l, "mailto:")
}
