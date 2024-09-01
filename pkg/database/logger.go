package database

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm/logger"
)

type myLogger struct{}

var _ logger.Interface = (*myLogger)(nil)

func (l myLogger) Info(ctx context.Context, msg string, data ...interface{})  {}
func (l myLogger) Warn(ctx context.Context, msg string, data ...interface{})  {}
func (l myLogger) Error(ctx context.Context, msg string, data ...interface{}) {}
func (l myLogger) LogMode(level logger.LogLevel) logger.Interface             { return l }
func (l myLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	if err == nil {
		fmt.Println(sql + "\n\n")
	}
}
