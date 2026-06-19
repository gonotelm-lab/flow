package sql

import (
	"fmt"

	"gorm.io/gorm"
)

func Open(driver Driver, config *Config) (*gorm.DB, error) {
	switch driver {
	case DriverPgsql:
		return OpenPgSql(config)
	}

	return nil, fmt.Errorf("driver %s is not supported", driver)
}
