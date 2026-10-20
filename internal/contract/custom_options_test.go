package contract

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/pluginpb"
)

// No protoc is needed for the failure/canonicalization tests. This also proves
// extensions work without generated annotation types linked into the compiler.
func customFixture(t *testing.T) *descriptorpb.FileDescriptorSet {
	t.Helper()
	set := descriptorSet(nil, false, enumValue("ZERO", 0))
	set.File[0].EnumType[0].Options = nil
	set.File[0].Dependency = []string{"policy.proto"}
	policy := &descriptorpb.FileDescriptorProto{
		Name: proto.String("policy.proto"), Package: proto.String("policy"), Syntax: proto.String("proto2"),
		Dependency: []string{"google/protobuf/descriptor.proto"},
		MessageType: []*descriptorpb.DescriptorProto{{Name: proto.String("Rules"), Field: []*descriptorpb.FieldDescriptorProto{
			{Name: proto.String("minimum"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()},
			{Name: proto.String("mode"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum(), TypeName: proto.String(".policy.Rules.Mode"), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()},
		}, EnumType: []*descriptorpb.EnumDescriptorProto{{Name: proto.String("Mode"), Value: []*descriptorpb.EnumValueDescriptorProto{enumValue("DEFAULT", 0)}}}}},
		Extension: []*descriptorpb.FieldDescriptorProto{{Name: proto.String("rules"), Number: proto.Int32(50001),
			Extendee: proto.String(".google.protobuf.FieldOptions"), Type: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: proto.String(".policy.Rules"), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()}},
	}
	set.File = append(set.File, policy, protodesc.ToFileDescriptorProto(descriptorpb.File_google_protobuf_descriptor_proto))
	// Options arrive as unknown wire fields when the extension isn't linked in.
	set.File[0].MessageType[0].Field[0].Options = &descriptorpb.FieldOptions{}
	setRuleWire(set, []byte{8, 1})
	return set
}

func setRuleWire(set *descriptorpb.FileDescriptorSet, payload []byte) {
	wire := protowire.AppendTag(nil, 50001, protowire.BytesType)
	wire = protowire.AppendBytes(wire, payload)
	set.File[0].MessageType[0].Field[0].Options.ProtoReflect().SetUnknown(wire)
}

func buildCustom(t *testing.T, set *descriptorpb.FileDescriptorSet) *Snapshot {
	t.Helper()
	s, err := Build(set, "test.S", "test", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCustomOptionsDeterminismAndDefinitionChanges(t *testing.T) {
	set := customFixture(t)
	first := buildCustom(t, set)
	path := filepath.Join(t.TempDir(), "lock.json")
	if err := Write(path, first); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path); err != nil {
		t.Fatal(err)
	}
	// Input file order and source locations/comments don't change semantics.
	reordered := proto.Clone(set).(*descriptorpb.FileDescriptorSet)
	reordered.File[1].SourceCodeInfo = &descriptorpb.SourceCodeInfo{Location: []*descriptorpb.SourceCodeInfo_Location{{Span: []int32{0, 0, 0}, LeadingComments: proto.String("comment")}}}
	reordered.File[0], reordered.File[2] = reordered.File[2], reordered.File[0]
	if buildCustom(t, reordered).Digest != first.Digest {
		t.Fatal("descriptor file order or source info changed fingerprint")
	}
	// Equivalent wire encoding is normalized rather than fingerprinted blindly.
	setRuleWire(set, []byte{8, 0x81, 0})
	if buildCustom(t, set).Digest != first.Digest {
		t.Fatal("equivalent custom option encoding changed fingerprint")
	}
	// An interpreted dynamic extension follows the same path as unknown bytes.
	files, err := protodesc.NewFiles(set)
	if err != nil {
		t.Fatal(err)
	}
	decoded := &descriptorpb.FieldOptions{}
	wire, _ := proto.Marshal(set.File[0].MessageType[0].Field[0].Options)
	if err := (proto.UnmarshalOptions{Resolver: dynamicpb.NewTypes(files)}).Unmarshal(wire, decoded); err != nil {
		t.Fatal(err)
	}
	set.File[0].MessageType[0].Field[0].Options = decoded
	if buildCustom(t, set).Digest != first.Digest {
		t.Fatal("interpreted extension changed fingerprint")
	}
	set = customFixture(t)
	for _, change := range []struct {
		name   string
		mutate func(*descriptorpb.FileDescriptorSet)
	}{
		{"value", func(s *descriptorpb.FileDescriptorSet) { setRuleWire(s, []byte{8, 2}) }},
		{"definition", func(s *descriptorpb.FileDescriptorSet) {
			s.File[1].MessageType[0].Field[0].DefaultValue = proto.String("2")
		}},
		{"removal", func(s *descriptorpb.FileDescriptorSet) { s.File[0].MessageType[0].Field[0].Options = nil }},
	} {
		t.Run(change.name, func(t *testing.T) {
			modified := proto.Clone(set).(*descriptorpb.FileDescriptorSet)
			change.mutate(modified)
			current := buildCustom(t, modified)
			if current.Digest == first.Digest {
				t.Fatal("custom option change did not affect digest")
			}
			for _, pair := range [][2]*Snapshot{{first, current}, {current, first}} {
				if bump, _ := Compare(pair[0], pair[1]); bump != Major {
					t.Fatalf("bump = %s, want major", bump)
				}
			}
		})
	}
}

func TestCustomOptionsRejectUnresolvedAndMalformedInputs(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*descriptorpb.FileDescriptorSet)
	}{
		{"missing import", func(s *descriptorpb.FileDescriptorSet) { s.File = s.File[:1] }},
		{"unknown nested field", func(s *descriptorpb.FileDescriptorSet) {
			setRuleWire(s, protowire.AppendVarint(protowire.AppendTag(nil, 99, protowire.VarintType), 1))
		}},
		{"unknown enum value", func(s *descriptorpb.FileDescriptorSet) { setRuleWire(s, []byte{16, 99}) }},
		{"malformed payload", func(s *descriptorpb.FileDescriptorSet) { setRuleWire(s, []byte{8}) }},
		{"wrong wire type", func(s *descriptorpb.FileDescriptorSet) {
			s.File[0].MessageType[0].Field[0].Options.ProtoReflect().SetUnknown(protowire.AppendVarint(protowire.AppendTag(nil, 50001, protowire.VarintType), 1))
		}},
		{"uninterpreted", func(s *descriptorpb.FileDescriptorSet) {
			s.File[0].MessageType[0].Field[0].Options.UninterpretedOption = []*descriptorpb.UninterpretedOption{{Name: []*descriptorpb.UninterpretedOption_NamePart{{NamePart: proto.String("missing"), IsExtension: proto.Bool(true)}}, StringValue: []byte("x")}}
		}},
		{"edition definitions", func(s *descriptorpb.FileDescriptorSet) {
			s.File[1].Syntax = proto.String("editions")
			s.File[1].Edition = descriptorpb.Edition_EDITION_2023.Enum()
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := customFixture(t)
			tc.mutate(s)
			if _, err := Build(s, "test.S", "test", "1.0.0"); err == nil {
				t.Fatal("accepted unsupported custom option input")
			}
		})
	}
}

func TestCustomOptionsCollectNestedExtensionDefinitions(t *testing.T) {
	set := customFixture(t)
	set.File[1].MessageType[0].ExtensionRange = []*descriptorpb.DescriptorProto_ExtensionRange{{Start: proto.Int32(10), End: proto.Int32(20)}}
	set.File[0].Dependency = append(set.File[0].Dependency, "nested.proto")
	set.File = append(set.File, &descriptorpb.FileDescriptorProto{
		Name: proto.String("nested.proto"), Package: proto.String("nested"), Syntax: proto.String("proto2"), Dependency: []string{"policy.proto"},
		Extension: []*descriptorpb.FieldDescriptorProto{{Name: proto.String("rule"), Number: proto.Int32(10), Extendee: proto.String(".policy.Rules"), Type: descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()}},
	})
	setRuleWire(set, []byte{8, 1, 80, 2})
	s := buildCustom(t, set)
	path := filepath.Join(t.TempDir(), "lock.json")
	if err := Write(path, s); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path); err != nil {
		t.Fatal(err)
	}
	set.File[3].Extension[0].DefaultValue = proto.String("3")
	if bump, _ := Compare(s, buildCustom(t, set)); bump != Major {
		t.Fatalf("nested extension definition change = %s", bump)
	}
}

func TestCustomOptionsOnNewFieldsRequireMajor(t *testing.T) {
	set := customFixture(t)
	before := buildCustom(t, set)
	field := proto.Clone(set.File[0].MessageType[0].Field[0]).(*descriptorpb.FieldDescriptorProto)
	field.Name = proto.String("additional")
	field.Number = proto.Int32(2)
	set.File[0].MessageType[0].Field = append(set.File[0].MessageType[0].Field, field)
	after := buildCustom(t, set)
	if bump, _ := Compare(before, after); bump != Major {
		t.Fatalf("custom annotation on new field = %s, want major", bump)
	}
	// The identical structural addition without an annotation remains minor.
	field.Options = nil
	if bump, _ := Compare(before, buildCustom(t, set)); bump != Minor {
		t.Fatalf("unannotated new field = %s, want minor", bump)
	}
}

func TestCustomOptionsCheckImportedDefinitionMetadata(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*descriptorpb.FileDescriptorSet)
	}{
		{"unknown definition field", func(s *descriptorpb.FileDescriptorSet) {
			s.File[1].MessageType[0].Field[0].ProtoReflect().SetUnknown([]byte{0xA0, 0x06, 1})
		}},
		{"unknown definition option", func(s *descriptorpb.FileDescriptorSet) {
			f := s.File[1].MessageType[0].Field[0]
			f.Options = &descriptorpb.FieldOptions{}
			f.Options.ProtoReflect().SetUnknown(protowire.AppendVarint(protowire.AppendTag(nil, 60000, protowire.VarintType), 1))
		}},
		{"uninterpreted definition option", func(s *descriptorpb.FileDescriptorSet) {
			s.File[1].MessageType[0].Field[0].Options = &descriptorpb.FieldOptions{UninterpretedOption: []*descriptorpb.UninterpretedOption{{Name: []*descriptorpb.UninterpretedOption_NamePart{{NamePart: proto.String("missing"), IsExtension: proto.Bool(true)}}, PositiveIntValue: proto.Uint64(1)}}}
		}},
		{"duplicate definition file", func(s *descriptorpb.FileDescriptorSet) {
			s.File = append(s.File, proto.Clone(s.File[1]).(*descriptorpb.FileDescriptorProto))
		}},
		{"duplicate extension number", func(s *descriptorpb.FileDescriptorSet) {
			ext := proto.Clone(s.File[1].Extension[0]).(*descriptorpb.FieldDescriptorProto)
			ext.Name = proto.String("conflicting")
			s.File[1].Extension = append(s.File[1].Extension, ext)
		}},
		{"duplicate nested extension number", func(s *descriptorpb.FileDescriptorSet) {
			ext := proto.Clone(s.File[1].Extension[0]).(*descriptorpb.FieldDescriptorProto)
			ext.Name = proto.String("conflicting")
			s.File[1].MessageType[0].NestedType = []*descriptorpb.DescriptorProto{{Name: proto.String("Nested"), Extension: []*descriptorpb.FieldDescriptorProto{ext}}}
		}},
		{"duplicate imported extension number", func(s *descriptorpb.FileDescriptorSet) {
			ext := proto.Clone(s.File[1].Extension[0]).(*descriptorpb.FieldDescriptorProto)
			ext.Name = proto.String("conflicting")
			s.File = append(s.File, &descriptorpb.FileDescriptorProto{
				Name: proto.String("conflicting.proto"), Package: proto.String("conflicting"), Syntax: proto.String("proto2"), Dependency: []string{"policy.proto", "google/protobuf/descriptor.proto"}, Extension: []*descriptorpb.FieldDescriptorProto{ext},
			})
			s.File[0].Dependency = append(s.File[0].Dependency, "conflicting.proto")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			set := customFixture(t)
			tc.mutate(set)
			if _, err := Build(set, "test.S", "test", "1.0.0"); err == nil {
				t.Fatal("accepted unsupported imported definition semantics")
			}
		})
	}
}

func TestCustomOptionsFingerprintTransitiveDefinitions(t *testing.T) {
	set := customFixture(t)
	set.File[1].Dependency = append(set.File[1].Dependency, "limits.proto")
	set.File = append(set.File, &descriptorpb.FileDescriptorProto{
		Name: proto.String("limits.proto"), Package: proto.String("limits"), Syntax: proto.String("proto2"),
		MessageType: []*descriptorpb.DescriptorProto{{Name: proto.String("Limit"), Field: []*descriptorpb.FieldDescriptorProto{{
			Name: proto.String("value"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), DefaultValue: proto.String("1"),
		}}}},
	})
	set.File[1].MessageType[0].Field = append(set.File[1].MessageType[0].Field, &descriptorpb.FieldDescriptorProto{
		Name: proto.String("limit"), Number: proto.Int32(3), Type: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: proto.String(".limits.Limit"), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
	})
	before := buildCustom(t, set)
	set.File[3].MessageType[0].Field[0].DefaultValue = proto.String("2")
	after := buildCustom(t, set)
	if before.Digest == after.Digest {
		t.Fatal("transitive definition change did not affect digest")
	}
	if bump, _ := Compare(before, after); bump != Major {
		t.Fatalf("transitive definition change = %s, want major", bump)
	}
}

func TestCustomOptionLocksAreValidatedBeforeGeneration(t *testing.T) {
	base := buildCustom(t, customFixture(t))
	for _, tc := range []struct {
		name   string
		mutate func(*Snapshot)
	}{
		{"missing definitions", func(s *Snapshot) {
			for i, f := range s.Features {
				if f.Name == customDefinitions {
					s.Features = append(s.Features[:i], s.Features[i+1:]...)
					break
				}
			}
		}},
		{"ambiguous definitions", func(s *Snapshot) {
			for i, f := range s.Features {
				if f.Name != customDefinitions {
					continue
				}
				data, err := hex.DecodeString(f.Value)
				if err != nil {
					t.Fatal(err)
				}
				set := &descriptorpb.FileDescriptorSet{}
				if err := proto.Unmarshal(data, set); err != nil {
					t.Fatal(err)
				}
				for _, file := range set.File {
					if file.GetName() != "policy.proto" {
						continue
					}
					ext := proto.Clone(file.Extension[0]).(*descriptorpb.FieldDescriptorProto)
					ext.Name = proto.String("conflicting")
					file.Extension = append(file.Extension, ext)
				}
				data, err = (proto.MarshalOptions{Deterministic: true}).Marshal(set)
				if err != nil {
					t.Fatal(err)
				}
				s.Features[i].Value = hex.EncodeToString(data)
			}
		}},
		{"invalid owner", func(s *Snapshot) {
			for i, f := range s.Features {
				if strings.HasPrefix(f.Name, "custom-field-options:") {
					s.Features[i].Name = "custom-field-options:missing.field"
				}
			}
		}},
		{"unknown option", func(s *Snapshot) {
			for i, f := range s.Features {
				if strings.HasPrefix(f.Name, "custom-field-options:") {
					s.Features[i].Value = hex.EncodeToString(protowire.AppendVarint(protowire.AppendTag(nil, 50009, protowire.VarintType), 1))
				}
			}
		}},
		{"empty values", func(s *Snapshot) {
			for i, f := range s.Features {
				if strings.HasPrefix(f.Name, "custom-field-options:") {
					s.Features[i].Value = ""
				}
			}
		}},
		{"noncanonical definitions", func(s *Snapshot) {
			for i, f := range s.Features {
				if f.Name == customDefinitions {
					s.Features[i].Value = strings.ToUpper(f.Value)
				}
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, _ := json.Marshal(base)
			var s Snapshot
			_ = json.Unmarshal(data, &s)
			tc.mutate(&s)
			sort.Slice(s.Features, func(i, j int) bool { return s.Features[i].Name < s.Features[j].Name })
			s.Digest, _ = calculateDigest(&s)
			for _, lang := range []string{"go", "python", "java", "dotnet"} {
				if _, err := Generate(&s, lang, ""); err == nil {
					t.Fatalf("%s accepted corrupt custom option lock", lang)
				}
			}
			path := filepath.Join(t.TempDir(), "lock.json")
			data, _ = json.Marshal(&s)
			_ = os.WriteFile(path, data, 0600)
			if _, err := Read(path); err == nil {
				t.Fatal("Read accepted corrupt option lock with recomputed digest")
			}
		})
	}
}

func TestProtocCustomOptionFixtureAndPlugin(t *testing.T) {
	protoc, err := exec.LookPath("protoc")
	if err != nil {
		t.Skip("protoc is required for custom-option integration")
	}
	root := filepath.Join("testdata", "options")
	set, err := CompileDescriptorSet(protoc, root, "service.proto")
	if err != nil {
		t.Fatal(err)
	}
	s, err := Build(set, "annotated.API", "annotated", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"go", "python", "java", "dotnet"} {
		if _, err := Generate(s, lang, ""); err != nil {
			t.Fatalf("generate %s from checked custom options: %v", lang, err)
		}
	}
	for _, kind := range []string{"file", "service", "method", "message", "field", "oneof", "enum", "enum-value"} {
		found := false
		for _, f := range s.Features {
			found = found || strings.HasPrefix(f.Name, "custom-"+kind+"-options:")
		}
		if !found {
			t.Fatalf("missing custom %s options", kind)
		}
	}
	tmp := t.TempDir()
	if err := Write(filepath.Join(tmp, "lock.json"), s); err != nil {
		t.Fatal(err)
	}
	bindings := `{"bindings":[{"service":"annotated.API","lock":"lock.json","output":"contract.py","service_import":"service_pb2_grpc","service_export":"APIStub"}]}`
	bindingPath := filepath.Join(tmp, "bindings.json")
	if err := os.WriteFile(bindingPath, []byte(bindings), 0600); err != nil {
		t.Fatal(err)
	}
	req := &pluginpb.CodeGeneratorRequest{ProtoFile: set.File, FileToGenerate: []string{"service.proto"}}
	if _, err := GeneratePluginResponse(req, "python", bindingPath); err != nil {
		t.Fatal(err)
	}
	if _, err := TypeScriptBinding(s, set, "./service_pb.js", "API"); err != nil {
		t.Fatal(err)
	}
	graph, err := RuntimeGraph(set, "annotated.API")
	if err != nil {
		t.Fatal(err)
	}
	// HTTP/rule changes are source semantics, outside Protobuf-ES's runtime graph.
	changed := proto.Clone(set).(*descriptorpb.FileDescriptorSet)
	for _, file := range changed.File {
		if file.GetName() == "service.proto" {
			file.Service[0].Method[0].Options = nil
		}
	}
	changedGraph, err := RuntimeGraph(changed, "annotated.API")
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(graph)
	b, _ := json.Marshal(changedGraph)
	if string(a) != string(b) {
		t.Fatal("annotation unexpectedly changed runtime structural graph")
	}
	if _, err := TypeScriptBinding(s, changed, "./service_pb.js", "API"); err == nil {
		t.Fatal("TypeScript accepted stale HTTP annotation")
	}
	// Simulate modern protoc's source-retention split and require the full input.
	for _, file := range set.File {
		if file.GetName() == "policy.proto" {
			for _, extension := range file.Extension {
				if extension.GetName() == "http" {
					extension.Options = &descriptorpb.FieldOptions{Retention: descriptorpb.FieldOptions_RETENTION_SOURCE.Enum()}
				}
			}
		}
	}
	s, err = Build(set, "annotated.API", "annotated", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if err := Write(filepath.Join(tmp, "lock.json"), s); err != nil {
		t.Fatal(err)
	}
	changed = proto.Clone(set).(*descriptorpb.FileDescriptorSet)
	for _, file := range changed.File {
		if file.GetName() == "service.proto" {
			file.Service[0].Method[0].Options = nil
		}
	}
	var source *descriptorpb.FileDescriptorProto
	for _, file := range set.File {
		if file.GetName() == "service.proto" {
			source = file
		}
	}
	req.ProtoFile = changed.File
	req.SourceFileDescriptors = []*descriptorpb.FileDescriptorProto{source}
	if _, err := GeneratePluginResponse(req, "python", bindingPath); err != nil {
		t.Fatalf("source-retention descriptor lost: %v", err)
	}
	inconsistent := proto.Clone(req).(*pluginpb.CodeGeneratorRequest)
	inconsistent.SourceFileDescriptors[0].MessageType[0].Field[0].Name = proto.String("renamed")
	if _, err := GeneratePluginResponse(inconsistent, "python", bindingPath); err == nil {
		t.Fatal("plugin let full source descriptors conceal a changed protobuf field")
	}
	inconsistent = proto.Clone(req).(*pluginpb.CodeGeneratorRequest)
	inconsistent.SourceFileDescriptors[0].Service[0].Options.Deprecated = proto.Bool(true)
	if _, err := GeneratePluginResponse(inconsistent, "python", bindingPath); err == nil {
		t.Fatal("plugin let full source descriptors conceal a changed runtime option")
	}
	req.SourceFileDescriptors = nil
	if _, err := GeneratePluginResponse(req, "python", bindingPath); err == nil {
		t.Fatal("plugin accepted stale HTTP annotation")
	}
	req.SourceFileDescriptors = []*descriptorpb.FileDescriptorProto{source, source}
	if _, err := GeneratePluginResponse(req, "python", bindingPath); err == nil {
		t.Fatal("plugin accepted duplicate full source descriptors")
	}
}

func TestProtocRetainsSourceOnlyOptions(t *testing.T) {
	protoc, err := exec.LookPath("protoc")
	if err != nil {
		t.Skip("protoc is required")
	}
	help, err := exec.Command(protoc, "--help").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(help), "--retain_options") {
		t.Skip("protoc predates source option retention")
	}
	tmp := t.TempDir()
	for _, name := range []string{"policy.proto", "service.proto"} {
		data, err := os.ReadFile(filepath.Join("testdata", "options", name))
		if err != nil {
			t.Fatal(err)
		}
		if name == "policy.proto" {
			data = []byte(strings.Replace(string(data), "optional Rules rules = 50001;", "optional Rules rules = 50001 [retention = RETENTION_SOURCE];", 1))
		}
		if err := os.WriteFile(filepath.Join(tmp, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	set, err := CompileDescriptorSet(protoc, tmp, "service.proto")
	if err != nil {
		t.Fatal(err)
	}
	s, err := Build(set, "annotated.API", "annotated", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range s.Features {
		if f.Name == "custom-field-options:annotated.Request.name" {
			return
		}
	}
	t.Fatal("CLI discarded source-retention validation rules")
}
