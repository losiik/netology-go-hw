package domain

import "context"

type userKey struct{}

func WithUserID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, userKey{}, id)
}

func UserIDFromContext(ctx context.Context) int64 {
	id, _ := ctx.Value(userKey{}).(int64)
	return id
}
