package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/markhuangai/dense-mem/internal/requestctx"
)

func TestClientIPMiddlewareStoresRealIPInContext(t *testing.T) {
	e := echo.New()
	e.Use(ClientIPMiddleware())
	e.GET("/test", func(c echo.Context) error {
		got := requestctx.ClientIPFromContext(c.Request().Context())
		if got != "203.0.113.10" {
			t.Errorf("ClientIPFromContext() = %q; want 203.0.113.10", got)
		}
		return c.NoContent(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(echo.HeaderXForwardedFor, "203.0.113.10")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusNoContent)
	}
}
