package contract

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

func inferenceService(t *testing.T, name, path, pkg string, opts *descriptorpb.FileOptions) protoreflect.ServiceDescriptor {
	t.Helper()
	file, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{Name: proto.String(path), Syntax: proto.String("proto3"), Package: proto.String(pkg), Options: opts, Service: []*descriptorpb.ServiceDescriptorProto{{Name: proto.String(name)}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return file.Services().Get(0)
}

func TestNativeInferenceNamingAndIndependentOverrides(t *testing.T) {
	opts := &descriptorpb.FileOptions{GoPackage: proto.String("example.test/pb;differentAlias"), JavaPackage: proto.String("com.example.types"), CsharpNamespace: proto.String("Explicit.Types")}
	for _, tc := range []struct{ name, goName string }{{"snake_case", "SnakeCase"}, {"foo__bar", "Foo_Bar"}, {"_leading", "XLeading"}, {"HTTP_service2API", "HTTPService2API"}, {"foo2_bar", "Foo2Bar"}} {
		service := inferenceService(t, tc.name, "nested-dir/name-edge.proto", "ignored.proto_namespace", opts)
		for _, lang := range []string{"go", "python", "java", "dotnet"} {
			expectedImport := map[string]string{"go": "example.test/pb", "python": "nested_dir.name_edge_pb2_grpc", "java": "com.example.types", "dotnet": "Explicit.Types"}[lang]
			expectedName := tc.name
			if lang == "go" {
				expectedName = tc.goName
			}
			imp, exp, err := inferNativeBinding(service, lang, "", "")
			if err != nil || imp != expectedImport || exp != expectedName {
				t.Fatalf("%s/%s got %q %q %v", lang, tc.name, imp, exp, err)
			}
			override := "relocated.types"
			if lang == "go" {
				override = "example.test/relocated"
			}
			imp, exp, err = inferNativeBinding(service, lang, override, "")
			if err != nil || imp != override || exp != expectedName {
				t.Fatalf("%s import-only override failed: %q %q %v", lang, imp, exp, err)
			}
			imp, exp, err = inferNativeBinding(service, lang, "", "CustomService")
			if err != nil || imp != expectedImport || exp != "CustomService" {
				t.Fatalf("%s export-only override failed: %q %q %v", lang, imp, exp, err)
			}
		}
	}
}

func TestInferenceDefaultsFailuresAndGlobalNamespace(t *testing.T) {
	service := inferenceService(t, "snake_case", "fallback.proto", "some_api.v2alpha.HTTP_api", nil)
	imp, exp, err := inferNativeBinding(service, "dotnet", "", "")
	if err != nil || imp != "SomeApi.V2Alpha.HTTPApi" || exp != "snake_case" {
		t.Fatalf("C# fallback %q %q %v", imp, exp, err)
	}
	imp, _, err = inferNativeBinding(service, "java", "", "")
	if err != nil || imp != "some_api.v2alpha.HTTP_api" {
		t.Fatalf("Java fallback %q %v", imp, err)
	}
	if _, _, err := inferNativeBinding(service, "go", "", ""); err == nil || !strings.Contains(err.Error(), "go_package") {
		t.Fatalf("missing Go package error: %v", err)
	}
	if _, _, err := inferNativeBinding(service, "go", "consumer.test/pb", ""); err != nil {
		t.Fatal(err)
	}
	global := inferenceService(t, "GlobalService", "global.proto", "", &descriptorpb.FileOptions{CsharpNamespace: proto.String("")})
	imp, exp, err = inferNativeBinding(global, "dotnet", "", "")
	if err != nil || imp != "" || exp != "GlobalService" {
		t.Fatalf("global namespace: %q %q %v", imp, exp, err)
	}
	snapshot := generationFixture(t)
	source, err := GenerateBound(snapshot, "dotnet", "", imp, exp, "both")
	if err != nil || !strings.Contains(string(source), "global::GlobalService.GlobalServiceClient") || strings.Contains(string(source), "global::.") {
		t.Fatalf("global C# output: %s %v", source, err)
	}
	if _, _, err := inferNativeBinding(global, "java", "", ""); err == nil {
		t.Fatal("accepted unusable Java default package")
	}
	dotted := inferenceService(t, "Service", "foo.bar.proto", "pkg", nil)
	if _, _, err := inferNativeBinding(dotted, "python", "", ""); err == nil {
		t.Fatal("guessed unimportable dotted filename")
	}
	if _, _, err := inferNativeBinding(dotted, "python", "relocated.pb", ""); err != nil {
		t.Fatal(err)
	}
	keyword := inferenceService(t, "class", "keyword.proto", "pkg", nil)
	for _, lang := range []string{"go", "java"} {
		imp := ""
		if lang == "go" {
			imp = "example.test/pb"
		}
		if _, _, err := inferNativeBinding(keyword, lang, imp, ""); err != nil {
			t.Fatalf("%s keyword service: %v", lang, err)
		}
	}
	if _, _, err := inferNativeBinding(keyword, "python", "", ""); err == nil {
		t.Fatal("accepted Python keyword service; gRPC emits an invalid experimental class")
	}
	if _, _, err := inferNativeBinding(keyword, "dotnet", "", ""); err == nil {
		t.Fatal("accepted C# keyword type emitted verbatim by gRPC")
	}
}

func TestDescriptorInferenceNeverFallsBackToGenericHelpers(t *testing.T) {
	set := generatorFixture()
	snapshot, err := Build(set, "orders.v1.OrdersService", "orders", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"go", "java", "python", "dotnet"} {
		source, err := GenerateNativeFromDescriptors(snapshot, set, lang, "", "", "", "")
		if err != nil {
			t.Fatalf("%s: %v", lang, err)
		}
		if !strings.Contains(string(source), "OrdersService") {
			t.Fatalf("%s missing typed service", lang)
		}
		changed := proto.Clone(set).(*descriptorpb.FileDescriptorSet)
		changed.File[0].MessageType[0].Field[0].Name = proto.String("changed")
		if _, err := GenerateNativeFromDescriptors(snapshot, changed, lang, "", "", "", ""); err == nil {
			t.Fatalf("%s accepted stale lock", lang)
		}
	}
	set.File[0].Options.GoPackage = nil
	// Descriptor options are not part of the wire contract, so this lock stays valid.
	if _, err := GenerateNativeFromDescriptors(snapshot, set, "go", "", "", "", ""); err == nil {
		t.Fatal("silently used generic API without go_package")
	}
}
