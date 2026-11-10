package middleware

import "context"

type routeKey struct{}

func WithRoute(ctx context.Context, route string) context.Context {
	return context.WithValue(ctx, routeKey{}, route)
}

func RouteFromContext(ctx context.Context) string {
	route, _ := ctx.Value(routeKey{}).(string)
	return route
}
