package http

import (
	"context"
	"crypto/subtle"
	"fmt"
	nethttp "net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"

	"github.com/dense-mem/dense-mem/internal/config"
	"github.com/dense-mem/dense-mem/internal/domain"
	dto "github.com/dense-mem/dense-mem/internal/http/dto"
	"github.com/dense-mem/dense-mem/internal/http/handler"
	httpvalidation "github.com/dense-mem/dense-mem/internal/http/validation"
	"github.com/dense-mem/dense-mem/internal/httperr"
	"github.com/dense-mem/dense-mem/internal/observability"
	"github.com/dense-mem/dense-mem/internal/service"
)

// NewControlPortalServer creates the token-protected management portal server.
func NewControlPortalServer(
	cfg config.ConfigProvider,
	profileSvc handler.ProfileServiceInterface,
	apiKeySvc handler.APIKeyServiceInterface,
	logger observability.LogProvider,
) (*echo.Echo, error) {
	if cfg == nil {
		return nil, fmt.Errorf("control portal: config is required")
	}
	if strings.TrimSpace(cfg.GetControlPortalToken()) == "" {
		return nil, fmt.Errorf("control portal: token is required")
	}

	e := echo.New()
	e.HTTPErrorHandler = httperr.ErrorHandler
	e.Use(echomw.Recover())
	e.Use(echomw.RequestLoggerWithConfig(echomw.RequestLoggerConfig{
		HandleError: true,
		LogMethod:   true,
		LogURI:      true,
		LogStatus:   true,
		LogValuesFunc: func(_ echo.Context, v echomw.RequestLoggerValues) error {
			if logger == nil {
				return nil
			}
			attrs := []observability.LogAttr{
				observability.String("method", v.Method),
				observability.String("uri", v.URI),
				observability.Int("status", v.Status),
			}
			if v.Error != nil {
				logger.Error("control_http_request", v.Error, attrs...)
				return nil
			}
			logger.Info("control_http_request", attrs...)
			return nil
		},
	}))

	control := &controlPortalHandler{profiles: profileSvc, keys: apiKeySvc}
	api := e.Group("/control/api")
	api.Use(controlPortalMiddleware(cfg.GetControlPortalToken()))
	api.GET("/session", control.session)
	api.GET("/profiles", control.listProfiles)
	api.POST("/profiles", control.createProfile)
	api.PATCH("/profiles/:profileId", control.updateProfile)
	api.DELETE("/profiles/:profileId", control.deleteProfile)
	api.GET("/profiles/:profileId/api-keys", control.listAPIKeys)
	api.POST("/profiles/:profileId/api-keys", control.createAPIKey)
	api.DELETE("/profiles/:profileId/api-keys/:keyId", control.deleteAPIKey)

	if staticDir := defaultPortalStaticDir(); staticDir != "" {
		e.Static("/", staticDir)
	}

	return e, nil
}

type controlPortalHandler struct {
	profiles handler.ProfileServiceInterface
	keys     handler.APIKeyServiceInterface
}

func (h *controlPortalHandler) session(c echo.Context) error {
	return c.JSON(nethttp.StatusOK, map[string]any{"data": map[string]bool{"authenticated": true}})
}

func (h *controlPortalHandler) listProfiles(c echo.Context) error {
	limit, offset := controlPagination(c)
	profiles, err := h.profiles.List(c.Request().Context(), limit, offset)
	if err != nil {
		return err
	}
	total, err := h.profiles.Count(c.Request().Context())
	if err != nil {
		return err
	}
	items := make([]controlProfileResponse, 0, len(profiles))
	for _, profile := range profiles {
		items = append(items, toControlProfile(profile))
	}
	return c.JSON(nethttp.StatusOK, handler.PaginationEnvelope{
		Data:       items,
		Pagination: handler.Pagination{Limit: limit, Offset: offset, Total: total},
	})
}

func (h *controlPortalHandler) createProfile(c echo.Context) error {
	var body dto.CreateProfileRequest
	if err := c.Bind(&body); err != nil {
		return httperr.New(httperr.VALIDATION_ERROR, "malformed JSON body")
	}
	if err := httpvalidation.ValidateStruct(&body); err != nil {
		return httperr.New(httperr.VALIDATION_ERROR, err.Error())
	}
	profile, err := h.profiles.Create(c.Request().Context(), service.CreateProfileRequest{
		Name:        body.Name,
		Description: body.Description,
		Metadata:    body.Metadata,
		Config:      body.Config,
	}, nil, "control", c.RealIP(), "")
	if err != nil {
		return err
	}
	return c.JSON(nethttp.StatusCreated, map[string]any{"data": toControlProfile(profile)})
}

func (h *controlPortalHandler) updateProfile(c echo.Context) error {
	profileID, err := parseControlUUID(c.Param("profileId"), "profile ID")
	if err != nil {
		return err
	}
	var body dto.UpdateProfileRequest
	if err := c.Bind(&body); err != nil {
		return httperr.New(httperr.VALIDATION_ERROR, "malformed JSON body")
	}
	if err := httpvalidation.ValidateStruct(&body); err != nil {
		return httperr.New(httperr.VALIDATION_ERROR, err.Error())
	}
	var namePtr, descPtr *string
	if body.Name != "" {
		namePtr = &body.Name
	}
	if body.Description != "" {
		descPtr = &body.Description
	}
	profile, err := h.profiles.Update(c.Request().Context(), profileID, service.UpdateProfileRequest{
		Name:        namePtr,
		Description: descPtr,
		Metadata:    body.Metadata,
		Config:      body.Config,
	}, nil, "control", c.RealIP(), "")
	if err != nil {
		return err
	}
	return c.JSON(nethttp.StatusOK, map[string]any{"data": toControlProfile(profile)})
}

func (h *controlPortalHandler) deleteProfile(c echo.Context) error {
	profileID, err := parseControlUUID(c.Param("profileId"), "profile ID")
	if err != nil {
		return err
	}
	if err := h.profiles.Delete(c.Request().Context(), profileID, nil, "control", c.RealIP(), ""); err != nil {
		return err
	}
	return c.JSON(nethttp.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

func (h *controlPortalHandler) listAPIKeys(c echo.Context) error {
	profileID, err := parseControlUUID(c.Param("profileId"), "profile ID")
	if err != nil {
		return err
	}
	limit, offset := controlPagination(c)
	keys, err := h.keys.ListByProfile(c.Request().Context(), profileID, limit, offset)
	if err != nil {
		return err
	}
	total, err := h.keys.CountByProfile(c.Request().Context(), profileID)
	if err != nil {
		return err
	}
	items := make([]controlAPIKeyResponse, 0, len(keys))
	for _, key := range keys {
		items = append(items, toControlAPIKey(key))
	}
	return c.JSON(nethttp.StatusOK, handler.PaginationEnvelope{
		Data:       items,
		Pagination: handler.Pagination{Limit: limit, Offset: offset, Total: total},
	})
}

func (h *controlPortalHandler) createAPIKey(c echo.Context) error {
	profileID, err := parseControlUUID(c.Param("profileId"), "profile ID")
	if err != nil {
		return err
	}
	var body controlCreateAPIKeyRequest
	if err := c.Bind(&body); err != nil {
		return httperr.New(httperr.VALIDATION_ERROR, "malformed JSON body")
	}
	req := service.CreateAPIKeyRequest{
		RateLimit: body.RateLimit,
	}
	if body.ExpiresAt != nil {
		expiresAt, err := time.Parse(time.RFC3339, *body.ExpiresAt)
		if err != nil {
			return httperr.New(httperr.VALIDATION_ERROR, "expires_at must be RFC3339")
		}
		req.ExpiresAt = &expiresAt
	}
	key, rawKey, err := h.keys.CreateStandardKey(c.Request().Context(), profileID, req, nil, "control", c.RealIP(), "")
	if err != nil {
		return err
	}
	return c.JSON(nethttp.StatusCreated, map[string]any{
		"data": map[string]any{
			"api_key": rawKey,
			"key":     toControlAPIKey(key),
		},
	})
}

func (h *controlPortalHandler) deleteAPIKey(c echo.Context) error {
	profileID, err := parseControlUUID(c.Param("profileId"), "profile ID")
	if err != nil {
		return err
	}
	keyID, err := parseControlUUID(c.Param("keyId"), "key ID")
	if err != nil {
		return err
	}
	if err := h.keys.DeleteForProfile(c.Request().Context(), profileID, keyID, nil, "control", c.RealIP(), ""); err != nil {
		return err
	}
	return c.JSON(nethttp.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

type controlCreateAPIKeyRequest struct {
	RateLimit int     `json:"rate_limit"`
	ExpiresAt *string `json:"expires_at"`
}

func controlPortalMiddleware(token string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			origin := c.Request().Header.Get(echo.HeaderOrigin)
			if origin != "" {
				c.Response().Header().Set(echo.HeaderVary, echo.HeaderOrigin)
				c.Response().Header().Set(echo.HeaderAccessControlAllowOrigin, origin)
				c.Response().Header().Set(echo.HeaderAccessControlAllowHeaders, "Authorization, Content-Type, X-Control-Portal-Token")
				c.Response().Header().Set(echo.HeaderAccessControlAllowMethods, "GET, POST, PATCH, DELETE, OPTIONS")
			}
			if c.Request().Method == nethttp.MethodOptions {
				return c.NoContent(nethttp.StatusNoContent)
			}
			if !controlTokenMatches(c.Request(), token) {
				return httperr.New(httperr.AUTH_INVALID, "invalid control portal token")
			}
			return next(c)
		}
	}
}

func controlTokenMatches(req *nethttp.Request, expected string) bool {
	got := req.Header.Get("X-Control-Portal-Token")
	if got == "" {
		auth := req.Header.Get(echo.HeaderAuthorization)
		if strings.HasPrefix(auth, "Bearer ") {
			got = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		}
	}
	if got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1
}

func defaultPortalStaticDir() string {
	candidates := []string{
		filepath.Join("web", "dist"),
		filepath.Join("/app", "dense-mem", "web", "dist"),
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate
		}
	}
	return ""
}

func parseControlUUID(raw, label string) (uuid.UUID, error) {
	if raw == "" {
		return uuid.Nil, httperr.New(httperr.VALIDATION_ERROR, label+" is required")
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, httperr.New(httperr.INVALID_UUID, "invalid "+label+" format")
	}
	return id, nil
}

func controlPagination(c echo.Context) (int, int) {
	limit := 20
	offset := 0
	if raw := c.QueryParam("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			if parsed > 100 {
				parsed = 100
			}
			limit = parsed
		}
	}
	if raw := c.QueryParam("offset"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
			offset = parsed
		}
	}
	return limit, offset
}

type controlProfileResponse struct {
	ID          uuid.UUID      `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Metadata    map[string]any `json:"metadata"`
	Config      map[string]any `json:"config"`
	CreatedAt   string         `json:"created_at"`
	UpdatedAt   string         `json:"updated_at"`
}

func toControlProfile(profile *domain.Profile) controlProfileResponse {
	return controlProfileResponse{
		ID:          profile.ID,
		Name:        profile.Name,
		Description: profile.Description,
		Metadata:    profile.Metadata,
		Config:      profile.Config,
		CreatedAt:   profile.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   profile.UpdatedAt.Format(time.RFC3339),
	}
}

type controlAPIKeyResponse struct {
	ID         uuid.UUID `json:"id"`
	ProfileID  uuid.UUID `json:"profile_id"`
	KeySuffix  string    `json:"key_suffix"`
	RateLimit  int       `json:"rate_limit"`
	LastUsedAt *string   `json:"last_used_at"`
	ExpiresAt  *string   `json:"expires_at"`
	CreatedAt  string    `json:"created_at"`
}

func toControlAPIKey(key *domain.APIKey) controlAPIKeyResponse {
	return controlAPIKeyResponse{
		ID:         key.ID,
		ProfileID:  key.ProfileID,
		KeySuffix:  key.KeySuffix,
		RateLimit:  key.RateLimit,
		LastUsedAt: controlTimePtr(key.LastUsedAt),
		ExpiresAt:  controlTimePtr(key.ExpiresAt),
		CreatedAt:  key.CreatedAt.Format(time.RFC3339),
	}
}

func controlTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	formatted := t.Format(time.RFC3339)
	return &formatted
}

// ShutdownControlPortal gracefully shuts down the control portal server.
func ShutdownControlPortal(e *echo.Echo, logger observability.LogProvider) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.Shutdown(ctx); err != nil && logger != nil {
		logger.Error("control portal shutdown error", err)
		return err
	}
	return nil
}
