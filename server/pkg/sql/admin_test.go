package sql

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidatePgIdentifier(t *testing.T) {
	require.NoError(t, ValidatePgIdentifier("flowdb"))
	require.NoError(t, ValidatePgIdentifier("flow_db_1"))
	require.Error(t, ValidatePgIdentifier(""))
	require.Error(t, ValidatePgIdentifier("1abc"))
	require.Error(t, ValidatePgIdentifier("bad-name"))
	require.Error(t, ValidatePgIdentifier(`bad";DROP`))
}

func TestQuotePgIdentifier(t *testing.T) {
	got, err := QuotePgIdentifier("flowdb")
	require.NoError(t, err)
	require.Equal(t, `"flowdb"`, got)

	_, err = QuotePgIdentifier("bad-name")
	require.Error(t, err)
}

func TestEnsurePgDatabase_NilConfig(t *testing.T) {
	require.Error(t, EnsurePgDatabase(nil))
}

func TestEnsurePgDatabase_InvalidName(t *testing.T) {
	require.Error(t, EnsurePgDatabase(&Config{DbName: "bad-name"}))
}
