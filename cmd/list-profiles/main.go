package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/dense-mem/dense-mem/internal/operatorcli"
)

type cliConfig struct {
	limit  int
	offset int
}

type profileItem struct {
	ProfileID   string         `json:"profile_id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Config      map[string]any `json:"config,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type output struct {
	Items  []profileItem `json:"items"`
	Total  int64         `json:"total"`
	Limit  int           `json:"limit"`
	Offset int           `json:"offset"`
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

	profiles, err := services.ProfileService.List(ctx, cfg.limit, cfg.offset)
	if err != nil {
		return fmt.Errorf("list profiles: %w", err)
	}
	total, err := services.ProfileService.Count(ctx)
	if err != nil {
		return fmt.Errorf("count profiles: %w", err)
	}

	items := make([]profileItem, 0, len(profiles))
	for _, profile := range profiles {
		items = append(items, profileItem{
			ProfileID:   profile.ID.String(),
			Name:        profile.Name,
			Description: profile.Description,
			Metadata:    profile.Metadata,
			Config:      profile.Config,
			CreatedAt:   profile.CreatedAt,
			UpdatedAt:   profile.UpdatedAt,
		})
	}

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output{
		Items:  items,
		Total:  total,
		Limit:  cfg.limit,
		Offset: cfg.offset,
	})
}

func parseCLI(args []string, stderr io.Writer) (cliConfig, error) {
	var cfg cliConfig

	fs := flag.NewFlagSet("list-profiles", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.IntVar(&cfg.limit, "limit", 100, "Maximum number of profiles to return")
	fs.IntVar(&cfg.offset, "offset", 0, "Offset for pagination")

	if err := fs.Parse(args); err != nil {
		return cliConfig{}, err
	}
	return cfg, nil
}
