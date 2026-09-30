package migration

import (
	"io/fs"
	"testing"

	"github.com/gonotelm-lab/flow/server/pkg/sql"
	"github.com/stretchr/testify/require"
)

func TestFSFor(t *testing.T) {
	fsys, ok := FSFor(sql.DriverPgsql)
	require.True(t, ok)
	require.NotNil(t, fsys)

	content, err := fs.ReadFile(fsys, "00001_init.sql")
	require.NoError(t, err)
	require.Contains(t, string(content), "-- +goose Up")

	_, ok = FSFor(sql.Driver("mysql"))
	require.False(t, ok)
}
