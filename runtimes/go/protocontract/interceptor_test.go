package protocontract

import (
	"encoding/json"
	"os"
	"testing"
)

func TestSharedContractVectors(t *testing.T) {
	data, err := os.ReadFile("../../contract-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct {
		Name, API, Server string
		Values            []string
		Compatible        bool
	}
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	for _, vector := range vectors {
		t.Run(vector.Name, func(t *testing.T) {
			err := CompatibleValues(vector.API, vector.Values, vector.Server)
			if (err == nil) != vector.Compatible {
				t.Fatalf("CompatibleValues() error = %v, want compatible=%v", err, vector.Compatible)
			}
		})
	}
}

func TestCanonicalConfigurationValidationIsEager(t *testing.T) {
	for _, tc := range []struct {
		api, version string
		valid        bool
	}{
		{"example/orders:v1", "1.2.3", true}, {"demo@echo", "1.2.3", false}, {"demo.echo", "01.2.3", false},
		{"demo.echo", "2147483648.0.0", false}, {"demo.echo", "1.2.3\n", false},
	} {
		_, err := NewClient(tc.api, tc.version, "demo.v1.EchoService")
		if (err == nil) != tc.valid {
			t.Errorf("NewClient(%q, %q): error=%v, want valid=%v", tc.api, tc.version, err, tc.valid)
		}
	}
}
