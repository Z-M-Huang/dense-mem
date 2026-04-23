package main

import "testing"

func TestParseScopes(t *testing.T) {
	got := parseScopes("read, write,read , ,write,custom")
	want := []string{"read", "write", "custom"}

	if len(got) != len(want) {
		t.Fatalf("len(parseScopes) = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parseScopes()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseOptionalJSONObject(t *testing.T) {
	got, err := parseOptionalJSONObject(`{"team":"alpha"}`)
	if err != nil {
		t.Fatalf("parseOptionalJSONObject() unexpected error: %v", err)
	}
	if got["team"] != "alpha" {
		t.Fatalf("parseOptionalJSONObject()[team] = %v, want alpha", got["team"])
	}
}

func TestParseOptionalJSONObjectRejectsNonObject(t *testing.T) {
	_, err := parseOptionalJSONObject(`["nope"]`)
	if err == nil {
		t.Fatal("parseOptionalJSONObject() expected error, got nil")
	}
}

func TestResolvePostgresDSNUsesExplicitDSN(t *testing.T) {
	dsn, err := resolvePostgresDSN(func(key string) string {
		if key == "POSTGRES_DSN" {
			return "postgres://u:p@db:5432/app?sslmode=disable"
		}
		return ""
	})
	if err != nil {
		t.Fatalf("resolvePostgresDSN() unexpected error: %v", err)
	}
	if dsn != "postgres://u:p@db:5432/app?sslmode=disable" {
		t.Fatalf("resolvePostgresDSN() = %q", dsn)
	}
}

func TestResolvePostgresDSNBuildsFromComponents(t *testing.T) {
	env := map[string]string{
		"POSTGRES_HOST":     "db",
		"POSTGRES_USER":     "densemem",
		"POSTGRES_PASSWORD": "secret",
		"POSTGRES_DB":       "app",
	}
	dsn, err := resolvePostgresDSN(func(key string) string { return env[key] })
	if err != nil {
		t.Fatalf("resolvePostgresDSN() unexpected error: %v", err)
	}
	want := "postgres://densemem:secret@db:5432/app?sslmode=disable"
	if dsn != want {
		t.Fatalf("resolvePostgresDSN() = %q, want %q", dsn, want)
	}
}

func TestResolvePostgresDSNRequiresComponents(t *testing.T) {
	_, err := resolvePostgresDSN(func(string) string { return "" })
	if err == nil {
		t.Fatal("resolvePostgresDSN() expected error, got nil")
	}
}
