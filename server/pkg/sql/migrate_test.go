package sql

import (
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

func TestGooseDialect(t *testing.T) {
	dialect, locker, err := gooseDialect(DriverPgsql)
	require.NoError(t, err)
	require.Equal(t, goose.DialectPostgres, dialect)
	require.NotNil(t, locker)

	_, _, err = gooseDialect(Driver("mysql"))
	require.Error(t, err)
}
