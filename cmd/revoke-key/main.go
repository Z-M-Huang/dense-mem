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
	keyID     string
}

type output struct {
	ProfileID string `json:"profile_id"`
	KeyID     string `json:"key_id"`
	Status    string `json:"status"`
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

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	services, err := operatorcli.OpenServices(ctx, dsn, operatorcli.NewLogger(stderr))
	if err != nil {
		return err
	}
	defer services.Close()

	if err := services.APIKeyService.RevokeForProfile(ctx, profileID, keyID, nil, operatorcli.DefaultActorRole, operatorcli.DefaultClientIP, operatorcli.CorrelationID()); err != nil {
		return fmt.Errorf("revoke key: %w", err)
	}

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output{
		ProfileID: profileID.String(),
		KeyID:     keyID.String(),
		Status:    "revoked",
	})
}

func parseCLI(args []string, stderr io.Writer) (cliConfig, error) {
	var cfg cliConfig

	fs := flag.NewFlagSet("revoke-key", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&cfg.profileID, "profile-id", "", "Profile UUID that owns the key")
	fs.StringVar(&cfg.keyID, "key-id", "", "API key UUID to revoke")

	if err := fs.Parse(args); err != nil {
		return cliConfig{}, err
	}
	cfg.profileID = strings.TrimSpace(cfg.profileID)
	cfg.keyID = strings.TrimSpace(cfg.keyID)
	if cfg.profileID == "" {
		return cliConfig{}, errors.New("--profile-id is required")
	}
	if cfg.keyID == "" {
		return cliConfig{}, errors.New("--key-id is required")
	}
	return cfg, nil
}
