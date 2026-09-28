package contract

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestGenerateClientLanguages(t *testing.T) {
	for _, language := range []string{"typescript", "python", "go", "java", "dotnet"} {
		t.Run(language, func(t *testing.T) {
			s := &Snapshot{Format: 1, API: "example/orders:v1", Version: "2.3.4", Service: Service{Name: "example.v1.Orders"}}
			first, err := Generate(s, language, "")
			if err != nil {
				t.Fatal(err)
			}
			for _, value := range []string{s.API, s.Version, s.Service.Name, "DO NOT EDIT"} {
				if !strings.Contains(string(first), value) {
					t.Fatalf("generated module does not contain %q", value)
				}
			}
			if language != "typescript" && !strings.Contains(strings.ToLower(string(first)), "serverinterceptor") {
				t.Fatal("generated contract is missing its server entry point")
			}
			second, err := Generate(s, language, "")
			if err != nil || !bytes.Equal(first, second) {
				t.Fatal("generation is not deterministic")
			}
			s.Version = "4.0.0"
			next, err := Generate(s, language, "")
			if err != nil || !strings.Contains(string(next), `"4.0.0"`) || strings.Contains(string(next), `"2.3.4"`) {
				t.Fatal("generated version did not follow the lock")
			}
			if _, err := Generate(nil, language, ""); err == nil {
				t.Fatal("accepted missing lock")
			}
			s.API = "api\nother"
			if _, err := Generate(s, language, ""); err == nil {
				t.Fatal("accepted invalid metadata")
			}
		})
	}
}

func TestGeneratePackageValidation(t *testing.T) {
	s := &Snapshot{Format: 1, API: "api", Version: "1.0.0", Service: Service{Name: "Service"}}
	for _, language := range []string{"go", "java", "dotnet"} {
		for _, name := range []string{"package", "_", "bad-name", "a;evil", "a..b"} {
			if _, err := Generate(s, language, name); err == nil {
				t.Fatalf("%s accepted %q", language, name)
			}
		}
		name := "orderscontract"
		if language != "go" {
			name = "contracts.orders"
		}
		output, err := Generate(s, language, name)
		if err != nil || !strings.Contains(string(output), name) {
			t.Fatalf("%s package missing: %v", language, err)
		}
	}
	for _, language := range []string{"python", "typescript"} {
		if _, err := Generate(s, language, "unused"); err == nil {
			t.Fatal("ignored unsupported package option")
		}
	}
	if _, err := Generate(s, "ruby", ""); err == nil {
		t.Fatal("accepted unsupported language")
	}
}

func TestCheckedInGoContractMatchesLock(t *testing.T) {
	s, err := Read("../../contracts/demo.echo.json")
	if err != nil {
		t.Fatal(err)
	}
	want, err := Generate(s, "go", "echocontract")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../../gen/go/contracts/echo/contract.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n")), want) {
		t.Fatal("regenerate gen/go/contracts/echo/contract.go from contracts/demo.echo.json")
	}
}
