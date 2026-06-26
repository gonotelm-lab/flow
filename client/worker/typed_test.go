// flow/client/worker/typed_test.go
package worker

import (
	"context"
	"errors"
	"testing"

	workerv1 "github.com/gonotelm-lab/flow/api/worker/v1"
	"github.com/stretchr/testify/require"
)

func TestRegisterTyped_Success(t *testing.T) {
	c := &Client{cfg: ConfigWithDefaults(Config{Codec: JSONCodec{}})}

	RegisterTyped[map[string]string, map[string]string](c, func(ctx context.Context, in map[string]string) (map[string]string, error) {
		return map[string]string{"v": in["k"]}, nil
	})

	require.NotNil(t, c.handler)
	action, payload := ResolveReport(c.handler(context.Background(), []byte(`{"k":"x"}`)))
	require.Equal(t, workerv1.ReportAction_SUCCESS, action)
	require.JSONEq(t, `{"v":"x"}`, string(payload))
}

func TestRegisterTyped_Error(t *testing.T) {
	c := &Client{cfg: ConfigWithDefaults(Config{Codec: JSONCodec{}})}

	RegisterTyped[map[string]string, map[string]string](c, func(ctx context.Context, in map[string]string) (map[string]string, error) {
		return nil, errors.New("bad")
	})

	_, err := c.handler(context.Background(), []byte(`{"k":"x"}`))
	require.Error(t, err)
	action, payload := ResolveReport(nil, err)
	require.Equal(t, workerv1.ReportAction_FAIL, action)
	require.Equal(t, []byte("bad"), payload)
}

func TestRegisterTypedResult(t *testing.T) {
	c := &Client{cfg: ConfigWithDefaults(Config{Codec: JSONCodec{}})}

	RegisterTypedResult[map[string]string](c, func(ctx context.Context, in map[string]string) (Result, error) {
		return ErrorResult{Data: []byte("nope")}, nil
	})

	action, payload := ResolveReport(c.handler(context.Background(), []byte(`{"k":"x"}`)))
	require.Equal(t, workerv1.ReportAction_FAIL, action)
	require.Equal(t, []byte("nope"), payload)
}
