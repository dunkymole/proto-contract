package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateFromLock(t *testing.T) {
	dir := t.TempDir()
	lock := filepath.Join(dir, "contract.json")
	contents := `{"format":1,"api":"example.orders","version":"2.3.0","service":{"name":"example.v1.Orders"}}`
	if err := os.WriteFile(lock, []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "gen", "orders_contract.ts")
	if err := generate([]string{"--lock", lock, "--lang", "typescript", "--out", out}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil || !strings.Contains(string(b), `version: "2.3.0"`) {
		t.Fatalf("invalid generated module: %s, %v", b, err)
	}
	b, err = os.ReadFile(lock)
	if err != nil || string(b) != contents {
		t.Fatal("generation changed the input lock")
	}
	if err := generate([]string{"--lock", lock, "--out", lock}); err == nil {
		t.Fatal("allowed overwriting the lock")
	}
}

func TestGenerateRequiresSupportedOptions(t *testing.T) {
	for _, args := range [][]string{
		{}, {"--lock", "missing.json"}, {"--out", "out.ts"},
		{"--lock", "missing.json", "--out", "out.ts", "--lang", "ruby"},
	} {
		if err := generate(args); err == nil {
			t.Fatalf("accepted invalid arguments: %v", args)
		}
	}
}

func TestCheckAndUpdateRejectLegacyLocksExplicitly(t *testing.T) {
	dir := t.TempDir()
	lock := filepath.Join(dir, "legacy.json")
	contents := `{"format":1,"api":"demo.echo","version":"1.1.0","service":{"name":"demo.v1.EchoService"}}`
	if err := os.WriteFile(lock, []byte(contents), 0644); err != nil { t.Fatal(err) }
	args := []string{"--proto", "schema.proto", "--service", "demo.v1.EchoService", "--lock", lock}
	for name, command := range map[string]func([]string) error{"check": check, "update": update} {
		if err := command(args); err == nil || !strings.Contains(err.Error(), "lacks captured descriptor semantics") {
			t.Fatalf("%s did not explain format-1 rejection: %v", name, err)
		}
	}
	b, err := os.ReadFile(lock)
	if err != nil || string(b) != contents { t.Fatal("legacy rejection modified the lock") }
}
