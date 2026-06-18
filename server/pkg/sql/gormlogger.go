package sql

import (
	"log/slog"
	"time"

	"gorm.io/gorm/logger"
)

func NewGormSlogger(lg *slog.Logger) logger.Interface {
	if lg == nil {
		lg = slog.Default()
	}

	return logger.NewSlogLogger(
		lg,
		logger.Config{
			SlowThreshold:             500 * time.Millisecond,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			ParameterizedQueries:      true,
			Colorful:                  false,
		},
	)
}
