package database

import "gorm.io/gorm"

type opts struct {
	config *gorm.Config
}

type Option func(*opts)

func WithGormConfig(cfg *gorm.Config) Option {
	return func(o *opts) {
		o.config = cfg
	}
}
