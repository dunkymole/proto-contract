package contract

import (
	"strings"
	"testing"
)

func TestNativeBindingValidation(t *testing.T) {
	for _, language := range []string{"go", "python", "java", "dotnet"} {
		for _, pair := range [][2]string{{"example.pb", ""}, {"evil\nimport", "Orders"}, {"example.pb", "Orders;evil"}} {
			if _, err := GenerateBound(generationFixture(t), language, "", pair[0], pair[1], ""); err == nil {
				t.Fatalf("%s accepted unsafe binding %#v", language, pair)
			}
		}
	}
	if _, err := GenerateBound(generationFixture(t), "go", "", "example/pb", "Orders", "server"); err == nil {
		t.Fatal("accepted unsupported target")
	}
	if _, err := GenerateBound(generationFixture(t), "dotnet", "", "Example.Pb", "Orders", "invalid"); err == nil {
		t.Fatal("accepted invalid target")
	}
}

func TestDotnetBindingSeparatesClientAndServerDependencies(t *testing.T) {
	for _, target := range []string{"client", "server", "both"} {
		source, err := GenerateBound(generationFixture(t), "dotnet", "", "Example.Pb", "Orders", target)
		if err != nil {
			t.Fatal(err)
		}
		hasClient := strings.Contains(string(source), "Example.Pb.Orders.OrdersClient")
		hasServer := strings.Contains(string(source), "MapService<TService>")
		if hasClient != (target != "server") || hasServer != (target != "client") {
			t.Fatalf("wrong dependencies for target %s", target)
		}
		if target == "client" && strings.Contains(string(source), "Microsoft.AspNetCore") {
			t.Fatal("client output requires ASP.NET")
		}
	}
}
