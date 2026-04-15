package middleware

import "context"

// SetPrincipalForTest injects a principal into ctx for tests that need to
// exercise downstream handlers/middleware without running the real auth flow.
//
// This helper exists so test code in sibling packages (e.g. handler tests)
// can construct authenticated request contexts without re-implementing the
// unexported context-key plumbing. Production code never calls this; the
// name and signature make intent clear, and the `principalContextKey` type
// remains unexported to preserve the "no forged principals from arbitrary
// downstream packages" invariant for non-test code paths.
func SetPrincipalForTest(ctx context.Context, principal *Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}
