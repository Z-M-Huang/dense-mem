package http

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// registerPublicRoutes registers public routes that do not require authentication,
// profile resolution, or rate limiting. These routes are used for health checks
// and readiness probes in container orchestration environments.
func registerPublicRoutes(e *echo.Echo, checks []HealthCheck) {
	// Health endpoint - simple liveness check
	// Returns 200 {"status":"ok"} - no middleware applied
	e.GET("/health", handleHealth)

	// Ready endpoint - readiness check with dependency validation
	// Returns 200 {"status":"ready","dependencies":{...}} if all checks pass
	// Returns 503 {"status":"degraded","dependencies":{...}} if any check fails
	// No auth/profile/rate-limit middleware applied
	e.GET("/ready", handleReady(checks))
}

// handleHealth handles the /health endpoint.
// It returns a simple 200 OK response indicating the service is alive.
func handleHealth(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{
		"status": "ok",
	})
}

// handleReady returns a handler function for the /ready endpoint.
// It executes all health checks and returns the appropriate status.
func handleReady(checks []HealthCheck) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()
		dependencies := make(map[string]string)
		allPass := true

		// Execute all health checks
		for i, check := range checks {
			checkName := getCheckName(i)
			if err := check(ctx); err != nil {
				dependencies[checkName] = "failed"
				allPass = false
			} else {
				dependencies[checkName] = "ok"
			}
		}

		if allPass {
			return c.JSON(http.StatusOK, map[string]interface{}{
				"status":       "ready",
				"dependencies": dependencies,
			})
		}

		return c.JSON(http.StatusServiceUnavailable, map[string]interface{}{
			"status":       "degraded",
			"dependencies": dependencies,
		})
	}
}

// getCheckName returns a name for the health check at the given index.
// This is a placeholder that will be enhanced when named checks are registered.
func getCheckName(index int) string {
	// For now, use generic names. Later units will register named checks.
	return "check-" + string(rune('a'+index))
}
