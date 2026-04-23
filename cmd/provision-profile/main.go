package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/dense-mem/dense-mem/internal/operatorcli"
	"github.com/dense-mem/dense-mem/internal/service"
)

const (
	defaultScopesCSV = "read,write"
	defaultKeyLabel  = "default"
)

type cliConfig struct {
	name         string
	description  string
	metadataJSON string
	configJSON   string
	keyLabel     string
	scopesCSV    string
	rateLimit    int
	expiresAt    string
}

type provisionOutput struct {
	ProfileID   string   `json:"profile_id"`
	ProfileName string   `json:"profile_name"`
	APIKey      string   `json:"api_key"`
	KeyLabel    string   `json:"key_label"`
	Scopes      []string `json:"scopes"`
	ExpiresAt   *string  `json:"expires_at,omitempty"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	cfg, err := parseCLI(args, stderr)
	if err != nil {
		return err
	}

	metadata, err := parseOptionalJSONObject(cfg.metadataJSON)
	if err != nil {
		return fmt.Errorf("invalid --metadata-json: %w", err)
	}
	configMap, err := parseOptionalJSONObject(cfg.configJSON)
	if err != nil {
		return fmt.Errorf("invalid --config-json: %w", err)
	}
	scopes := parseScopes(cfg.scopesCSV)
	if len(scopes) == 0 {
		return errors.New("--scopes must contain at least one scope")
	}

	var expiresAt *time.Time
	if strings.TrimSpace(cfg.expiresAt) != "" {
		t, err := time.Parse(time.RFC3339, cfg.expiresAt)
		if err != nil {
			return fmt.Errorf("invalid --expires-at: must be RFC3339: %w", err)
		}
		expiresAt = &t
	}

	dsn, err := resolvePostgresDSN(os.Getenv)
	if err != nil {
		return err
	}

	logger := operatorcli.NewLogger(stderr)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	services, err := operatorcli.OpenServices(ctx, dsn, logger)
	if err != nil {
		return err
	}
	defer services.Close()

	correlationID := operatorcli.CorrelationID()
	profile, err := services.ProfileService.Create(ctx, service.CreateProfileRequest{
		Name:        cfg.name,
		Description: cfg.description,
		Metadata:    metadata,
		Config:      configMap,
	}, nil, operatorcli.DefaultActorRole, operatorcli.DefaultClientIP, correlationID)
	if err != nil {
		return fmt.Errorf("create profile: %w", err)
	}

	_, rawKey, err := services.APIKeyService.CreateStandardKey(ctx, profile.ID, service.CreateAPIKeyRequest{
		Label:     cfg.keyLabel,
		Scopes:    scopes,
		RateLimit: cfg.rateLimit,
		ExpiresAt: expiresAt,
	}, nil, operatorcli.DefaultActorRole, operatorcli.DefaultClientIP, correlationID)
	if err != nil {
		cleanupErr := services.ProfileService.Delete(ctx, profile.ID, nil, operatorcli.DefaultActorRole, operatorcli.DefaultClientIP, correlationID)
		if cleanupErr != nil {
			return fmt.Errorf("create api key: %w (cleanup failed: %v)", err, cleanupErr)
		}
		return fmt.Errorf("create api key: %w", err)
	}

	var expiresAtStr *string
	if expiresAt != nil {
		formatted := expiresAt.UTC().Format(time.RFC3339)
		expiresAtStr = &formatted
	}

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(provisionOutput{
		ProfileID:   profile.ID.String(),
		ProfileName: profile.Name,
		APIKey:      rawKey,
		KeyLabel:    cfg.keyLabel,
		Scopes:      scopes,
		ExpiresAt:   expiresAtStr,
	})
}

func parseCLI(args []string, stderr io.Writer) (cliConfig, error) {
	var cfg cliConfig

	fs := flag.NewFlagSet("provision-profile", flag.ContinueOnError)
	fs.SetOutput(stderr)

	fs.StringVar(&cfg.name, "name", "", "Profile name (required)")
	fs.StringVar(&cfg.description, "description", "", "Profile description")
	fs.StringVar(&cfg.metadataJSON, "metadata-json", "", "Optional profile metadata JSON object")
	fs.StringVar(&cfg.configJSON, "config-json", "", "Optional profile config JSON object")
	fs.StringVar(&cfg.keyLabel, "key-label", defaultKeyLabel, "API key label")
	fs.StringVar(&cfg.scopesCSV, "scopes", defaultScopesCSV, "Comma-separated scopes for the generated key")
	fs.IntVar(&cfg.rateLimit, "rate-limit", 0, "Per-key rate limit override")
	fs.StringVar(&cfg.expiresAt, "expires-at", "", "Optional RFC3339 expiration for the generated key")

	if err := fs.Parse(args); err != nil {
		return cliConfig{}, err
	}

	cfg.name = strings.TrimSpace(cfg.name)
	cfg.description = strings.TrimSpace(cfg.description)
	cfg.keyLabel = strings.TrimSpace(cfg.keyLabel)
	cfg.scopesCSV = strings.TrimSpace(cfg.scopesCSV)
	cfg.metadataJSON = strings.TrimSpace(cfg.metadataJSON)
	cfg.configJSON = strings.TrimSpace(cfg.configJSON)
	cfg.expiresAt = strings.TrimSpace(cfg.expiresAt)

	if cfg.name == "" {
		return cliConfig{}, errors.New("--name is required")
	}
	if cfg.keyLabel == "" {
		cfg.keyLabel = defaultKeyLabel
	}

	return cfg, nil
}

func parseOptionalJSONObject(raw string) (map[string]any, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, err
	}
	if parsed == nil {
		return nil, errors.New("must be a JSON object")
	}
	return parsed, nil
}

func parseScopes(raw string) []string {
	parts := strings.Split(raw, ",")
	scopes := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		scope := strings.TrimSpace(part)
		if scope == "" {
			continue
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		scopes = append(scopes, scope)
	}
	return scopes
}

func resolvePostgresDSN(getenv func(string) string) (string, error) {
	return operatorcli.ResolvePostgresDSN(getenv)
}
