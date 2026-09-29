package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

func TestRuntimeGraphPreservesDescriptorSemantics(t *testing.T) {
	set := generatorFixture()
	graph, err := RuntimeGraph(set, "orders.v1.OrdersService")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(graph)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, want := range []string{
		`"localName":"getURL"`,
		`"localName":"foo2Bar"`,
		`"type":"number","value":0.1`,
		`"localName":"MODE_ZERO"`,
		`"localName":"ZERO"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("runtime graph missing %s: %s", want, text)
		}
	}
}

func TestPluginValidatesExactRequestAndEmitsDeterministicMultiServiceBindings(t *testing.T) {
	tmp := t.TempDir()
	set := generatorFixture()
	for _, name := range []string{"orders.v1.OrdersService", "orders.v1.AuditService"} {
		snapshot, err := Build(set, name, "example/orders:v1", "1.1.0")
		if err != nil {
			t.Fatal(err)
		}
		if err := Write(filepath.Join(tmp, strings.TrimPrefix(name, "orders.v1.")+".json"), snapshot); err != nil {
			t.Fatal(err)
		}
	}
	bindings := `{"bindings":[{"service":"orders.v1.OrdersService","lock":"OrdersService.json","output":"gen/orders.ts","service_import":"./orders_pb.js","service_export":"OrdersService"},{"service":"orders.v1.AuditService","lock":"AuditService.json","output":"gen/audit.ts","service_import":"./audit_pb.js","service_export":"AuditService"}]}`
	bindingsPath := filepath.Join(tmp, "bindings.json")
	if err := os.WriteFile(bindingsPath, []byte(bindings), 0600); err != nil {
		t.Fatal(err)
	}
	request := &pluginpb.CodeGeneratorRequest{FileToGenerate: []string{"fixture.proto"}, ProtoFile: set.File}
	first, err := GeneratePluginResponse(request, "typescript", bindingsPath)
	if err != nil {
		t.Fatal(err)
	}
	second, err := GeneratePluginResponse(request, "typescript", bindingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.File) != 2 || proto.Size(first) != proto.Size(second) || !proto.Equal(first, second) {
		t.Fatalf("multi-service output is not deterministic: %#v", first.File)
	}
	if first.File[0].GetName() != "gen/audit.ts" || !strings.Contains(first.File[0].GetContent(), `service: serviceDescriptor`) || !strings.Contains(first.File[0].GetContent(), `api: "example/orders:v1"`) {
		t.Fatalf("unexpected generated TypeScript: %s", first.File[0].GetContent())
	}
	changed := proto.Clone(request).(*pluginpb.CodeGeneratorRequest)
	changed.ProtoFile[0].MessageType[0].Field[0].Name = proto.String("renamed")
	if _, err := GeneratePluginResponse(changed, "typescript", bindingsPath); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("expected stale lock rejection, got %v", err)
	}
}

func TestPluginRejectsUnsafeAndDuplicateOutputs(t *testing.T) {
	for _, output := range []string{"../escape.ts", "/absolute.ts", "C:/escape.ts", "gen\\bad.ts"} {
		if safeOutputPath(output) {
			t.Errorf("safeOutputPath(%q) = true", output)
		}
	}
	if !safeOutputPath("gen/orders.ts") {
		t.Fatal("ordinary protoc-relative output rejected")
	}
}

func TestBindingsUseBoundedCanonicalStrictJSON(t *testing.T) {
	for _, input := range []string{
		`{"bindings":[],"bindings":[]}`,
		`{"Bindings":[]}`,
		`{"bindings":[]} {"ignored":true}`,
	} {
		path := filepath.Join(t.TempDir(), "bindings.json")
		if err := os.WriteFile(path, []byte(input), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := readBindings(path); err == nil {
			t.Errorf("accepted invalid bindings JSON %q", input)
		}
	}
}

func TestPluginUsesNativeLanguageGenerators(t *testing.T) {
	tmp := t.TempDir()
	set := generatorFixture()
	lock, err := Build(set, "orders.v1.OrdersService", "example/orders:v1", "1.1.0")
	if err != nil {
		t.Fatal(err)
	}
	if err := Write(filepath.Join(tmp, "orders.json"), lock); err != nil {
		t.Fatal(err)
	}
	config := `{"bindings":[{"service":"orders.v1.OrdersService","lock":"orders.json","output":"out/generated"}]}`
	path := filepath.Join(tmp, "bindings.json")
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	request := &pluginpb.CodeGeneratorRequest{FileToGenerate: []string{"fixture.proto"}, ProtoFile: set.File}
	for _, language := range []string{"go", "python", "java", "dotnet"} {
		response, err := GeneratePluginResponse(request, language, path)
		if err != nil {
			t.Fatalf("%s plugin generation: %v", language, err)
		}
		if len(response.File) != 1 || !strings.Contains(response.File[0].GetContent(), lock.Version) {
			t.Errorf("%s plugin output missing contract identity: %#v", language, response.File)
		}
	}
}

func generatorFixture() *descriptorpb.FileDescriptorSet {
	labelOptional := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	fieldTypeFloat := descriptorpb.FieldDescriptorProto_TYPE_FLOAT
	fieldTypeEnum := descriptorpb.FieldDescriptorProto_TYPE_ENUM
	fieldTypeInt32 := descriptorpb.FieldDescriptorProto_TYPE_INT32
	methodInput := ".orders.v1.Request"
	methodOutput := ".orders.v1.Response"
	modeName, stateName := "MODE", "State"
	return &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{{
		Name: proto.String("fixture.proto"), Package: proto.String("orders.v1"), Syntax: proto.String("proto2"),
		EnumType: []*descriptorpb.EnumDescriptorProto{
			{Name: &modeName, Value: []*descriptorpb.EnumValueDescriptorProto{{Name: proto.String("MODE_ZERO"), Number: proto.Int32(0)}, {Name: proto.String("OTHER"), Number: proto.Int32(1)}}},
			{Name: &stateName, Value: []*descriptorpb.EnumValueDescriptorProto{{Name: proto.String("STATE_ZERO"), Number: proto.Int32(0)}, {Name: proto.String("STATE_ONE"), Number: proto.Int32(1)}}},
		},
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: proto.String("Request"), Field: []*descriptorpb.FieldDescriptorProto{
				{Name: proto.String("float_value"), Number: proto.Int32(1), Label: &labelOptional, Type: &fieldTypeFloat, DefaultValue: proto.String("0.1")},
				{Name: proto.String("mode"), Number: proto.Int32(2), Label: &labelOptional, Type: &fieldTypeEnum, TypeName: proto.String(".orders.v1.MODE"), DefaultValue: proto.String("MODE_ZERO")},
				{Name: proto.String("foo_2_bar"), Number: proto.Int32(3), Label: &labelOptional, Type: &fieldTypeInt32},
				{Name: proto.String("state"), Number: proto.Int32(4), Label: &labelOptional, Type: &fieldTypeEnum, TypeName: proto.String(".orders.v1.State")},
			}},
			{Name: proto.String("Response")},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{
			{Name: proto.String("OrdersService"), Method: []*descriptorpb.MethodDescriptorProto{{Name: proto.String("GetURL"), InputType: &methodInput, OutputType: &methodOutput}}},
			{Name: proto.String("AuditService"), Method: []*descriptorpb.MethodDescriptorProto{{Name: proto.String("GetURL"), InputType: &methodInput, OutputType: &methodOutput}}},
		},
	}}}
}
