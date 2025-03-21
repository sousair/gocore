package database

import (
	"fmt"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const POSTGRES_DSN = "postgresql://%s:%s@%s:%s/%s"

func NewPostgres(options ...Option) (*gorm.DB, error) {
	username, ok := os.LookupEnv("DB_USERNAME")
	if !ok {
		return nil, ErrUsernameNotSet
	}

	password, ok := os.LookupEnv("DB_PASSWORD")
	if !ok {
		return nil, ErrPasswordNotSet
	}

	host, ok := os.LookupEnv("DB_HOST")
	if !ok {
		return nil, ErrHostNotSet
	}

	port, ok := os.LookupEnv("DB_PORT")
	if !ok {
		port = "5432"
	}

	name, ok := os.LookupEnv("DB_NAME")
	if !ok {
		return nil, ErrNameNotSet
	}

	connStr := fmt.Sprintf(POSTGRES_DSN, username, password, host, port, name)

	opts := opts{}
	for _, option := range options {
		option(&opts)
	}

	if opts.config == nil {
		opts.config = defaultConfig
	}

	return gorm.Open(postgres.Open(connStr), opts.config)
}
