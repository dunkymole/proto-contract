package contract

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestNextVersion(t *testing.T) {
	for _, tc := range []struct {
		in   string
		bump Bump
		want string
	}{{"1.2.3", None, "1.2.3"}, {"1.2.3", Minor, "1.3.0"}, {"1.2.3", Major, "2.0.0"}} {
		got, err := NextVersion(tc.in, tc.bump)
		if err != nil || got != tc.want {
			t.Fatalf("NextVersion(%q,%s)=%q,%v", tc.in, tc.bump, got, err)
		}
	}
	for _, invalid := range []string{"01.2.3", "1.2", "2147483648.0.0", "1.0.2147483648"} {
		if _, err := NextVersion(invalid, None); err == nil {
			t.Fatalf("accepted invalid version %q", invalid)
		}
	}
}

func TestCompareFieldAddition(t *testing.T) {
	a := &Snapshot{Service: Service{Name: "x", Methods: []Method{{Name: "Call", Input: "Req", Output: "Res"}}}, Messages: []Message{{Name: "Req", Fields: []Field{{Number: 1, Name: "value"}}}, {Name: "Res"}}}
	b := &Snapshot{Service: a.Service, Messages: []Message{{Name: "Req", Fields: []Field{{Number: 1, Name: "value"}, {Number: 2, Name: "extra"}}}, {Name: "Res"}}}
	bump, _ := Compare(a, b)
	if bump != Minor {
		t.Fatalf("got %s", bump)
	}
	b.Messages[0].Fields[0].Type = "TYPE_INT32"
	bump, _ = Compare(a, b)
	if bump != Major {
		t.Fatalf("got %s", bump)
	}
}

func descriptorSet(defaultValue *string, required bool, enumValues ...*descriptorpb.EnumValueDescriptorProto) *descriptorpb.FileDescriptorSet {
	label := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	if required {
		label = descriptorpb.FieldDescriptorProto_LABEL_REQUIRED
	}
	field := &descriptorpb.FieldDescriptorProto{Name: proto.String("status"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum(), TypeName: proto.String(".test.Status"), Label: label.Enum()}
	if defaultValue != nil {
		field.DefaultValue = proto.String(*defaultValue)
	}
	request := &descriptorpb.DescriptorProto{Name: proto.String("Request"), Field: []*descriptorpb.FieldDescriptorProto{field}}
	response := &descriptorpb.DescriptorProto{Name: proto.String("Response")}
	service := &descriptorpb.ServiceDescriptorProto{Name: proto.String("S"), Method: []*descriptorpb.MethodDescriptorProto{{Name: proto.String("Call"), InputType: proto.String(".test.Request"), OutputType: proto.String(".test.Response")}}}
	enum := &descriptorpb.EnumDescriptorProto{Name: proto.String("Status"), Value: enumValues}
	return &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{{Name: proto.String("test.proto"), Package: proto.String("test"), Syntax: proto.String("proto2"), Service: []*descriptorpb.ServiceDescriptorProto{service}, MessageType: []*descriptorpb.DescriptorProto{request, response}, EnumType: []*descriptorpb.EnumDescriptorProto{enum}}}}
}

func enumValue(name string, number int32) *descriptorpb.EnumValueDescriptorProto {
	return &descriptorpb.EnumValueDescriptorProto{Name: proto.String(name), Number: proto.Int32(number)}
}

func TestBuildCapturesDefaultsAndAliasesDeterministically(t *testing.T) {
	defaultOne, defaultTwo := "1", "2"
	values := []*descriptorpb.EnumValueDescriptorProto{enumValue("ZERO", 0), enumValue("ALIAS", 0)}
	first, err := Build(descriptorSet(&defaultOne, false, values...), "test.S", "test", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build(descriptorSet(&defaultOne, false, []*descriptorpb.EnumValueDescriptorProto{values[1], values[0]}...), "test.S", "test", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest == second.Digest {
		t.Fatal("changing the canonical alias did not change the digest")
	}
	if bump, _ := Compare(first, second); bump != Major {
		t.Fatalf("canonical alias change bump = %s, want major", bump)
	}
	triple := []*descriptorpb.EnumValueDescriptorProto{enumValue("ZERO", 0), enumValue("A", 1), enumValue("B", 1), enumValue("C", 1)}
	ordered, err := Build(descriptorSet(&defaultOne, false, triple...), "test.S", "test", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	reorderedValues := []*descriptorpb.EnumValueDescriptorProto{triple[0], triple[1], triple[3], triple[2]}
	reordered, err := Build(descriptorSet(&defaultOne, false, reorderedValues...), "test.S", "test", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if ordered.Digest == reordered.Digest {
		t.Fatal("reordering noncanonical aliases did not change the digest")
	}
	if bump, _ := Compare(ordered, reordered); bump != Major {
		t.Fatalf("noncanonical alias reorder bump = %s, want major", bump)
	}
	oldAlias, err := Build(descriptorSet(&defaultOne, false, enumValue("ZERO", 0), enumValue("A", 1)), "test.S", "test", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	newAlias, err := Build(descriptorSet(&defaultOne, false, enumValue("ZERO", 0), enumValue("A", 1), enumValue("B", 1)), "test.S", "test", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if bump, _ := Compare(oldAlias, newAlias); bump != Major {
		t.Fatalf("new alias for an existing number bump = %s, want major", bump)
	}
	if got := first.Messages[0].Fields[0]; !got.HasDefault || got.DefaultValue != defaultOne {
		t.Fatalf("explicit default was not captured: %+v", got)
	}
	changed, err := Build(descriptorSet(&defaultTwo, false, values...), "test.S", "test", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest == changed.Digest {
		t.Fatal("default change did not affect the digest")
	}
	bump, _ := Compare(first, changed)
	if bump != Major {
		t.Fatalf("default change bump = %s, want major", bump)
	}
	emptyDefault := ""
	empty, err := Build(descriptorSet(&emptyDefault, false, values...), "test.S", "test", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if !empty.Messages[0].Fields[0].HasDefault || empty.Messages[0].Fields[0].DefaultValue != "" || empty.Digest == first.Digest {
		t.Fatal("explicit empty default was lost or treated as absent")
	}

	removedAlias, err := Build(descriptorSet(&defaultOne, false, values[:1]...), "test.S", "test", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	bump, _ = Compare(first, removedAlias)
	if bump != Major {
		t.Fatalf("alias removal bump = %s, want major", bump)
	}
}

func TestRequiredFieldAndEnumEvolutionIsMajor(t *testing.T) {
	base := &Snapshot{Service: Service{Name: "S"}, Messages: []Message{{Name: "Req"}, {Name: "Res"}}}
	for _, location := range []string{"request", "response"} {
		messages := []Message{{Name: "Req"}, {Name: "Res"}}
		index := 0
		if location == "response" {
			index = 1
		}
		messages[index].Fields = []Field{{Number: 1, Name: "required", Type: "TYPE_STRING", Label: "LABEL_REQUIRED"}}
		cur := &Snapshot{Service: base.Service, Messages: messages}
		bump, _ := Compare(base, cur)
		if bump != Major {
			t.Fatalf("required %s field addition = %s, want major", location, bump)
		}
	}
	old := &Snapshot{Service: Service{Name: "S"}, Messages: []Message{{Name: "Req", Fields: []Field{{Number: 1, Name: "state", Type: "TYPE_ENUM", TypeName: "State", Label: "LABEL_REQUIRED"}}}}, Enums: []Enum{{Name: "State", Values: []EnumValue{{Name: "READY", Number: 0}}}}}
	cur := &Snapshot{Service: old.Service, Messages: old.Messages, Enums: []Enum{{Name: "State", Values: []EnumValue{{Name: "READY", Number: 0}, {Name: "NEW", Number: 1}}}}}
	bump, _ := Compare(old, cur)
	if bump != Major {
		t.Fatalf("required enum addition = %s, want major", bump)
	}
}

func TestBuildRejectsUnsupportedDescriptorFeatures(t *testing.T) {
	set := descriptorSet(nil, false, enumValue("ZERO", 0))
	set.File[0].MessageType[0].Field[0].Options = &descriptorpb.FieldOptions{Deprecated: proto.Bool(true)}
	withOption, err := Build(set, "test.S", "test", "1.0.0")
	if err != nil {
		t.Fatalf("rejected fingerprintable field options: %v", err)
	}
	set.File[0].MessageType[0].Field[0].Options.Deprecated = proto.Bool(false)
	withoutOption, err := Build(set, "test.S", "test", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if withOption.Digest == withoutOption.Digest {
		t.Fatal("field option change did not affect digest")
	}
	if bump, _ := Compare(withOption, withoutOption); bump != Major {
		t.Fatalf("field option change bump = %s, want major", bump)
	}
	if bump, _ := Compare(withoutOption, withOption); bump != Major {
		t.Fatalf("new option on an existing field bump = %s, want major", bump)
	}
	set = descriptorSet(nil, false, enumValue("ZERO", 0))
	set.File[0].MessageType[0].Field[0].Options = &descriptorpb.FieldOptions{}
	set.File[0].MessageType[0].Field[0].Options.ProtoReflect().SetUnknown([]byte{0xa0, 0x06, 0x01})
	if _, err := Build(set, "test.S", "test", "1.0.0"); err == nil {
		t.Fatal("accepted an unclassified custom field option")
	}

	proto2 := descriptorSet(nil, false, enumValue("ZERO", 0))
	proto3 := descriptorSet(nil, false, enumValue("ZERO", 0))
	proto3.File[0].Syntax = proto.String("proto3")
	oldSyntax, err := Build(proto2, "test.S", "test", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	newSyntax, err := Build(proto3, "test.S", "test", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if oldSyntax.Digest == newSyntax.Digest {
		t.Fatal("syntax change did not affect digest")
	}
	if bump, _ := Compare(oldSyntax, newSyntax); bump != Major {
		t.Fatalf("syntax change bump = %s, want major", bump)
	}
	set = descriptorSet(nil, false, enumValue("ZERO", 0))
	set.File[0].MessageType[0].Field[0].Type = descriptorpb.FieldDescriptorProto_TYPE_GROUP.Enum()
	if _, err := Build(set, "test.S", "test", "1.0.0"); err == nil {
		t.Fatal("accepted unsupported group field")
	}
	set = descriptorSet(nil, false, enumValue("ZERO", 0))
	set.File[0].Syntax = proto.String("editions")
	set.File[0].Edition = descriptorpb.Edition_EDITION_2023.Enum()
	if _, err := Build(set, "test.S", "test", "1.0.0"); err == nil {
		t.Fatal("accepted editions descriptor")
	}
}

func TestMigrateFormat1RequiresMatchingStructureAndMajorVersion(t *testing.T) {
	defaultValue := "42"
	current, err := Build(descriptorSet(&defaultValue, false, enumValue("ZERO", 0)), "test.S", "test", "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	legacy := *current
	legacy.Format = 1
	legacy.Messages = append([]Message(nil), current.Messages...)
	for i := range legacy.Messages {
		legacy.Messages[i].Fields = append([]Field(nil), current.Messages[i].Fields...)
		legacy.Messages[i].MapEntry = false
		legacy.Messages[i].Syntax = ""
		for j := range legacy.Messages[i].Fields {
			legacy.Messages[i].Fields[j].HasDefault = false
			legacy.Messages[i].Fields[j].DefaultValue = ""
			legacy.Messages[i].Fields[j].JSONName = ""
		}
	}
	legacy.Enums = append([]Enum(nil), current.Enums...)
	for i := range legacy.Enums {
		legacy.Enums[i].Values = append([]EnumValue(nil), current.Enums[i].Values...)
		legacy.Enums[i].DefaultName = ""
		legacy.Enums[i].Syntax = ""
		for j := range legacy.Enums[i].Values {
			legacy.Enums[i].Values[j].AliasOrder = 0
		}
	}
	legacy.Features = nil
	migrated, err := MigrateFormat1(&legacy, current)
	if err != nil {
		t.Fatal(err)
	}
	if migrated.Format != 2 || migrated.Version != "2.0.0" || !migrated.Messages[0].Fields[0].HasDefault {
		t.Fatalf("bad migration: %+v", migrated)
	}
	if !current.Messages[0].Fields[0].HasDefault || current.Messages[0].Fields[0].DefaultValue != defaultValue {
		t.Fatal("migration mutated the current descriptor snapshot")
	}
	if current.Enums[0].DefaultName != "ZERO" || current.Enums[0].Values[0].AliasOrder != 0 {
		t.Fatal("migration mutated enum canonical-name metadata")
	}
	current.Messages[0].Fields[0].Name = "different"
	if _, err := MigrateFormat1(&legacy, current); err == nil {
		t.Fatal("migrated a structurally different descriptor")
	}
}
