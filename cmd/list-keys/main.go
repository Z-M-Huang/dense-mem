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

	"github.com/dense-mem/dense-mem/internal/operatorcli"
)

type cliConfig struct {
	profileID string
	limit     int
	offset    int
}

type keyItem struct {
	KeyID      string     `json:"key_id"`
	Label      string     `json:"label"`
	Scopes     []string   `json:"scopes"`
	RateLimit  int        `json:"rate_limit"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

type output struct {
	ProfileID string    `json:"profile_id"`
	Items     []keyItem `json:"items"`
	Total     int64     `json:"total"`
	Limit     int       `json:"limit"`
	Offset    int       `json:"offset"`
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

	dsn, err := operatorcli.ResolvePostgresDSN(os.Getenv)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	services, err := operatorcli.OpenServices(ctx, dsn, operatorcli.NewLogger(stderr))
	if err != nil {
		return err
	}
	defer services.Close()

	keys, err := services.APIKeyService.ListByProfile(ctx, profileID, cfg.limit, cfg.offset)
	if err != nil {
		return fmt.Errorf("list keys: %w", err)
	}
	total, err := services.APIKeyService.CountByProfile(ctx, profileID)
	if err != nil {
		return fmt.Errorf("count keys: %w", err)
	}

	items := make([]keyItem, 0, len(keys))
	for _, key := range keys {
		items = append(items, keyItem{
			KeyID:      key.ID.String(),
			Label:      key.Label,
			Scopes:     key.Scopes,
			RateLimit:  key.RateLimit,
			LastUsedAt: key.LastUsedAt,
			ExpiresAt:  key.ExpiresAt,
			CreatedAt:  key.CreatedAt,
			RevokedAt:  key.RevokedAt,
		})
	}

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output{
		ProfileID: profileID.String(),
		Items:     items,
		Total:     total,
		Limit:     cfg.limit,
		Offset:    cfg.offset,
	})
}

func parseCLI(args []string, stderr io.Writer) (cliConfig, error) {
	var cfg cliConfig

	fs := flag.NewFlagSet("list-keys", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&cfg.profileID, "profile-id", "", "Profile UUID to inspect")
	fs.IntVar(&cfg.limit, "limit", 100, "Maximum number of keys to return")
	fs.IntVar(&cfg.offset, "offset", 0, "Offset for pagination")

	if err := fs.Parse(args); err != nil {
		return cliConfig{}, err
	}
	cfg.profileID = strings.TrimSpace(cfg.profileID)
	if cfg.profileID == "" {
		return cliConfig{}, errors.New("--profile-id is required")
	}
	return cfg, nil
}
