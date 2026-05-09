// Package requestctx carries request-scoped values through context.
package requestctx

import (
	"context"
	"strings"
)

type clientIPKey struct{}

// WithClientIP returns a new context carrying ip as the request client IP.
func WithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, clientIPKey{}, strings.TrimSpace(ip))
}

// ClientIPFromContext returns the request client IP previously set with WithClientIP.
// Returns an empty string when no client IP is present.
func ClientIPFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if ip, ok := ctx.Value(clientIPKey{}).(string); ok {
		return strings.TrimSpace(ip)
	}
	return ""
}
