package database

import (
	"log"
	"os"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func NewSQLite(options ...Option) (*gorm.DB, error) {
	filePath := os.Getenv("DB_FILE_PATH")

	if filePath == "" {
		log.Println("DB_FILE_PATH is not set, using system memory instead")

		filePath = "file::memory:?cache=shared"
	}

	opts := &opts{}
	for _, option := range options {
		option(opts)
	}

	if opts.config == nil {
		opts.config = &gorm.Config{}
	}

	return gorm.Open(sqlite.Open(filePath), opts.config)
}
