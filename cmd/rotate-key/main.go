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

	"github.com/google/uuid"

	"github.com/markhuangai/dense-mem/internal/operatorcli"
	"github.com/markhuangai/dense-mem/internal/service"
)

type cliConfig struct {
	profileID string
	keyID     string
	expiresAt string
}

type output struct {
	ProfileID string  `json:"profile_id"`
	OldKeyID  string  `json:"old_key_id"`
	NewKeyID  string  `json:"new_key_id"`
	APIKey    string  `json:"api_key"`
	ExpiresAt *string `json:"expires_at,omitempty"`
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

	profileID, err := uuid.Parse(cfg.profileID)
	if err != nil {
		return fmt.Errorf("invalid --profile-id: %w", err)
	}
	keyID, err := uuid.Parse(cfg.keyID)
	if err != nil {
		return fmt.Errorf("invalid --key-id: %w", err)
	}

	dsn, err := operatorcli.ResolvePostgresDSN(os.Getenv)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	services, err := operatorcli.OpenServices(ctx, dsn, operatorcli.NewLogger(stderr))
	if err != nil {
		return err
	}
	defer services.Close()

	existing, err := services.APIKeyService.GetByIDForProfile(ctx, profileID, keyID)
	if err != nil {
		return fmt.Errorf("load existing key: %w", err)
	}

	expiresAt := existing.ExpiresAt
	if cfg.expiresAt != "" {
		parsed, err := time.Parse(time.RFC3339, cfg.expiresAt)
		if err != nil {
			return fmt.Errorf("invalid --expires-at: must be RFC3339: %w", err)
		}
		expiresAt = &parsed
	}

	correlationID := operatorcli.CorrelationID()
	newKey, rawKey, err := services.APIKeyService.CreateStandardKey(ctx, profileID, service.CreateAPIKeyRequest{
		RateLimit: existing.RateLimit,
		ExpiresAt: expiresAt,
	}, nil, operatorcli.DefaultActorRole, operatorcli.DefaultClientIP, correlationID)
	if err != nil {
		return fmt.Errorf("create replacement key: %w", err)
	}

	if err := services.APIKeyService.DeleteForProfile(ctx, profileID, keyID, nil, operatorcli.DefaultActorRole, operatorcli.DefaultClientIP, correlationID); err != nil {
		rollbackErr := services.APIKeyService.DeleteForProfile(ctx, profileID, newKey.ID, nil, operatorcli.DefaultActorRole, operatorcli.DefaultClientIP, correlationID)
		if rollbackErr != nil {
			return fmt.Errorf("delete old key: %w (rollback failed for new key %s: %v)", err, newKey.ID.String(), rollbackErr)
		}
		return fmt.Errorf("delete old key: %w", err)
	}

	var expiresAtStr *string
	if expiresAt != nil {
		formatted := expiresAt.UTC().Format(time.RFC3339)
		expiresAtStr = &formatted
	}

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output{
		ProfileID: profileID.String(),
		OldKeyID:  keyID.String(),
		NewKeyID:  newKey.ID.String(),
		APIKey:    rawKey,
		ExpiresAt: expiresAtStr,
	})
}

func parseCLI(args []string, stderr io.Writer) (cliConfig, error) {
	var cfg cliConfig

	fs := flag.NewFlagSet("rotate-key", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&cfg.profileID, "profile-id", "", "Profile UUID that owns the key")
	fs.StringVar(&cfg.keyID, "key-id", "", "API key UUID to rotate")
	fs.StringVar(&cfg.expiresAt, "expires-at", "", "Optional RFC3339 expiration override for the replacement key")

	if err := fs.Parse(args); err != nil {
		return cliConfig{}, err
	}
	cfg.profileID = strings.TrimSpace(cfg.profileID)
	cfg.keyID = strings.TrimSpace(cfg.keyID)
	cfg.expiresAt = strings.TrimSpace(cfg.expiresAt)

	if cfg.profileID == "" {
		return cliConfig{}, errors.New("--profile-id is required")
	}
	if cfg.keyID == "" {
		return cliConfig{}, errors.New("--key-id is required")
	}

	return cfg, nil
}
