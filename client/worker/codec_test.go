package worker

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type sample struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func TestJSONCodec_RoundTrip(t *testing.T) {
	codec := JSONCodec{}
	in := sample{Name: "alice", Age: 3}
	data, err := codec.Marshal(in)
	require.NoError(t, err)
	var out sample
	require.NoError(t, codec.Unmarshal(data, &out))
	require.Equal(t, in, out)
}
