package database

import "gorm.io/gorm"

func setupConfig(cfg *gorm.Config) *gorm.Config {
	if cfg == nil {
		cfg = &gorm.Config{}
	}

	if cfg.Logger == nil {
		cfg.Logger = myLogger{}
	}

	return cfg
}
