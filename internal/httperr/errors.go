package httperr

import (
	"fmt"
)

// ErrorCode represents a typed error code for API errors.
type ErrorCode string

// Error code constants
const (
	AUTH_MISSING                    ErrorCode = "AUTH_MISSING"
	AUTH_INVALID                    ErrorCode = "AUTH_INVALID"
	AUTH_EXPIRED                    ErrorCode = "AUTH_EXPIRED"
	AUTH_REVOKED                    ErrorCode = "AUTH_REVOKED"
	FORBIDDEN                       ErrorCode = "FORBIDDEN"
	NOT_FOUND                       ErrorCode = "NOT_FOUND"
	VALIDATION_ERROR                ErrorCode = "VALIDATION_ERROR"
	PROFILE_ID_REQUIRED             ErrorCode = "PROFILE_ID_REQUIRED"
	INVALID_UUID                    ErrorCode = "INVALID_UUID"
	CONFLICT                        ErrorCode = "CONFLICT"
	PROFILE_HAS_ACTIVE_KEYS         ErrorCode = "PROFILE_HAS_ACTIVE_KEYS"
	RATE_LIMITED                    ErrorCode = "RATE_LIMITED"
	SERVICE_UNAVAILABLE             ErrorCode = "SERVICE_UNAVAILABLE"
	INTERNAL_ERROR                  ErrorCode = "INTERNAL_ERROR"
	EMBEDDING_GENERATION_NOT_CONFIGURED ErrorCode = "EMBEDDING_GENERATION_NOT_CONFIGURED"
)

// ErrorDetail represents a single validation error detail.
type ErrorDetail struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// APIError represents a structured API error with code, message, and optional details.
type APIError struct {
	Code    ErrorCode     `json:"code"`
	Message string        `json:"message"`
	Details []ErrorDetail `json:"details"`
}

// Error implements the error interface.
func (e *APIError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// APIErrorProvider is the companion interface for APIError.
// Consumers and tests depend on this abstraction rather than the concrete struct.
type APIErrorProvider interface {
	Error() string
	GetCode() ErrorCode
	GetMessage() string
	GetDetails() []ErrorDetail
}

// Ensure APIError implements APIErrorProvider
var _ APIErrorProvider = (*APIError)(nil)

// GetCode returns the error code.
func (e *APIError) GetCode() ErrorCode {
	return e.Code
}

// GetMessage returns the error message.
func (e *APIError) GetMessage() string {
	return e.Message
}

// GetDetails returns the error details.
func (e *APIError) GetDetails() []ErrorDetail {
	return e.Details
}

// New creates a new APIError with the given code and message.
func New(code ErrorCode, message string) *APIError {
	return &APIError{
		Code:    code,
		Message: message,
		Details: nil,
	}
}

// NewWithDetails creates a new APIError with validation details.
func NewWithDetails(code ErrorCode, message string, details []ErrorDetail) *APIError {
	return &APIError{
		Code:    code,
		Message: message,
		Details:   details,
	}
}

// ErrorEnvelope is the JSON envelope for error responses.
type ErrorEnvelope struct {
	Error *APIError `json:"error"`
}

// NewErrorEnvelope creates a new error envelope wrapping an APIError.
func NewErrorEnvelope(err *APIError) ErrorEnvelope {
	return ErrorEnvelope{Error: err}
}
