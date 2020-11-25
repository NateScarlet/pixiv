package client

import (
	"context"
)

type contextKey struct{}

// For get client from context.
func For(ctx context.Context) *Client {
	v, _ := ctx.Value(contextKey{}).(*Client)
	if v == nil {
		return Default
	}

	return v
}

// With set client to context.
func With(ctx context.Context, v *Client) context.Context {
	return context.WithValue(ctx, contextKey{}, v)
}
