// flow/client/worker/typed.go
package worker

import "context"

func RegisterTyped[TIn, TOut any](c *Client, fn func(ctx context.Context, in TIn) (TOut, error)) {
	c.Handle(func(ctx context.Context, payload []byte) (Result, error) {
		var in TIn
		if err := c.cfg.Codec.Unmarshal(payload, &in); err != nil {
			return nil, err
		}
		out, err := fn(ctx, in)
		if err != nil {
			return nil, err
		}
		data, err := c.cfg.Codec.Marshal(out)
		if err != nil {
			return nil, err
		}
		return OkResult{Data: data}, nil
	})
}

func RegisterTypedResult[TIn any](c *Client, fn func(ctx context.Context, in TIn) (Result, error)) {
	c.Handle(func(ctx context.Context, payload []byte) (Result, error) {
		var in TIn
		if err := c.cfg.Codec.Unmarshal(payload, &in); err != nil {
			return nil, err
		}
		return fn(ctx, in)
	})
}
