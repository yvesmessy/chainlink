package dbutil

import (
	"strings"

	"gorm.io/gorm"
)

func IsPostgresURL(url string) bool {
	return strings.HasPrefix(strings.ToLower(url), "postgres")
}

// IsPostgres returns true if the underlying database is postgres.
func IsPostgres(db *gorm.DB) bool {
	return db.Dialector.Name() == "postgres"
}

// SetTimezone sets the time zone to UTC
func SetTimezone(db *gorm.DB) error {
	if IsPostgres(db) {
		return db.Exec(`SET TIME ZONE 'UTC'`).Error
	}
	return nil
}
