package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dunkymole/proto-contract/internal/contract"
)

func TestGenerateFromLock(t *testing.T) {
	dir := t.TempDir()
	lock := filepath.Join("..", "..", "contracts", "demo.echo.json")
	snapshot, err := contract.Read(lock)
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(lock)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "gen", "orders_contract.ts")
	if err := generate([]string{"--lock", lock, "--lang", "typescript", "--out", out}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil || !strings.Contains(string(b), `version: "`+snapshot.Version+`"`) {
		t.Fatalf("invalid generated module: %s, %v", b, err)
	}
	b, err = os.ReadFile(lock)
	if err != nil || !bytes.Equal(b, contents) {
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

func TestSnapshotRefusesOverwriteUnlessForced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.json")
	if err := os.WriteFile(path, []byte("keep me"), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"--proto", "missing.proto", "--service", "test.Service", "--api", "test", "--version", "1.0.0", "--out", path}
	err := snapshot(args)
	if err == nil || !strings.Contains(err.Error(), "pass --force") {
		t.Fatalf("snapshot overwrite error = %v", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "keep me" {
		t.Fatal("snapshot overwrite refusal changed existing file")
	}
}

func TestCheckAndUpdateRejectUnsupportedLockFormatsExplicitly(t *testing.T) {
	dir := t.TempDir()
	lock := filepath.Join(dir, "legacy.json")
	contents := `{"format":1,"api":"demo.echo","version":"1.1.0","service":{"name":"demo.v1.EchoService"}}`
	if err := os.WriteFile(lock, []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
	args := []string{"--proto", "schema.proto", "--service", "demo.v1.EchoService", "--lock", lock}
	for name, command := range map[string]func([]string) error{"check": check, "update": update} {
		if err := command(args); err == nil || !strings.Contains(err.Error(), "unsupported contract lock format") {
			t.Fatalf("%s did not explain unsupported format rejection: %v", name, err)
		}
	}
	b, err := os.ReadFile(lock)
	if err != nil || string(b) != contents {
		t.Fatal("legacy rejection modified the lock")
	}
}
