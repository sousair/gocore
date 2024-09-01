package database

import (
	"fmt"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const POSTGRES_DSN = "postgresql://%s:%s@%s:%s/%s"

func NewPostgres() (*gorm.DB, error) {
	username, ok := os.LookupEnv("DB_USERNAME")
	if !ok {
		return nil, ErrDBUsernameNotSet
	}

	password, ok := os.LookupEnv("DB_PASSWORD")
	if !ok {
		return nil, ErrDBPasswordNotSet
	}

	host, ok := os.LookupEnv("DB_HOST")
	if !ok {
		return nil, ErrDBHostNotSet
	}

	port, ok := os.LookupEnv("DB_PORT")
	if !ok {
		port = "5432"
	}

	name, ok := os.LookupEnv("DB_NAME")
	if !ok {
		return nil, ErrDBNameNotSet
	}

	connectionString := fmt.Sprintf(POSTGRES_DSN, username, password, host, port, name)

	return gorm.Open(postgres.Open(connectionString), &gorm.Config{
		Logger: myLogger{},
	})
}
