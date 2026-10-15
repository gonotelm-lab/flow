package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/a8m/envsubst"
	"github.com/gonotelm-lab/flow/server/pkg/sql"
)

var Conf *Config

type Config struct {
	DB *DBConfig `toml:"db"`

	Registry *RegistryConfig `toml:"registry"`

	Worker *WorkerConfig `toml:"worker"`

	// Scheduler 为 nil 时不启动定时回调调度器（与 Worker 同样的可选语义）。
	Scheduler *SchedulerConfig `toml:"scheduler"`

	ApiServer *ApiServer `toml:"apiServer"`

	Otel *OtelConfig `toml:"otel"`
}

type DBConfig struct {
	Driver   sql.Driver  `toml:"driver"`
	AutoInit bool        `toml:"autoInit"`
	Config   *sql.Config `toml:"config"`
}

type RegistryConfig struct {
	Expiry            time.Duration `toml:"expiry"`
	KeepaliveInterval time.Duration `toml:"keepaliveInterval"`
	SweepInterval     time.Duration `toml:"sweepInterval"`
	SweepBatch        int           `toml:"sweepBatch"`

	WatchInterval        time.Duration `toml:"watchInterval"`
	WatchBatchSize       int           `toml:"watchBatchSize"`
	WatchMaxRetryBackoff time.Duration `toml:"watchMaxRetryBackoff"`
}

type WorkerConfig struct {
	PollWait          time.Duration `toml:"pollWait"`
	PollCheckInterval time.Duration `toml:"pollCheckInterval"`
	StaleTaskTimeout  time.Duration `toml:"staleTaskTimeout"`
	StaleScanInterval time.Duration `toml:"staleScanInterval"`
	RetryScanInterval time.Duration `toml:"retryScanInterval"`
	RetryScanBatch    int           `toml:"retryScanBatch"`
	StaleScanBatch    int           `toml:"staleScanBatch"`
}

func Init(path string) error {
	cfg, err := Load(path)
	if err != nil {
		return err
	}

	Conf = cfg
	return nil
}

func MustInit(path string) {
	if err := Init(path); err != nil {
		panic(err)
	}

	slog.Info("config initialized", "path", path)
}

func Load(path string) (*Config, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("config path is empty")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file %s failed: %w", path, err)
	}

	expanded, err := envsubst.String(string(content))
	if err != nil {
		return nil, fmt.Errorf("expand env in config file %s failed: %w", path, err)
	}

	cfg := &Config{}
	if _, err := toml.Decode(expanded, cfg); err != nil {
		return nil, fmt.Errorf("decode toml %s failed: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (cfg *Config) Validate() error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	if cfg.DB == nil {
		return fmt.Errorf("db config is nil")
	}
	if cfg.Registry == nil {
		return fmt.Errorf("registry config is nil")
	}

	if err := cfg.DB.Validate(); err != nil {
		return fmt.Errorf("db validate failed: %w", err)
	}
	if err := cfg.Registry.Validate(); err != nil {
		return fmt.Errorf("registry validate failed: %w", err)
	}
	if err := cfg.Otel.Validate(); err != nil {
		return fmt.Errorf("otel validate failed: %w", err)
	}
	if cfg.Worker != nil {
		if err := cfg.Worker.Validate(); err != nil {
			return fmt.Errorf("worker validate failed: %w", err)
		}
	}
	if cfg.Scheduler != nil {
		if err := cfg.Scheduler.Validate(); err != nil {
			return fmt.Errorf("scheduler validate failed: %w", err)
		}
	}

	return nil
}

func (cfg *DBConfig) Validate() error {
	if cfg == nil {
		return fmt.Errorf("db config is nil")
	}
	if strings.TrimSpace(string(cfg.Driver)) == "" {
		return fmt.Errorf("db.driver is empty")
	}
	if cfg.Config == nil {
		return fmt.Errorf("db.config is nil")
	}
	if strings.TrimSpace(cfg.Config.Host) == "" {
		return fmt.Errorf("db.config.host is empty")
	}
	if cfg.Config.Port <= 0 {
		return fmt.Errorf("db.config.port must be positive")
	}
	if strings.TrimSpace(cfg.Config.User) == "" {
		return fmt.Errorf("db.config.user is empty")
	}
	if strings.TrimSpace(cfg.Config.Password) == "" {
		return fmt.Errorf("db.config.password is empty")
	}
	if strings.TrimSpace(cfg.Config.DbName) == "" {
		return fmt.Errorf("db.config.dbName is empty")
	}

	return nil
}

func (cfg *RegistryConfig) Validate() error {
	if cfg == nil {
		return fmt.Errorf("registry config is nil")
	}
	if cfg.Expiry <= 0 {
		return fmt.Errorf("registry.expiry must be positive")
	}
	if cfg.KeepaliveInterval <= 0 {
		return fmt.Errorf("registry.keepaliveInterval must be positive")
	}
	if cfg.SweepInterval <= 0 {
		return fmt.Errorf("registry.sweepInterval must be positive")
	}
	if cfg.SweepBatch <= 0 {
		return fmt.Errorf("registry.sweepBatch must be positive")
	}
	if cfg.WatchInterval <= 0 {
		return fmt.Errorf("registry.watchInterval must be positive")
	}
	if cfg.WatchBatchSize <= 0 {
		return fmt.Errorf("registry.watchBatchSize must be positive")
	}
	if cfg.WatchMaxRetryBackoff <= 0 {
		return fmt.Errorf("registry.watchMaxRetryBackoff must be positive")
	}

	return nil
}

func (cfg *WorkerConfig) Validate() error {
	if cfg == nil {
		return fmt.Errorf("worker config is nil")
	}
	if cfg.PollWait <= 0 {
		return fmt.Errorf("worker.pollWait must be positive")
	}
	if cfg.PollCheckInterval < 0 {
		return fmt.Errorf("worker.pollCheckInterval must be non-negative")
	}
	if cfg.StaleTaskTimeout <= 0 {
		return fmt.Errorf("worker.staleTaskTimeout must be positive")
	}
	if cfg.StaleScanInterval <= 0 {
		return fmt.Errorf("worker.staleScanInterval must be positive")
	}
	if cfg.RetryScanInterval <= 0 {
		return fmt.Errorf("worker.retryScanInterval must be positive")
	}
	if cfg.RetryScanBatch <= 0 {
		return fmt.Errorf("worker.retryScanBatch must be positive")
	}
	if cfg.StaleScanBatch <= 0 {
		return fmt.Errorf("worker.staleScanBatch must be positive")
	}

	return nil
}

type ApiServer struct {
	Http *HttpServer `toml:"http"`
	Grpc *GrpcServer `toml:"grpc"`
}

type HttpServer struct {
	Listen string `toml:"listen"`
	Port   int    `toml:"port"`
}

type GrpcServer struct {
	Listen string `toml:"listen"`
	Port   int    `toml:"port"`
}

type OtelConfig struct {
	Enabled      bool     `toml:"enabled"`
	ServiceName  string   `toml:"serviceName"`
	Endpoint     string   `toml:"endpoint"`
	Protocol     string   `toml:"protocol"`
	SamplerRatio *float64 `toml:"samplerRatio"` // OTel traceidratio 语义，0=不采样；nil=未配置（环境变量/SDK 默认）
}

func (cfg *OtelConfig) Validate() error {
	if cfg == nil || !cfg.Enabled {
		return nil
	}
	switch cfg.Protocol {
	case "", "grpc", "http":
	default:
		return fmt.Errorf("otel.protocol must be one of grpc, http, got %q", cfg.Protocol)
	}
	if cfg.SamplerRatio != nil && (*cfg.SamplerRatio < 0 || *cfg.SamplerRatio > 1) {
		return fmt.Errorf("otel.samplerRatio must be in [0, 1], got %v", *cfg.SamplerRatio)
	}
	return nil
}

// SchedulerConfig 是定时回调调度器的配置。
//
// 重试策略（间隔序列、次数、抖动）全部是服务端全局配置，客户端不可指定、不可覆盖。
type SchedulerConfig struct {
	// TickInterval 到期判定与重试扫描的粒度
	TickInterval time.Duration `toml:"tickInterval"`
	// ScanBatch 单次扫描认领的最大条数
	ScanBatch int `toml:"scanBatch"`
	// DeliverConcurrency 单实例并发投递上限
	DeliverConcurrency int `toml:"deliverConcurrency"`
	// HTTPTimeout 单次回调的 HTTP 超时
	HTTPTimeout time.Duration `toml:"httpTimeout"`
	// LeaseDuration 认领后的租约时长
	LeaseDuration time.Duration `toml:"leaseDuration"`
	// LatenessTolerance 迟到容差：now - next_fire_at 超过它即视为「停机期间错过」，
	// 不触发而是跳过并重算 next_fire_at
	LatenessTolerance time.Duration `toml:"latenessTolerance"`
	// RetryJitterRatio 重试间隔的抖动比例，避免同点 schedule 同时打对方
	RetryJitterRatio float64 `toml:"retryJitterRatio"`

	// FastRetryIntervalsRaw / DeadLetterIntervalsRaw 是逗号分隔的时长序列，
	// 由 envsubst 注入模板得到。
	FastRetryIntervalsRaw  string `toml:"fastRetryIntervals"`
	DeadLetterIntervalsRaw string `toml:"deadLetterIntervals"`

	// FastRetryIntervals / DeadLetterIntervals 是解析后的结果，由 Validate 填充。
	FastRetryIntervals  []time.Duration `toml:"-"`
	DeadLetterIntervals []time.Duration `toml:"-"`
}

// DefaultSchedulerConfig 返回一份可直接使用的默认配置，便于测试与程序化构造。
func DefaultSchedulerConfig() *SchedulerConfig {
	cfg := &SchedulerConfig{
		TickInterval:           time.Second,
		ScanBatch:              100,
		DeliverConcurrency:     64,
		HTTPTimeout:            10 * time.Second,
		LeaseDuration:          30 * time.Second,
		LatenessTolerance:      30 * time.Second,
		RetryJitterRatio:       0.2,
		FastRetryIntervalsRaw:  "1s,2s,4s,8s,16s",
		DeadLetterIntervalsRaw: "1m,5m,30m",
	}
	_ = cfg.Validate()
	return cfg
}

// Validate 校验配置，并把 FastRetryIntervalsRaw / DeadLetterIntervalsRaw 解析成
// 时长序列填进对应字段（因此 Validate 会改写接收者）。
func (cfg *SchedulerConfig) Validate() error {
	if cfg == nil {
		return fmt.Errorf("scheduler config is nil")
	}
	if cfg.TickInterval <= 0 {
		return fmt.Errorf("scheduler.tickInterval must be positive")
	}
	if cfg.ScanBatch <= 0 {
		return fmt.Errorf("scheduler.scanBatch must be positive")
	}
	if cfg.DeliverConcurrency <= 0 {
		return fmt.Errorf("scheduler.deliverConcurrency must be positive")
	}
	if cfg.HTTPTimeout <= 0 {
		return fmt.Errorf("scheduler.httpTimeout must be positive")
	}
	if cfg.LeaseDuration <= 0 {
		return fmt.Errorf("scheduler.leaseDuration must be positive")
	}
	// 最关键的一条：租约若不长于 HTTP 超时，正常投递会先被自己的租约过期判成
	// 「可接管」，于是另一个实例重投——这是本可避免的重复投递。
	if cfg.LeaseDuration <= cfg.HTTPTimeout {
		return fmt.Errorf(
			"scheduler.leaseDuration (%s) must be greater than scheduler.httpTimeout (%s)",
			cfg.LeaseDuration, cfg.HTTPTimeout)
	}
	if cfg.LatenessTolerance <= 0 {
		return fmt.Errorf("scheduler.latenessTolerance must be positive")
	}
	if cfg.RetryJitterRatio < 0 || cfg.RetryJitterRatio >= 1 {
		return fmt.Errorf("scheduler.retryJitterRatio must be in [0, 1), got %v", cfg.RetryJitterRatio)
	}

	fast, err := parseDurationList(cfg.FastRetryIntervalsRaw)
	if err != nil {
		return fmt.Errorf("scheduler.fastRetryIntervals invalid: %w", err)
	}
	if len(fast) == 0 {
		return fmt.Errorf("scheduler.fastRetryIntervals must not be empty")
	}

	dead, err := parseDurationList(cfg.DeadLetterIntervalsRaw)
	if err != nil {
		return fmt.Errorf("scheduler.deadLetterIntervals invalid: %w", err)
	}
	if len(dead) == 0 {
		return fmt.Errorf("scheduler.deadLetterIntervals must not be empty")
	}

	cfg.FastRetryIntervals = fast
	cfg.DeadLetterIntervals = dead

	return nil
}

// parseDurationList 解析逗号分隔的时长序列，忽略空白项。
func parseDurationList(raw string) ([]time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	parts := strings.Split(raw, ",")
	out := make([]time.Duration, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		d, err := time.ParseDuration(part)
		if err != nil {
			return nil, fmt.Errorf("%q is not a valid duration: %w", part, err)
		}
		if d <= 0 {
			return nil, fmt.Errorf("%q must be positive", part)
		}
		out = append(out, d)
	}

	return out, nil
}
