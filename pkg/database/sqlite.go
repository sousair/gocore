package database

import (
	"log"
	"os"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func NewSQLite(cfg *gorm.Config) (*gorm.DB, error) {
	filePath := os.Getenv("DB_FILE_PATH")

	if filePath == "" {
		log.Println("DB_FILE_PATH is not set, using system memory instead")

		filePath = "file::memory:?cache=shared"
	}

	cfg = setupConfig(cfg)

	return gorm.Open(sqlite.Open(filePath), cfg)
}
