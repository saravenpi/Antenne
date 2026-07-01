package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Admin is a station operator. Mono-station: usually a single admin account.
type Admin struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	Username     string    `gorm:"uniqueIndex;not null" json:"username"`
	PasswordHash string    `gorm:"not null" json:"-"`
	CreatedAt    time.Time `json:"createdAt"`
}

// BeforeCreate assigns a UUID if one is not set.
func (a *Admin) BeforeCreate(*gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return nil
}

// Track is an uploaded audio file that feeds the 24/7 playlist.
type Track struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	Title       string    `gorm:"not null" json:"title"`
	Artist      string    `json:"artist"`
	Filename    string    `gorm:"not null" json:"-"` // path on disk, relative to StorageDir
	DurationSec float64   `json:"durationSec"`
	Position    int       `gorm:"index" json:"position"` // playlist order
	CreatedAt   time.Time `json:"createdAt"`
}

func (t *Track) BeforeCreate(*gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return nil
}

// Settings holds the single-row mono-station configuration.
type Settings struct {
	ID          uint   `gorm:"primaryKey" json:"-"`
	StationName string `gorm:"default:'Antenne'" json:"stationName"`
	Shuffle     bool   `json:"shuffle"`
	CrossfadeMs int    `gorm:"default:2000" json:"crossfadeMs"`
	// BannedWords is stored as a newline-separated list; expose the parsed slice
	// via BannedWordsList.
	BannedWords string `json:"-"`
	SlowModeSec int    `gorm:"default:2" json:"slowModeSec"`
	// Background is a CSS `background` value for the public listener page
	// (color, gradient, or `url(...) center/cover`). Empty = default theme.
	Background string `json:"-"`
}

// Clip is a recorded slice of the live broadcast, encoded to MP3.
type Clip struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	Title       string    `json:"title"`
	Filename    string    `gorm:"not null" json:"-"` // <uuid>.mp3 under ClipsDir
	DurationSec float64   `json:"durationSec"`
	CreatedAt   time.Time `json:"createdAt"`
}

func (c *Clip) BeforeCreate(*gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return nil
}

// ChatMessage is a single live-chat message. IP is kept server-side only.
type ChatMessage struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	Name      string    `json:"name"`
	Body      string    `json:"body"`
	IP        string    `json:"-"`
	Deleted   bool      `json:"-"`
	CreatedAt time.Time `json:"createdAt"`
}

func (m *ChatMessage) BeforeCreate(*gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}

// Ban blocks an IP from posting to the chat entirely.
type Ban struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	IP        string    `gorm:"uniqueIndex" json:"ip"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"createdAt"`
}

func (b *Ban) BeforeCreate(*gorm.DB) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	return nil
}

// Restriction shadow-bans an IP: their messages are echoed back to them only.
type Restriction struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	IP        string    `gorm:"uniqueIndex" json:"ip"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"createdAt"`
}

func (r *Restriction) BeforeCreate(*gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return nil
}
