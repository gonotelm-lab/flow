package testsuite

import (
	"context"
	"crypto/rand"
	stderrors "errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gonotelm-lab/flow/server/pkg/sql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

const (
	EnvDBHost = "FLOW_DB_HOST"
	EnvDBPort = "FLOW_DB_PORT"
	EnvDBUser = "FLOW_DB_USER"
	EnvDBPass = "FLOW_DB_PASS"
	EnvDBName = "FLOW_DB_NAME"
)

type TestDb struct {
	db         *gorm.DB
	driver     string
	config     sql.Config
	logger     gormlogger.Interface
	testDBName string
}

func NewTestGormDB(driver string, config *sql.Config) (*TestDb, error) {
	if config == nil {
		return nil, fmt.Errorf("db config is nil")
	}

	normalizedDriver, err := normalizeDriver(driver)
	if err != nil {
		return nil, err
	}

	cfg := *config
	if err := validateConfig(normalizedDriver, &cfg); err != nil {
		return nil, err
	}

	return &TestDb{
		driver: normalizedDriver,
		config: cfg,
		logger: newTestLogger(),
	}, nil
}

func NewTestGormDBFromEnv(driver string) (*TestDb, error) {
	normalizedDriver, err := normalizeDriver(driver)
	if err != nil {
		return nil, err
	}

	if normalizedDriver != "pgsql" {
		return nil, fmt.Errorf("driver %s env loader is not implemented yet", normalizedDriver)
	}

	envVals, err := readRequiredEnv(EnvDBHost, EnvDBPort, EnvDBUser, EnvDBPass, EnvDBName)
	if err != nil {
		return nil, err
	}

	port, err := strconv.Atoi(envVals[EnvDBPort])
	if err != nil {
		return nil, fmt.Errorf("invalid env %s=%q: %w", EnvDBPort, envVals[EnvDBPort], err)
	}

	return NewTestGormDB("pgsql", &sql.Config{
		Host:     envVals[EnvDBHost],
		Port:     port,
		User:     envVals[EnvDBUser],
		Password: envVals[EnvDBPass],
		DbName:   envVals[EnvDBName],
	})
}

func (t *TestDb) GetDB() *gorm.DB {
	if t == nil {
		return nil
	}
	return t.db
}

func (t *TestDb) Setup(fsys fs.FS) error {
	if t == nil {
		return fmt.Errorf("test db is nil")
	}
	if t.driver != "pgsql" {
		return fmt.Errorf("driver %s setup is not implemented yet", t.driver)
	}

	return t.setupPgsql(fsys)
}

func (t *TestDb) Cleanup() error {
	if t == nil {
		return nil
	}
	if t.driver != "pgsql" {
		return fmt.Errorf("driver %s cleanup is not implemented yet", t.driver)
	}

	return t.cleanupPgsql()
}

func (t *TestDb) setupPgsql(fsys fs.FS) error {
	if fsys == nil {
		return fmt.Errorf("migration fs is nil")
	}
	if t.db != nil {
		return fmt.Errorf("test db already setup")
	}

	testDBName, err := newRandomTestDBName()
	if err != nil {
		return err
	}
	if err := sql.CreatePgDatabase(&t.config, testDBName, t.logger); err != nil {
		return err
	}

	testConfig := t.config
	testConfig.DbName = testDBName
	testDB, err := sql.OpenPgSqlWithLogger(&testConfig, t.logger)
	if err != nil {
		_ = sql.DropPgDatabase(&t.config, testDBName, t.logger)
		return fmt.Errorf("open test db failed: %w", err)
	}
	cleanupOnFailure := func() {
		_ = closeGormDB(testDB)
		_ = sql.DropPgDatabase(&t.config, testDBName, t.logger)
	}

	sqlDB, err := testDB.DB()
	if err != nil {
		cleanupOnFailure()
		return fmt.Errorf("get sql db failed: %w", err)
	}
	if err := sql.MigrateUp(context.Background(), sql.Driver(t.driver), sqlDB, fsys); err != nil {
		cleanupOnFailure()
		return err
	}

	t.testDBName = testDBName
	t.db = testDB
	return nil
}

func (t *TestDb) cleanupPgsql() error {
	var errs []error
	errs = append(errs, closeGormDB(t.db))
	t.db = nil

	if t.testDBName != "" {
		errs = append(errs, sql.DropPgDatabase(&t.config, t.testDBName, t.logger))
	}
	t.testDBName = ""

	return joinErrors(errs...)
}

func normalizeDriver(driver string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "pgsql", "postgres", "postgresql":
		return "pgsql", nil
	case "mysql":
		return "mysql", nil
	case "sqlite", "sqlite3":
		return "sqlite", nil
	default:
		return "", fmt.Errorf("unsupported driver: %s", driver)
	}
}

func readRequiredEnv(keys ...string) (map[string]string, error) {
	missing := make([]string, 0, len(keys))
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		value := strings.TrimSpace(os.Getenv(key))
		if value == "" {
			missing = append(missing, key)
			continue
		}
		values[key] = value
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required env vars: %s", strings.Join(missing, ", "))
	}

	return values, nil
}

func validateConfig(driver string, config *sql.Config) error {
	if config == nil {
		return fmt.Errorf("db config is nil")
	}
	switch driver {
	case "pgsql", "mysql":
		if strings.TrimSpace(config.Host) == "" {
			return fmt.Errorf("db host is empty")
		}
		if config.Port <= 0 {
			return fmt.Errorf("db port must be positive")
		}
		if strings.TrimSpace(config.User) == "" {
			return fmt.Errorf("db user is empty")
		}
		if strings.TrimSpace(config.Password) == "" {
			return fmt.Errorf("db password is empty")
		}
		if strings.TrimSpace(config.DbName) == "" {
			return fmt.Errorf("db name is empty")
		}
		return nil
	case "sqlite":
		if strings.TrimSpace(config.DbName) == "" {
			return fmt.Errorf("sqlite db name/path is empty")
		}
		return nil
	default:
		return fmt.Errorf("driver %s validation is not implemented yet", driver)
	}
}

func closeGormDB(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get sql db failed: %w", err)
	}
	return sqlDB.Close()
}

func newRandomTestDBName() (string, error) {
	randBytes := make([]byte, 4)
	if _, err := rand.Read(randBytes); err != nil {
		return "", fmt.Errorf("read random bytes failed: %w", err)
	}

	name := fmt.Sprintf("gonotelm_test_%d_%x", time.Now().UnixNano(), randBytes)
	if len(name) > 63 {
		name = name[:63]
	}
	if err := sql.ValidatePgIdentifier(name); err != nil {
		return "", fmt.Errorf("generated invalid db name: %s", name)
	}

	return name, nil
}

func joinErrors(errs ...error) error {
	filtered := make([]error, 0, len(errs))
	for _, err := range errs {
		if err != nil {
			filtered = append(filtered, err)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return stderrors.Join(filtered...)
}

func newTestLogger() gormlogger.Interface {
	return gormlogger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		gormlogger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  gormlogger.Info,
			IgnoreRecordNotFoundError: true,
			ParameterizedQueries:      false,
			Colorful:                  false,
		},
	)
}
