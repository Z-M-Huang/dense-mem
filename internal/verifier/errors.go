package verifier

import (
	"errors"
	"fmt"
)

// ErrVerifierTimeout is returned when a verifier request times out.
var ErrVerifierTimeout = errors.New("verifier request timed out")

// ErrVerifierProvider is returned when the verifier provider encounters an error.
var ErrVerifierProvider = errors.New("verifier provider error")

// ErrVerifierRateLimit is returned when the verifier provider rate limits the request.
var ErrVerifierRateLimit = errors.New("verifier request rate limited")

// ErrVerifierMalformedResponse is returned when the verifier provider returns a response
// that cannot be parsed or does not conform to the expected schema.
var ErrVerifierMalformedResponse = errors.New("verifier malformed response")

// TimeoutError wraps ErrVerifierTimeout with additional context.
type TimeoutError struct {
	Provider string
	Message  string
}

// Error implements the error interface.
func (e *TimeoutError) Error() string {
	return fmt.Sprintf("%s: %s: %s", ErrVerifierTimeout, e.Provider, e.Message)
}

// Is allows errors.Is to match ErrVerifierTimeout.
func (e *TimeoutError) Is(target error) bool {
	return target == ErrVerifierTimeout
}

// ProviderError wraps ErrVerifierProvider with additional context.
type ProviderError struct {
	Provider string
	Message  string
	Cause    error
}

// Error implements the error interface.
func (e *ProviderError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %s: %v", ErrVerifierProvider, e.Provider, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s: %s", ErrVerifierProvider, e.Provider, e.Message)
}

// Is allows errors.Is to match ErrVerifierProvider.
func (e *ProviderError) Is(target error) bool {
	return target == ErrVerifierProvider
}

// Unwrap returns the underlying cause.
func (e *ProviderError) Unwrap() error {
	return e.Cause
}

// RateLimitError wraps ErrVerifierRateLimit with additional context.
type RateLimitError struct {
	Provider   string
	Message    string
	RetryAfter int
}

// Error implements the error interface.
func (e *RateLimitError) Error() string {
	return fmt.Sprintf("%s: %s: %s", ErrVerifierRateLimit, e.Provider, e.Message)
}

// Is allows errors.Is to match ErrVerifierRateLimit.
func (e *RateLimitError) Is(target error) bool {
	return target == ErrVerifierRateLimit
}

// MalformedResponseError wraps ErrVerifierMalformedResponse with additional context.
type MalformedResponseError struct {
	Provider string
	Message  string
	RawJSON  string
}

// Error implements the error interface.
func (e *MalformedResponseError) Error() string {
	return fmt.Sprintf("%s: %s: %s", ErrVerifierMalformedResponse, e.Provider, e.Message)
}

// Is allows errors.Is to match ErrVerifierMalformedResponse.
func (e *MalformedResponseError) Is(target error) bool {
	return target == ErrVerifierMalformedResponse
}
