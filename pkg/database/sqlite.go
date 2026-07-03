package database

import (
	"log/slog"
	"os"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func NewSQLite(options ...Option) (*gorm.DB, error) {
	filePath := os.Getenv("DB_FILE_PATH")

	if filePath == "" {
		filePath = "file::memory:?cache=shared"

		slog.Warn("sqlite.env_default", "var", "DB_FILE_PATH", "default", filePath) //nolint:sloglint // no ctx at construction
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
