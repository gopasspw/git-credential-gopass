package githttp

import "context"

type contextKey int

const ctxKeyUsername contextKey = iota

func WithUsername(ctx context.Context, username string) context.Context {
	return context.WithValue(ctx, ctxKeyUsername, username)
}

func Username(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyUsername).(string); ok {
		return v
	}
	return ""
}
