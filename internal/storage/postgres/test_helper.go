package postgres

import "os"

// GetTestDSN returns the DSN to use for testing from environment variable.
// This is exported for use by other packages' integration tests.
func GetTestDSN() string {
	return os.Getenv("DATABASE_URL")
}
