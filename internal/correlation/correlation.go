// Package correlation carries request correlation IDs through the context.
//
// This package is the neutral carrier between the HTTP middleware that seeds
// the ID and downstream services (audit, logs, metrics) that read it. Placing
// the key here keeps the service layer from importing http/middleware.
package correlation

import "context"

type contextKey struct{}

// WithID returns a new context carrying id as the correlation identifier.
func WithID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKey{}, id)
}

// FromContext returns the correlation ID previously set with WithID.
// Returns an empty string when no id is present.
func FromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(contextKey{}).(string); ok {
		return id
	}
	return ""
}
