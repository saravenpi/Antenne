package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/saravenpi/antenne/internal/db"
	"github.com/saravenpi/antenne/internal/models"
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
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
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
