package database

import "gorm.io/gorm"

type opts struct {
	config *gorm.Config
}

type Option func(*opts)

var defaultConfig = &gorm.Config{Logger: myLogger{}}

func WithGormConfig(cfg *gorm.Config) Option {
	return func(o *opts) {
		if cfg.Logger == nil {
			cfg.Logger = myLogger{}
		}

		o.config = cfg
	}
}
