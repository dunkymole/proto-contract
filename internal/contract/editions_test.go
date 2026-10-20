package contract

import (
	"encoding/hex"
	"sort"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

func editionFixture() *descriptorpb.FileDescriptorSet {
	s := descriptorSet(nil, false, enumValue("ZERO", 0))
	s.File[0].Syntax = proto.String("editions")
	s.File[0].Edition = descriptorpb.Edition_EDITION_2023.Enum()
	return s
}
func resolvedEdition(t *testing.T, s *Snapshot, owner string) *descriptorpb.FeatureSet {
	t.Helper()
	for _, f := range s.Features {
		if f.Name == "edition-features:"+owner {
			b, err := hex.DecodeString(f.Value)
			if err != nil {
				t.Fatal(err)
			}
			fs := new(descriptorpb.FeatureSet)
			if err := proto.Unmarshal(b, fs); err != nil {
				t.Fatal(err)
			}
			return fs
		}
	}
	t.Fatalf("missing resolved features for %s", owner)
	return nil
}
func TestEditionDefaultsInheritanceAndChanges(t *testing.T) {
	set := editionFixture()
	base, err := Build(set, "test.S", "test", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if got := resolvedEdition(t, base, "test.Request.status"); !proto.Equal(got, edition2023Defaults()) {
		t.Fatalf("defaults %v", got)
	}
	set.File[0].Options = &descriptorpb.FileOptions{Features: &descriptorpb.FeatureSet{EnumType: descriptorpb.FeatureSet_CLOSED.Enum(), JsonFormat: descriptorpb.FeatureSet_LEGACY_BEST_EFFORT.Enum()}}
	inherited, err := Build(set, "test.S", "test", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if resolvedEdition(t, inherited, "test.Status").GetEnumType() != descriptorpb.FeatureSet_CLOSED {
		t.Fatal("file enum setting not inherited")
	}
	if bump, _ := Compare(base, inherited); bump != Major {
		t.Fatalf("setting change: %s", bump)
	}
	if bump, _ := Compare(inherited, base); bump != Major {
		t.Fatalf("setting removal: %s", bump)
	}
	set.File[0].MessageType[0].Options = &descriptorpb.MessageOptions{Features: &descriptorpb.FeatureSet{JsonFormat: descriptorpb.FeatureSet_ALLOW.Enum()}}
	set.File[0].MessageType[0].Field[0].Options = &descriptorpb.FieldOptions{Features: &descriptorpb.FeatureSet{FieldPresence: descriptorpb.FeatureSet_LEGACY_REQUIRED.Enum()}}
	explicit, err := Build(set, "test.S", "test", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if resolvedEdition(t, explicit, "test.Request.status").GetJsonFormat() != descriptorpb.FeatureSet_ALLOW {
		t.Fatal("message JSON override not inherited")
	}
	set.File[0].EnumType[0].Value = append(set.File[0].EnumType[0].Value, enumValue("ONE", 1))
	added, err := Build(set, "test.S", "test", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if bump, _ := Compare(explicit, added); bump != Major {
		t.Fatalf("legacy required enum addition: %s", bump)
	}
}

func TestEditionUnsupportedInputsFailClosed(t *testing.T) {
	cases := map[string]func(*descriptorpb.FileDescriptorSet){
		"unknown edition": func(s *descriptorpb.FileDescriptorSet) { s.File[0].Edition = descriptorpb.Edition_EDITION_2024.Enum() },
		"unknown enum": func(s *descriptorpb.FileDescriptorSet) {
			s.File[0].Options = &descriptorpb.FileOptions{Features: &descriptorpb.FeatureSet{EnumType: descriptorpb.FeatureSet_EnumType(99).Enum()}}
		},
		"unknown zero": func(s *descriptorpb.FileDescriptorSet) {
			s.File[0].Options = &descriptorpb.FileOptions{Features: &descriptorpb.FeatureSet{EnumType: descriptorpb.FeatureSet_ENUM_TYPE_UNKNOWN.Enum()}}
		},
		"unknown bytes": func(s *descriptorpb.FileDescriptorSet) {
			f := new(descriptorpb.FeatureSet)
			f.ProtoReflect().SetUnknown([]byte{0x80, 0x01, 0x01})
			s.File[0].Options = &descriptorpb.FileOptions{Features: f}
		},
		"message scope": func(s *descriptorpb.FileDescriptorSet) {
			s.File[0].MessageType[0].Options = &descriptorpb.MessageOptions{Features: &descriptorpb.FeatureSet{Utf8Validation: descriptorpb.FeatureSet_VERIFY.Enum()}}
		},
		"service scope": func(s *descriptorpb.FileDescriptorSet) {
			s.File[0].Service[0].Options = &descriptorpb.ServiceOptions{Features: &descriptorpb.FeatureSet{JsonFormat: descriptorpb.FeatureSet_ALLOW.Enum()}}
		},
		"wrong field kind": func(s *descriptorpb.FileDescriptorSet) {
			s.File[0].MessageType[0].Field[0].Options = &descriptorpb.FieldOptions{Features: &descriptorpb.FeatureSet{MessageEncoding: descriptorpb.FeatureSet_DELIMITED.Enum()}}
		},
		"legacy syntax features": func(s *descriptorpb.FileDescriptorSet) {
			s.File[0].Syntax = proto.String("proto3")
			s.File[0].Edition = nil
			s.File[0].Options = &descriptorpb.FileOptions{Features: &descriptorpb.FeatureSet{EnumType: descriptorpb.FeatureSet_OPEN.Enum()}}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s := editionFixture()
			mutate(s)
			if _, err := Build(s, "test.S", "test", "1.0.0"); err == nil {
				t.Fatal("accepted unsupported editions input")
			}
		})
	}
}

func TestEditionLockTamperingFailsClosed(t *testing.T) {
	base, err := Build(editionFixture(), "test.S", "test", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"missing", "unknown", "mismatch"} {
		t.Run(mode, func(t *testing.T) {
			s := *base
			s.Features = append([]Feature(nil), base.Features...)
			for i, f := range s.Features {
				if f.Name != "edition-features:test.Request.status" {
					continue
				}
				if mode == "missing" {
					s.Features = append(s.Features[:i], s.Features[i+1:]...)
				} else {
					v := edition2023Defaults()
					if mode == "unknown" {
						v.EnumType = descriptorpb.FeatureSet_ENUM_TYPE_UNKNOWN.Enum()
					} else {
						v.FieldPresence = descriptorpb.FeatureSet_IMPLICIT.Enum()
						raw, _ := proto.Marshal(&descriptorpb.FieldOptions{Features: &descriptorpb.FeatureSet{FieldPresence: descriptorpb.FeatureSet_EXPLICIT.Enum()}})
						s.Features = append(s.Features, Feature{Name: "field-options:test.Request.status", Value: hex.EncodeToString(raw)})
					}
					b, _ := proto.Marshal(v)
					s.Features[i].Value = hex.EncodeToString(b)
				}
				break
			}
			sort.Slice(s.Features, func(i, j int) bool { return s.Features[i].Name < s.Features[j].Name })
			s.Digest, _ = calculateDigest(&s)
			if err := Validate(&s); err == nil {
				t.Fatal("accepted tampered lock")
			}
		})
	}
}

func TestEditionCanonicalDefaultsAndFieldAddition(t *testing.T) {
	set := editionFixture()
	base, err := Build(set, "test.S", "test", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	set.File[0].Options = &descriptorpb.FileOptions{Features: edition2023Defaults()}
	same, err := Build(set, "test.S", "test", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if base.Digest != same.Digest {
		t.Fatal("redundant explicit defaults changed digest")
	}
	set.File[0].MessageType[0].Field = append(set.File[0].MessageType[0].Field, &descriptorpb.FieldDescriptorProto{Name: proto.String("extra"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()})
	added, err := Build(set, "test.S", "test", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if bump, _ := Compare(base, added); bump != Minor {
		t.Fatalf("optional field addition=%s", bump)
	}
	set.File[0].MessageType[0].Field[1].Options = &descriptorpb.FieldOptions{Features: &descriptorpb.FeatureSet{FieldPresence: descriptorpb.FeatureSet_LEGACY_REQUIRED.Enum()}}
	required, err := Build(set, "test.S", "test", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if bump, _ := Compare(base, required); bump != Major {
		t.Fatalf("required field addition=%s", bump)
	}
}

func TestEditionImportedNestedInheritance(t *testing.T) {
	set := editionFixture()
	file := set.File[0]
	request := file.MessageType[0]
	request.Name = proto.String("Inner")
	file.MessageType[0] = &descriptorpb.DescriptorProto{Name: proto.String("Outer"), Options: &descriptorpb.MessageOptions{Features: &descriptorpb.FeatureSet{JsonFormat: descriptorpb.FeatureSet_LEGACY_BEST_EFFORT.Enum()}}, NestedType: []*descriptorpb.DescriptorProto{request}}
	service := file.Service[0]
	service.Method[0].InputType = proto.String(".test.Outer.Inner")
	file.Service = nil
	set.File = append(set.File, &descriptorpb.FileDescriptorProto{Name: proto.String("service.proto"), Package: proto.String("test"), Syntax: proto.String("proto3"), Dependency: []string{"test.proto"}, Service: []*descriptorpb.ServiceDescriptorProto{service}})
	built, err := Build(set, "test.S", "test", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if resolvedEdition(t, built, "test.Outer.Inner.status").GetJsonFormat() != descriptorpb.FeatureSet_LEGACY_BEST_EFFORT {
		t.Fatal("unreachable lexical ancestor JSON policy lost")
	}
	file.MessageType[0].Options.Features.JsonFormat = descriptorpb.FeatureSet_ALLOW.Enum()
	changed, err := Build(set, "test.S", "test", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if bump, _ := Compare(built, changed); bump != Major {
		t.Fatalf("imported inherited change=%s", bump)
	}
}

func TestEditionLockRejectsInvalidResolvedPolicies(t *testing.T) {
	base, err := Build(editionFixture(), "test.S", "test", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"packed", "nonfield required", "field enum override"} {
		t.Run(mode, func(t *testing.T) {
			s := *base
			s.Features = append([]Feature(nil), base.Features...)
			switch mode {
			case "packed":
				b, _ := proto.Marshal(&descriptorpb.FieldOptions{Packed: proto.Bool(true)})
				s.Features = append(s.Features, Feature{Name: "field-options:test.Request.status", Value: hex.EncodeToString(b)})
			case "nonfield required", "field enum override":
				for i, f := range s.Features {
					target := "edition-features:test.Request"
					if mode == "field enum override" {
						target += ".status"
					}
					if f.Name != target {
						continue
					}
					v := edition2023Defaults()
					if mode == "nonfield required" {
						v.FieldPresence = descriptorpb.FeatureSet_LEGACY_REQUIRED.Enum()
					} else {
						v.EnumType = descriptorpb.FeatureSet_CLOSED.Enum()
					}
					b, _ := proto.Marshal(v)
					s.Features[i].Value = hex.EncodeToString(b)
				}
			}
			sort.Slice(s.Features, func(i, j int) bool { return s.Features[i].Name < s.Features[j].Name })
			s.Digest, _ = calculateDigest(&s)
			if err := Validate(&s); err == nil {
				t.Fatal("accepted invalid editions lock")
			}
		})
	}
}

func TestEditionImplicitImportedEnumRules(t *testing.T) {
	for _, syntax := range []string{"proto2", "proto3"} {
		t.Run(syntax, func(t *testing.T) {
			set := editionFixture()
			file := set.File[0]
			file.Options = &descriptorpb.FileOptions{Features: &descriptorpb.FeatureSet{FieldPresence: descriptorpb.FeatureSet_IMPLICIT.Enum()}}
			enumeration := file.EnumType
			file.EnumType = nil
			file.Dependency = []string{"enum.proto"}
			set.File = append(set.File, &descriptorpb.FileDescriptorProto{Name: proto.String("enum.proto"), Package: proto.String("test"), Syntax: proto.String(syntax), EnumType: enumeration})
			_, err := Build(set, "test.S", "test", "1.0.0")
			if syntax == "proto2" && err == nil {
				t.Fatal("accepted implicit closed imported enum")
			}
			if syntax == "proto3" && err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestEditionLockRejectsNonApplicableFeatureOverrides(t *testing.T) {
	set := editionFixture()
	set.File[0].MessageType[0].Field[0].Type = descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum()
	set.File[0].MessageType[0].Field[0].TypeName = nil
	base, err := Build(set, "test.S", "test", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*descriptorpb.FeatureSet){"UTF8": func(f *descriptorpb.FeatureSet) { f.Utf8Validation = descriptorpb.FeatureSet_NONE.Enum() }, "packing": func(f *descriptorpb.FeatureSet) { f.RepeatedFieldEncoding = descriptorpb.FeatureSet_EXPANDED.Enum() }, "message encoding": func(f *descriptorpb.FeatureSet) { f.MessageEncoding = descriptorpb.FeatureSet_DELIMITED.Enum() }}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s := *base
			s.Features = append([]Feature(nil), base.Features...)
			for i, f := range s.Features {
				if f.Name != "edition-features:test.Request.status" {
					continue
				}
				v := edition2023Defaults()
				mutate(v)
				b, _ := proto.Marshal(v)
				s.Features[i].Value = hex.EncodeToString(b)
			}
			s.Digest, _ = calculateDigest(&s)
			if err := Validate(&s); err == nil {
				t.Fatal("accepted inapplicable override")
			}
		})
	}
}
