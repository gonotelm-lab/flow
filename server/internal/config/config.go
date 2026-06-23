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

	ApiServer *ApiServer `toml:"apiServer"`
}

type DBConfig struct {
	Driver sql.Driver  `toml:"driver"`
	Config *sql.Config `toml:"config"`
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
	if cfg.Worker != nil {
		if err := cfg.Worker.Validate(); err != nil {
			return fmt.Errorf("worker validate failed: %w", err)
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
