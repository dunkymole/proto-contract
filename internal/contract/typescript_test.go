package contract

import (
	"bytes"
	"strings"
	"testing"
)

func TestTypeScriptUsesLock(t *testing.T) {
	s := generationFixture(t)
	s.API = "example.orders"
	s.Digest, _ = calculateDigest(s)
	first, err := TypeScript(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`api: "example.orders"`, `version: "2.3.4"`, `service: "example.v1.Orders"`} {
		if !strings.Contains(string(first), want) {
			t.Fatalf("missing %s", want)
		}
	}
	second, err := TypeScript(s)
	if err != nil || !bytes.Equal(first, second) {
		t.Fatal("generation must be deterministic")
	}
	s.Version = "3.0.0"
	updated, err := TypeScript(s)
	if err != nil || !strings.Contains(string(updated), `version: "3.0.0"`) || bytes.Equal(first, updated) {
		t.Fatal("generated version must follow the lock")
	}
}

func TestTypeScriptRejectsInvalidLocks(t *testing.T) {
	for _, s := range []*Snapshot{
		nil,
		{Format: 3, API: "api", Version: "1.0.0", Service: Service{Name: "Service"}},
		{Format: 1, API: "api", Version: "1.0.0", Service: Service{Name: "Service"}},
		{Format: 2, API: "api@other", Version: "1.0.0", Service: Service{Name: "Service"}},
		{Format: 2, API: "api", Version: "invalid", Service: Service{Name: "Service"}},
		{Format: 2, API: "api", Version: "1.0.0"},
	} {
		if _, err := TypeScript(s); err == nil {
			t.Fatalf("accepted invalid lock: %+v", s)
		}
	}
}
