package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/a8m/envsubst"
	"github.com/gonotelm-lab/flow/server/pkg/sql"
)

var (
	Conf *Config
)

type Config struct {
	DB *DBConfig `toml:"db"`

	Registry *RegistryConfig `toml:"registry"`
}

type DBConfig struct {
	Driver string      `toml:"driver"`
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

	return nil
}

func (cfg *DBConfig) Validate() error {
	if cfg == nil {
		return fmt.Errorf("db config is nil")
	}
	if strings.TrimSpace(cfg.Driver) == "" {
		return fmt.Errorf("db driver is empty")
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
