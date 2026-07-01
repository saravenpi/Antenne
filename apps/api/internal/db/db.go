package db

import (
	"errors"
	"log"

	"github.com/saravenpi/antenne/internal/config"
	"github.com/saravenpi/antenne/internal/models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Open connects to Postgres, runs migrations, and seeds the bootstrap admin and
// settings row when they are missing.
func Open(cfg config.Config) (*gorm.DB, error) {
	gdb, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	if err := gdb.AutoMigrate(&models.Admin{}, &models.Track{}, &models.Settings{}); err != nil {
		return nil, err
	}

	if err := seedSettings(gdb); err != nil {
		return nil, err
	}
	if err := seedAdmin(gdb, cfg); err != nil {
		return nil, err
	}
	return gdb, nil
}

func seedSettings(gdb *gorm.DB) error {
	var count int64
	gdb.Model(&models.Settings{}).Count(&count)
	if count > 0 {
		return nil
	}
	return gdb.Create(&models.Settings{StationName: "Antenne", CrossfadeMs: 2000}).Error
}

func seedAdmin(gdb *gorm.DB, cfg config.Config) error {
	var count int64
	gdb.Model(&models.Admin{}).Count(&count)
	if count > 0 {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(cfg.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	admin := models.Admin{Username: cfg.AdminUsername, PasswordHash: string(hash)}
	if err := gdb.Create(&admin).Error; err != nil {
		return err
	}
	log.Printf("seeded bootstrap admin %q", cfg.AdminUsername)
	return nil
}

// ErrNotFound is a convenience wrapper for gorm.ErrRecordNotFound.
var ErrNotFound = gorm.ErrRecordNotFound

// IsNotFound reports whether err is a record-not-found error.
func IsNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
