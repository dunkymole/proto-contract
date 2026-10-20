package contract

import (
	"encoding/hex"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// Edition 2023 defaults are pinned rather than inferred from the installed protoc.
func edition2023Defaults() *descriptorpb.FeatureSet {
	return &descriptorpb.FeatureSet{FieldPresence: descriptorpb.FeatureSet_EXPLICIT.Enum(), EnumType: descriptorpb.FeatureSet_OPEN.Enum(), RepeatedFieldEncoding: descriptorpb.FeatureSet_PACKED.Enum(), Utf8Validation: descriptorpb.FeatureSet_VERIFY.Enum(), MessageEncoding: descriptorpb.FeatureSet_LENGTH_PREFIXED.Enum(), JsonFormat: descriptorpb.FeatureSet_ALLOW.Enum()}
}

func optionFeatures(options proto.Message) *descriptorpb.FeatureSet {
	if options == nil || !options.ProtoReflect().IsValid() {
		return nil
	}
	m := options.ProtoReflect()
	f := m.Descriptor().Fields().ByName("features")
	if f == nil || !m.Has(f) {
		return nil
	}
	return m.Get(f).Message().Interface().(*descriptorpb.FeatureSet)
}

func validateCoreFeatures(features *descriptorpb.FeatureSet, scope string, complete bool) error {
	if features == nil {
		if complete {
			return fmt.Errorf("missing resolved edition features")
		}
		return nil
	}
	m := features.ProtoReflect()
	if len(m.GetUnknown()) != 0 {
		return fmt.Errorf("unsupported unknown edition features")
	}
	count := 0
	var failure error
	m.Range(func(f protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		count++
		if f.IsExtension() || f.Number() < 1 || f.Number() > 6 || v.Enum() == 0 || f.Enum().Values().ByNumber(v.Enum()) == nil {
			failure = fmt.Errorf("unsupported edition feature %s=%v", f.FullName(), v)
			return false
		}
		if !complete && scope != "file" {
			allowed := scope == "field" && (f.Number() == 1 || f.Number() == 3 || f.Number() == 4 || f.Number() == 5) || scope == "enum" && (f.Number() == 2 || f.Number() == 6) || scope == "message" && f.Number() == 6
			if !allowed {
				failure = fmt.Errorf("edition feature %s is unsupported at %s scope", f.Name(), scope)
				return false
			}
		}
		return true
	})
	if failure != nil {
		return failure
	}
	if complete && count != 6 {
		return fmt.Errorf("resolved edition features must contain all six Edition 2023 core values")
	}
	return nil
}

// addEditionFeatures captures defaults and lexical inheritance for reachable
// declarations, including ancestors that are not themselves reachable.
func addEditionFeatures(s *Snapshot, idx *index, usedFiles map[*descriptorpb.FileDescriptorProto]bool) error {
	for file := range usedFiles {
		if err := validateDescriptorFile(file); err != nil {
			return err
		}
		if err := checkEditionOptions(file, "file", file.GetOptions()); err != nil {
			return err
		}
	}
	check := func(file *descriptorpb.FileDescriptorProto, scope string, options proto.Message) error {
		return checkEditionOptions(file, scope, options)
	}
	serviceFile := idx.files[s.Service.Name]
	sd := idx.services[s.Service.Name]
	if err := check(serviceFile, "service", sd.GetOptions()); err != nil {
		return err
	}
	for _, method := range sd.Method {
		if err := check(serviceFile, "method", method.GetOptions()); err != nil {
			return err
		}
	}
	resolve := func(name string, file *descriptorpb.FileDescriptorProto, own proto.Message) (*descriptorpb.FeatureSet, error) {
		effective := edition2023Defaults()
		proto.Merge(effective, file.GetOptions().GetFeatures())
		parts := strings.Split(name, ".")
		for i := 1; i < len(parts); i++ {
			ancestor := idx.messages[strings.Join(parts[:i], ".")]
			if ancestor != nil {
				if err := check(file, "message", ancestor.GetOptions()); err != nil {
					return nil, err
				}
				proto.Merge(effective, ancestor.GetOptions().GetFeatures())
			}
		}
		proto.Merge(effective, optionFeatures(own))
		return effective, nil
	}
	appendResolved := func(name string, features *descriptorpb.FeatureSet) error {
		b, err := (proto.MarshalOptions{Deterministic: true}).Marshal(features)
		if err != nil {
			return err
		}
		s.Features = append(s.Features, Feature{Name: "edition-features:" + name, Value: hex.EncodeToString(b)})
		return nil
	}
	for _, message := range s.Messages {
		file, md := idx.files[message.Name], idx.messages[message.Name]
		if err := check(file, "message", md.GetOptions()); err != nil {
			return err
		}
		for _, oneof := range md.OneofDecl {
			if err := check(file, "oneof", oneof.GetOptions()); err != nil {
				return err
			}
		}
		for _, field := range md.Field {
			if err := check(file, "field", field.GetOptions()); err != nil {
				return err
			}
		}
		if file.GetSyntax() != "editions" {
			continue
		}
		effective, err := resolve(message.Name, file, md.GetOptions())
		if err != nil {
			return err
		}
		if err := appendResolved(message.Name, effective); err != nil {
			return err
		}
		for _, field := range md.Field {
			if field.GetLabel() == descriptorpb.FieldDescriptorProto_LABEL_REQUIRED || field.GetProto3Optional() || (field.GetOptions() != nil && field.GetOptions().Packed != nil) {
				return fmt.Errorf("unsupported legacy field label, proto3_optional, or packed option in editions field %s.%s", message.Name, field.GetName())
			}
			if rawFeatures := field.GetOptions().GetFeatures(); rawFeatures != nil && rawFeatures.MessageEncoding != nil {
				if target := idx.messages[strings.TrimPrefix(field.GetTypeName(), ".")]; target != nil && target.GetOptions().GetMapEntry() {
					return fmt.Errorf("message encoding overrides on maps are unsupported")
				}
			}
			own := proto.Clone(effective).(*descriptorpb.FeatureSet)
			proto.Merge(own, field.GetOptions().GetFeatures())
			if err := validateEditionFieldSettings(field, own); err != nil {
				return fmt.Errorf("field %s.%s: %w", message.Name, field.GetName(), err)
			}
			if err := appendResolved(message.Name+"."+field.GetName(), own); err != nil {
				return err
			}
		}
	}
	for _, enumeration := range s.Enums {
		file, ed := idx.files[enumeration.Name], idx.enums[enumeration.Name]
		if err := check(file, "enum", ed.GetOptions()); err != nil {
			return err
		}
		for _, value := range ed.Value {
			if err := check(file, "enum-value", value.GetOptions()); err != nil {
				return err
			}
		}
		if file.GetSyntax() != "editions" {
			continue
		}
		effective, err := resolve(enumeration.Name, file, ed.GetOptions())
		if err != nil {
			return err
		}
		if effective.GetEnumType() == descriptorpb.FeatureSet_OPEN && (len(ed.Value) == 0 || ed.Value[0].GetNumber() != 0) {
			return fmt.Errorf("open editions enum %s must start with zero", enumeration.Name)
		}
		if err := appendResolved(enumeration.Name, effective); err != nil {
			return err
		}
	}
	return nil
}

func checkEditionOptions(file *descriptorpb.FileDescriptorProto, scope string, options proto.Message) error {
	features := optionFeatures(options)
	if features == nil {
		return nil
	}
	if file.GetSyntax() != "editions" {
		return fmt.Errorf("unsupported edition features in %s file %s", file.GetSyntax(), file.GetName())
	}
	if scope == "file" && features.GetFieldPresence() == descriptorpb.FeatureSet_LEGACY_REQUIRED {
		return fmt.Errorf("LEGACY_REQUIRED is supported only on singular fields")
	}
	return validateCoreFeatures(features, scope, false)
}

func validateEditionFeature(s *Snapshot, owner string, features *descriptorpb.FeatureSet) error {
	syntax := ""
	if m, ok := findMessage(s, owner); ok {
		syntax = m.Syntax
		if features.GetFieldPresence() == descriptorpb.FeatureSet_LEGACY_REQUIRED {
			return fmt.Errorf("required presence is field-only")
		}
	} else if e, ok := findEnum(s, owner); ok {
		syntax = e.Syntax
		if features.GetFieldPresence() == descriptorpb.FeatureSet_LEGACY_REQUIRED {
			return fmt.Errorf("required presence is field-only")
		}
	} else if parent, member, ok := splitMember(owner); ok {
		if m, found := findMessage(s, parent); found {
			for _, f := range m.Fields {
				if f.Name == member {
					syntax = m.Syntax
				}
			}
		}
	}
	if syntax != "editions" {
		return fmt.Errorf("edition feature references missing or non-editions owner %q", owner)
	}
	return validateCoreFeatures(features, "resolved", true)
}

func validateEditionSnapshot(s *Snapshot) error {
	resolved := map[string]*descriptorpb.FeatureSet{}
	for _, feature := range s.Features {
		if !strings.HasPrefix(feature.Name, "edition-features:") {
			continue
		}
		fs := new(descriptorpb.FeatureSet)
		b, err := hex.DecodeString(feature.Value)
		if err != nil {
			return err
		}
		if err := proto.Unmarshal(b, fs); err != nil {
			return err
		}
		resolved[strings.TrimPrefix(feature.Name, "edition-features:")] = fs
	}
	for _, m := range s.Messages {
		if m.Syntax != "editions" {
			continue
		}
		if resolved[m.Name] == nil {
			return fmt.Errorf("missing resolved edition features for %s", m.Name)
		}
		for _, f := range m.Fields {
			fs := resolved[m.Name+"."+f.Name]
			if fs == nil {
				return fmt.Errorf("missing resolved edition features for %s.%s", m.Name, f.Name)
			}
			if f.Label == "LABEL_REQUIRED" || f.Proto3Optional {
				return fmt.Errorf("legacy field presence in editions field %s.%s", m.Name, f.Name)
			}
			parent := resolved[m.Name]
			if fs.GetUtf8Validation() != parent.GetUtf8Validation() && f.Type != "TYPE_STRING" {
				return fmt.Errorf("UTF-8 override requires a string field")
			}
			packable := f.Label == "LABEL_REPEATED" && f.Type != "TYPE_MESSAGE" && f.Type != "TYPE_STRING" && f.Type != "TYPE_BYTES"
			if fs.GetRepeatedFieldEncoding() != parent.GetRepeatedFieldEncoding() && !packable {
				return fmt.Errorf("repeated encoding override requires a repeated packable field")
			}
			if fs.GetFieldPresence() != parent.GetFieldPresence() && (f.Label == "LABEL_REPEATED" || f.Oneof != "" || f.Type == "TYPE_MESSAGE" && fs.GetFieldPresence() == descriptorpb.FeatureSet_IMPLICIT) {
				return fmt.Errorf("invalid field presence override")
			}
			if fs.GetMessageEncoding() != parent.GetMessageEncoding() {
				target, _ := findMessage(s, f.TypeName)
				if f.Type != "TYPE_MESSAGE" || target.MapEntry {
					return fmt.Errorf("message encoding override requires a non-map message field")
				}
			}
			if fs.GetEnumType() != resolved[m.Name].GetEnumType() || fs.GetJsonFormat() != resolved[m.Name].GetJsonFormat() {
				return fmt.Errorf("field-only edition configuration cannot override enum or JSON policy: %s.%s", m.Name, f.Name)
			}
			if fs.GetFieldPresence() == descriptorpb.FeatureSet_LEGACY_REQUIRED && (f.Label == "LABEL_REPEATED" || f.Oneof != "") {
				return fmt.Errorf("invalid required edition presence for %s.%s", m.Name, f.Name)
			}
			if f.Type == "TYPE_ENUM" && f.Label != "LABEL_REPEATED" && f.Oneof == "" && fs.GetFieldPresence() == descriptorpb.FeatureSet_IMPLICIT {
				if target, ok := findEnum(s, f.TypeName); ok && target.Syntax == "proto2" {
					return fmt.Errorf("implicit presence may only use open enums: %s.%s", m.Name, f.Name)
				}
				if enumFeatures := resolved[f.TypeName]; enumFeatures != nil && enumFeatures.GetEnumType() == descriptorpb.FeatureSet_CLOSED {
					return fmt.Errorf("implicit presence may only use open enums: %s.%s", m.Name, f.Name)
				}
			}
			if f.HasDefault && fs.GetFieldPresence() == descriptorpb.FeatureSet_IMPLICIT {
				return fmt.Errorf("implicit editions field cannot have an explicit default")
			}
		}
	}
	for _, e := range s.Enums {
		if e.Syntax == "editions" {
			fs := resolved[e.Name]
			if fs == nil {
				return fmt.Errorf("missing resolved edition features for %s", e.Name)
			}
			if fs.GetEnumType() == descriptorpb.FeatureSet_OPEN {
				for _, v := range e.Values {
					if v.Name == e.DefaultName && v.Number != 0 {
						return fmt.Errorf("open editions enum %s must start with zero", e.Name)
					}
				}
			}
		}
	}
	return nil
}

// validateEditionOptionFeature also applies when reading a hand-authored lock:
// options cannot bypass the supported feature surface via standard fingerprints.
func validateEditionOptionFeature(s *Snapshot, kind, owner string, options proto.Message) error {
	if fieldOptions, ok := options.(*descriptorpb.FieldOptions); ok && fieldOptions != nil && fieldOptions.Packed != nil {
		if parent, _, valid := splitMember(owner); valid {
			if m, found := findMessage(s, parent); found && m.Syntax == "editions" {
				return fmt.Errorf("legacy packed option is unsupported on editions field %s", owner)
			}
		}
	}
	if optionFeatures(options) != nil {
		return fmt.Errorf("raw edition feature options are unsupported in locks; use resolved edition-features for %s", owner)
	}
	return nil
}

func clearOptionFeatures(options proto.Message) {
	if options == nil || !options.ProtoReflect().IsValid() {
		return
	}
	m := options.ProtoReflect()
	if f := m.Descriptor().Fields().ByName("features"); f != nil {
		m.Clear(f)
	}
}

func editionFieldRequired(s *Snapshot, owner string) bool {
	for _, feature := range s.Features {
		if feature.Name == "edition-features:"+owner {
			b, err := hex.DecodeString(feature.Value)
			if err != nil {
				return false
			}
			fs := new(descriptorpb.FeatureSet)
			if proto.Unmarshal(b, fs) == nil {
				return fs.GetFieldPresence() == descriptorpb.FeatureSet_LEGACY_REQUIRED
			}
		}
	}
	return false
}

func validateEditionFieldSettings(field *descriptorpb.FieldDescriptorProto, effective *descriptorpb.FeatureSet) error {
	explicit := field.GetOptions().GetFeatures()
	if explicit != nil {
		if explicit.FieldPresence != nil && (field.GetLabel() == descriptorpb.FieldDescriptorProto_LABEL_REPEATED || field.OneofIndex != nil) {
			return fmt.Errorf("presence overrides on repeated or oneof fields are unsupported")
		}
		if explicit.GetFieldPresence() == descriptorpb.FeatureSet_IMPLICIT && field.GetType() == descriptorpb.FieldDescriptorProto_TYPE_MESSAGE {
			return fmt.Errorf("message fields cannot use implicit presence")
		}
		if explicit.RepeatedFieldEncoding != nil && (field.GetLabel() != descriptorpb.FieldDescriptorProto_LABEL_REPEATED || field.GetType() == descriptorpb.FieldDescriptorProto_TYPE_MESSAGE || field.GetType() == descriptorpb.FieldDescriptorProto_TYPE_STRING || field.GetType() == descriptorpb.FieldDescriptorProto_TYPE_BYTES) {
			return fmt.Errorf("repeated encoding requires a repeated packable field")
		}
		if explicit.Utf8Validation != nil && field.GetType() != descriptorpb.FieldDescriptorProto_TYPE_STRING {
			return fmt.Errorf("UTF-8 validation requires a string field")
		}
		if explicit.MessageEncoding != nil && field.GetType() != descriptorpb.FieldDescriptorProto_TYPE_MESSAGE {
			return fmt.Errorf("message encoding requires a message field")
		}
	}
	if effective.GetFieldPresence() == descriptorpb.FeatureSet_LEGACY_REQUIRED && (field.GetLabel() == descriptorpb.FieldDescriptorProto_LABEL_REPEATED || field.OneofIndex != nil) {
		return fmt.Errorf("required presence requires a singular field outside a oneof")
	}
	if field.DefaultValue != nil && effective.GetFieldPresence() == descriptorpb.FeatureSet_IMPLICIT {
		return fmt.Errorf("implicit field cannot have a default")
	}
	return nil
}
