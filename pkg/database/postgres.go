package database

import (
	"fmt"
	"log"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const POSTGRES_DSN = "host=%s port=%s user=%s password=%s dbname=%s sslmode=%s"

const defaultSSLMode = "require"

func NewPostgres(options ...Option) (*gorm.DB, error) {
	host, ok := os.LookupEnv("DB_HOST")
	if !ok {
		return nil, ErrHostNotSet
	}

	port, ok := os.LookupEnv("DB_PORT")
	if !ok {
		log.Printf("DB_PORT is not set defaulting to 5432")
		port = "5432"
	}

	user, ok := os.LookupEnv("DB_USER")
	if !ok {
		return nil, ErrUsernameNotSet
	}

	pass, ok := os.LookupEnv("DB_PASS")
	if !ok {
		return nil, ErrPasswordNotSet
	}

	name, ok := os.LookupEnv("DB_NAME")
	if !ok {
		return nil, ErrNameNotSet
	}

	sslmode, ok := os.LookupEnv("DB_SSLMODE")
	if !ok {
		log.Printf("DB_SSLMODE is not set defaulting to require")
		sslmode = defaultSSLMode
	}

	connStr := fmt.Sprintf(POSTGRES_DSN,
		host,
		port,
		user,
		pass,
		name,
		sslmode,
	)

	opts := opts{}
	for _, option := range options {
		option(&opts)
	}

	if opts.config == nil {
		opts.config = defaultConfig
	}

	return gorm.Open(postgres.Open(connStr), opts.config)
}
