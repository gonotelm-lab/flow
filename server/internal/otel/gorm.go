package otel

import (
	"fmt"

	"gorm.io/gorm"
	"gorm.io/plugin/opentelemetry/tracing"
)

// InstallGormTrace 为 gorm 实例注册 SQL span 插件（官方插件）。需在数据库初始化后调用。
func InstallGormTrace(db *gorm.DB) error {
	if err := db.Use(tracing.NewPlugin(tracing.WithoutMetrics())); err != nil {
		return fmt.Errorf("register otel tracing plugin failed: %w", err)
	}
	return nil
}
