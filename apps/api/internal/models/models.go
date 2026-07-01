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
}
