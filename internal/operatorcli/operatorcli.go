package operatorcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/dense-mem/dense-mem/internal/repository"
	"github.com/dense-mem/dense-mem/internal/service"
	"github.com/dense-mem/dense-mem/internal/storage/postgres"
)

const (
	DefaultClientIP  = "127.0.0.1"
	DefaultActorRole = "system"
)

type postgresConfig struct {
	dsn string
}

func (c postgresConfig) GetPostgresDSN() string {
	return c.dsn
}

type Services struct {
	ProfileService service.ProfileService
	APIKeyService  service.APIKeyService
	closeFn        func()
}

func (s *Services) Close() {
	if s != nil && s.closeFn != nil {
		s.closeFn()
	}
}

func NewLogger(stderr io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

func CorrelationID() string {
	return uuid.NewString()
}

func OpenServices(ctx context.Context, dsn string, logger *slog.Logger) (*Services, error) {
	pgClient, err := postgres.OpenWithClient(ctx, postgresConfig{dsn: dsn})
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}

	rls := postgres.NewRLS()
	auditSvc := service.NewAuditServiceWithLogger(pgClient.GetDB(), logger)
	profileRepo := repository.NewProfileRepository(pgClient.GetDB(), rls)
	apiKeyRepo := repository.NewAPIKeyRepository(pgClient.GetDB(), rls)
	profileSvc := service.NewProfileServiceWithLogger(profileRepo, auditSvc, nil, logger)
	apiKeySvc := service.NewAPIKeyServiceWithLogger(apiKeyRepo, profileSvc, auditSvc, nil, nil, logger)

	return &Services{
		ProfileService: profileSvc,
		APIKeyService:  apiKeySvc,
		closeFn: func() {
			_ = pgClient.Close()
		},
	}, nil
}

func ResolvePostgresDSN(getenv func(string) string) (string, error) {
	if dsn := strings.TrimSpace(getenv("POSTGRES_DSN")); dsn != "" {
		return dsn, nil
	}

	host := strings.TrimSpace(getenv("POSTGRES_HOST"))
	user := strings.TrimSpace(getenv("POSTGRES_USER"))
	password := getenv("POSTGRES_PASSWORD")
	dbName := strings.TrimSpace(getenv("POSTGRES_DB"))
	port := strings.TrimSpace(getenv("POSTGRES_PORT"))
	sslmode := strings.TrimSpace(getenv("POSTGRES_SSLMODE"))

	if host == "" {
		return "", errors.New("POSTGRES_HOST or POSTGRES_DSN is required")
	}
	if user == "" {
		return "", errors.New("POSTGRES_USER or POSTGRES_DSN is required")
	}
	if password == "" {
		return "", errors.New("POSTGRES_PASSWORD or POSTGRES_DSN is required")
	}
	if dbName == "" {
		return "", errors.New("POSTGRES_DB or POSTGRES_DSN is required")
	}
	if port == "" {
		port = "5432"
	}
	if sslmode == "" {
		sslmode = "disable"
	}

	return fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		user,
		password,
		host,
		port,
		dbName,
		sslmode,
	), nil
}
