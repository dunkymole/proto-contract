package contract

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/descriptorpb"
)

// MaxLockBytes bounds untrusted lock parsing before JSON decoding.
const MaxLockBytes = 4 << 20

func readBoundedFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, MaxLockBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxLockBytes {
		return nil, fmt.Errorf("file %q exceeds maximum size of %d bytes", path, MaxLockBytes)
	}
	return data, nil
}

func decodeStrictJSON(data []byte, target any) error {
	scanner := json.NewDecoder(bytes.NewReader(data))
	if err := scanJSONValue(scanner); err != nil {
		return err
	}
	if _, err := scanner.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("JSON contains trailing data")
		}
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("JSON contains trailing data")
		}
		return err
	}
	return nil
}

func scanJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok {
				return fmt.Errorf("JSON object key is not a string")
			}
			if strings.ToLower(name) != name {
				return fmt.Errorf("JSON object key %q must use canonical lowercase spelling", name)
			}
			if seen[name] {
				return fmt.Errorf("duplicate JSON object key %q", name)
			}
			seen[name] = true
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim('}') {
			return fmt.Errorf("malformed JSON object")
		}
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim(']') {
			return fmt.Errorf("malformed JSON array")
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delim)
	}
	return nil
}

var (
	apiPattern      = regexp.MustCompile(`^[A-Za-z0-9._:/-]{1,64}$`)
	qualifiedName   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$`)
	protobufName    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	canonicalNumber = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
)

// Validate checks the lock as a complete, internally consistent graph. Call
// this before trusting a lock from disk or emitting generated code.
func Validate(s *Snapshot) error {
	if s == nil {
		return fmt.Errorf("lock is empty")
	}
	if s.Format != 2 {
		return fmt.Errorf("unsupported contract lock format %d", s.Format)
	}
	if !apiPattern.MatchString(s.API) {
		return fmt.Errorf("invalid API identifier %q", s.API)
	}
	if err := validateVersion(s.Version); err != nil {
		return err
	}
	if !qualifiedName.MatchString(s.Service.Name) {
		return fmt.Errorf("invalid service name %q", s.Service.Name)
	}
	if s.Messages == nil || s.Enums == nil || s.Service.Methods == nil {
		return fmt.Errorf("lock graph arrays must be present")
	}
	messages, enums := map[string]Message{}, map[string]Enum{}
	for _, m := range s.Messages {
		if !qualifiedName.MatchString(m.Name) {
			return fmt.Errorf("invalid message name %q", m.Name)
		}
		if _, ok := messages[m.Name]; ok {
			return fmt.Errorf("duplicate message %q", m.Name)
		}
		if _, ok := enums[m.Name]; ok {
			return fmt.Errorf("message and enum share name %q", m.Name)
		}
		if m.Fields == nil {
			return fmt.Errorf("fields for message %q must be present", m.Name)
		}
		if m.Syntax != "proto2" && m.Syntax != "proto3" {
			return fmt.Errorf("invalid syntax %q for message %s", m.Syntax, m.Name)
		}
		messages[m.Name] = m
	}
	for _, e := range s.Enums {
		if !qualifiedName.MatchString(e.Name) {
			return fmt.Errorf("invalid enum name %q", e.Name)
		}
		if _, ok := enums[e.Name]; ok {
			return fmt.Errorf("duplicate enum %q", e.Name)
		}
		if _, ok := messages[e.Name]; ok {
			return fmt.Errorf("message and enum share name %q", e.Name)
		}
		if e.Values == nil {
			return fmt.Errorf("values for enum %q must be present", e.Name)
		}
		if e.Syntax != "proto2" && e.Syntax != "proto3" {
			return fmt.Errorf("invalid syntax %q for enum %s", e.Syntax, e.Name)
		}
		enums[e.Name] = e
	}
	methodNames := map[string]bool{}
	for index, method := range s.Service.Methods {
		if !protobufName.MatchString(method.Name) || methodNames[method.Name] {
			return fmt.Errorf("invalid or duplicate method %q", method.Name)
		}
		methodNames[method.Name] = true
		if index > 0 && s.Service.Methods[index-1].Name >= method.Name {
			return fmt.Errorf("methods are not in canonical name order")
		}
		if _, ok := messages[method.Input]; !ok {
			return fmt.Errorf("method %s has dangling input message %q", method.Name, method.Input)
		} else if messages[method.Input].MapEntry {
			return fmt.Errorf("method %s input cannot be a synthetic map-entry message", method.Name)
		}
		if _, ok := messages[method.Output]; !ok {
			return fmt.Errorf("method %s has dangling output message %q", method.Name, method.Output)
		} else if messages[method.Output].MapEntry {
			return fmt.Errorf("method %s output cannot be a synthetic map-entry message", method.Name)
		}
	}
	for messageIndex, message := range s.Messages {
		if messageIndex > 0 && s.Messages[messageIndex-1].Name >= message.Name {
			return fmt.Errorf("messages are not in canonical name order")
		}
		fieldNumbers, fieldNames := map[int32]bool{}, map[string]bool{}
		oneofCounts, syntheticOneofs := map[string]int{}, map[string]int{}
		for fieldIndex, field := range message.Fields {
			if fieldIndex > 0 && message.Fields[fieldIndex-1].Number >= field.Number {
				return fmt.Errorf("fields in %s are not in canonical field-number order", message.Name)
			}
			if field.Number < 1 || field.Number > 536870911 || field.Number >= 19000 && field.Number <= 19999 {
				return fmt.Errorf("invalid field number %d in %s", field.Number, message.Name)
			}
			if fieldNumbers[field.Number] || !protobufName.MatchString(field.Name) || fieldNames[field.Name] {
				return fmt.Errorf("duplicate or invalid field %q/%d in %s", field.Name, field.Number, message.Name)
			}
			fieldNumbers[field.Number], fieldNames[field.Name] = true, true
			if !validDescriptorEnum(field.Type, descriptorpb.FieldDescriptorProto_Type_name) || field.Type == "TYPE_UNKNOWN" || field.Type == "TYPE_GROUP" {
				return fmt.Errorf("invalid or unsupported field type %q in %s.%s", field.Type, message.Name, field.Name)
			}
			if !validDescriptorEnum(field.Label, descriptorpb.FieldDescriptorProto_Label_name) || field.Label == "LABEL_UNKNOWN" {
				return fmt.Errorf("invalid field label %q in %s.%s", field.Label, message.Name, field.Name)
			}
			if message.Syntax == "proto3" && field.Label == descriptorpb.FieldDescriptorProto_LABEL_REQUIRED.String() {
				return fmt.Errorf("proto3 field %s.%s cannot be required", message.Name, field.Name)
			}
			isRef := field.Type == descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.String() || field.Type == descriptorpb.FieldDescriptorProto_TYPE_ENUM.String()
			if isRef != (field.TypeName != "") || field.TypeName != "" && !qualifiedName.MatchString(field.TypeName) {
				return fmt.Errorf("invalid type reference %q in %s.%s", field.TypeName, message.Name, field.Name)
			}
			if field.Type == descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.String() {
				if _, ok := messages[field.TypeName]; !ok {
					return fmt.Errorf("field %s.%s references missing message %q", message.Name, field.Name, field.TypeName)
				}
			}
			if field.Type == descriptorpb.FieldDescriptorProto_TYPE_ENUM.String() {
				if _, ok := enums[field.TypeName]; !ok {
					return fmt.Errorf("field %s.%s references missing enum %q", message.Name, field.Name, field.TypeName)
				}
			}
			if !field.HasDefault && field.DefaultValue != "" {
				return fmt.Errorf("field %s.%s has a value without explicit-default marker", message.Name, field.Name)
			}
			if field.HasDefault && field.Type == descriptorpb.FieldDescriptorProto_TYPE_ENUM.String() {
				found := false
				for _, value := range enums[field.TypeName].Values {
					if value.Name == field.DefaultValue {
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("field %s.%s has unknown enum default %q", message.Name, field.Name, field.DefaultValue)
				}
			}
			if field.HasDefault {
				if message.Syntax != "proto2" || field.Label == descriptorpb.FieldDescriptorProto_LABEL_REPEATED.String() || field.Type == descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.String() || field.Type == descriptorpb.FieldDescriptorProto_TYPE_GROUP.String() {
					return fmt.Errorf("field %s.%s cannot declare a default with its syntax, label, or type", message.Name, field.Name)
				}
				if field.Oneof != "" {
					return fmt.Errorf("oneof field %s.%s cannot declare a default", message.Name, field.Name)
				}
				if err := validateDefault(field); err != nil {
					return fmt.Errorf("invalid default for %s.%s: %w", message.Name, field.Name, err)
				}
			}
			if field.Oneof != "" && (!protobufName.MatchString(field.Oneof) || field.Label != descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.String()) {
				return fmt.Errorf("invalid oneof membership for %s.%s", message.Name, field.Name)
			}
			if field.Oneof != "" {
				oneofCounts[field.Oneof]++
			}
			if field.Proto3Optional && (message.Syntax != "proto3" || field.Oneof == "" || field.Label != descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.String()) {
				return fmt.Errorf("invalid proto3_optional declaration for %s.%s", message.Name, field.Name)
			}
			if field.Proto3Optional {
				syntheticOneofs[field.Oneof]++
			}
			if message.MapEntry && (field.Name != "key" && field.Name != "value" || field.Name == "key" && !validMapKey(field.Type) || field.Label != descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.String() || field.Oneof != "" || field.Proto3Optional || field.HasDefault) {
				return fmt.Errorf("invalid map-entry field %s.%s", message.Name, field.Name)
			}
		}
		for name, count := range syntheticOneofs {
			if count != 1 || oneofCounts[name] != 1 {
				return fmt.Errorf("synthetic proto3_optional oneof %s.%s must contain only its optional field", message.Name, name)
			}
		}
		if message.MapEntry && !validMapEntry(message) {
			return fmt.Errorf("malformed map-entry message %s", message.Name)
		}
	}
	for _, message := range s.Messages {
		for _, field := range message.Fields {
			if messageType := messages[field.TypeName]; messageType.MapEntry && field.Label != descriptorpb.FieldDescriptorProto_LABEL_REPEATED.String() {
				return fmt.Errorf("map-entry type %s must be referenced by a repeated field", field.TypeName)
			}
		}
	}
	for enumIndex, e := range s.Enums {
		if enumIndex > 0 && s.Enums[enumIndex-1].Name >= e.Name {
			return fmt.Errorf("enums are not in canonical name order")
		}
		if len(e.Values) == 0 {
			return fmt.Errorf("enum %s has no values", e.Name)
		}
		defaultIsFirst := false
		for _, value := range e.Values {
			if value.Name == e.DefaultName && value.AliasOrder == 0 {
				defaultIsFirst = true
			}
		}
		if !defaultIsFirst {
			return fmt.Errorf("enum %s must record its first declared value as default", e.Name)
		}
		if e.Syntax == "proto3" {
			defaultIsZeroFirst := false
			for _, value := range e.Values {
				if value.Name == e.DefaultName && value.Number == 0 && value.AliasOrder == 0 {
					defaultIsZeroFirst = true
				}
			}
			if !defaultIsZeroFirst {
				return fmt.Errorf("proto3 enum %s must declare its zero-valued default first", e.Name)
			}
		}
		valueNames, aliasRanks := map[string]bool{}, map[int32]map[int]bool{}
		for valueIndex, value := range e.Values {
			if valueIndex > 0 && (e.Values[valueIndex-1].Number > value.Number || e.Values[valueIndex-1].Number == value.Number && e.Values[valueIndex-1].AliasOrder >= value.AliasOrder) {
				return fmt.Errorf("enum values in %s are not in canonical number/alias order", e.Name)
			}
			if !protobufName.MatchString(value.Name) || valueNames[value.Name] {
				return fmt.Errorf("invalid or duplicate enum value %q in %s", value.Name, e.Name)
			}
			valueNames[value.Name] = true
			if value.AliasOrder < 0 {
				return fmt.Errorf("negative enum alias order in %s.%s", e.Name, value.Name)
			}
			if aliasRanks[value.Number] == nil {
				aliasRanks[value.Number] = map[int]bool{}
			}
			if aliasRanks[value.Number][value.AliasOrder] {
				return fmt.Errorf("duplicate enum alias order in %s", e.Name)
			}
			aliasRanks[value.Number][value.AliasOrder] = true
		}
		for number, ranks := range aliasRanks {
			orders := make([]int, 0, len(ranks))
			for rank := range ranks {
				orders = append(orders, rank)
			}
			sort.Ints(orders)
			for i, rank := range orders {
				if i != rank {
					return fmt.Errorf("noncontiguous alias order for enum %s value %d", e.Name, number)
				}
			}
		}
	}
	featureNames := map[string]bool{}
	for featureIndex, feature := range s.Features {
		if feature.Name == "" || featureNames[feature.Name] {
			return fmt.Errorf("empty or duplicate descriptor feature %q", feature.Name)
		}
		featureNames[feature.Name] = true
		if featureIndex > 0 && s.Features[featureIndex-1].Name >= feature.Name {
			return fmt.Errorf("descriptor features are not in canonical name order")
		}
		if err := validateFeature(s, feature); err != nil {
			return err
		}
	}
	for _, enum := range s.Enums {
		numbers := map[int32]bool{}
		hasAlias := false
		for _, value := range enum.Values {
			if numbers[value.Number] {
				hasAlias = true
			}
			numbers[value.Number] = true
		}
		if hasAlias && !enumAllowsAlias(s.Features, enum.Name) {
			return fmt.Errorf("enum %s has duplicate numeric values without allow_alias", enum.Name)
		}
	}
	digest, err := calculateDigest(s)
	if err != nil {
		return err
	}
	if s.Digest != digest {
		return fmt.Errorf("digest mismatch: stored %q, calculated %q", s.Digest, digest)
	}
	return nil
}

func validDescriptorEnum(value string, values map[int32]string) bool {
	for _, name := range values {
		if name == value {
			return true
		}
	}
	return false
}

func validMapKey(value string) bool {
	switch value {
	case "TYPE_INT32", "TYPE_INT64", "TYPE_UINT32", "TYPE_UINT64", "TYPE_SINT32", "TYPE_SINT64", "TYPE_FIXED32", "TYPE_FIXED64", "TYPE_SFIXED32", "TYPE_SFIXED64", "TYPE_BOOL", "TYPE_STRING":
		return true
	default:
		return false
	}
}

func validMapEntry(message Message) bool {
	if len(message.Fields) != 2 {
		return false
	}
	key, value := message.Fields[0], message.Fields[1]
	return key.Number == 1 && key.Name == "key" && validMapKey(key.Type) && value.Number == 2 && value.Name == "value"
}

func validateDefault(field Field) error {
	if field.Type == descriptorpb.FieldDescriptorProto_TYPE_ENUM.String() {
		return nil // Enum names are checked against the referenced declaration.
	}
	typeValue := descriptorpb.FieldDescriptorProto_Type_value[field.Type]
	labelValue := descriptorpb.FieldDescriptorProto_Label_value[field.Label]
	descriptor := &descriptorpb.FileDescriptorProto{
		Name:        proto.String("contract-default-validation.proto"),
		Syntax:      proto.String("proto2"),
		MessageType: []*descriptorpb.DescriptorProto{{Name: proto.String("Value"), Field: []*descriptorpb.FieldDescriptorProto{{Name: proto.String("value"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_Type(typeValue).Enum(), Label: descriptorpb.FieldDescriptorProto_Label(labelValue).Enum(), DefaultValue: proto.String(field.DefaultValue)}}}},
	}
	_, err := protodesc.NewFile(descriptor, nil)
	return err
}

func validateFeature(snapshot *Snapshot, feature Feature) error {
	separator := strings.IndexByte(feature.Name, ':')
	if separator <= 0 || separator == len(feature.Name)-1 {
		return fmt.Errorf("invalid descriptor feature name %q", feature.Name)
	}
	kind, owner := feature.Name[:separator], feature.Name[separator+1:]
	var message proto.Message
	switch kind {
	case "file-options":
		message = &descriptorpb.FileOptions{}
	case "service-options":
		if owner != snapshot.Service.Name {
			return fmt.Errorf("service feature has invalid owner %q", owner)
		}
		message = &descriptorpb.ServiceOptions{}
	case "method-options":
		prefix := snapshot.Service.Name + "."
		if !strings.HasPrefix(owner, prefix) {
			return fmt.Errorf("method feature has invalid owner %q", owner)
		}
		found := false
		for _, method := range snapshot.Service.Methods {
			if owner == prefix+method.Name {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("method feature references missing method %q", owner)
		}
		message = &descriptorpb.MethodOptions{}
	case "message-options", "message-declarations":
		if _, ok := findMessage(snapshot, owner); !ok {
			return fmt.Errorf("message feature references missing message %q", owner)
		}
		if kind == "message-options" {
			message = &descriptorpb.MessageOptions{}
		} else {
			message = &descriptorpb.DescriptorProto{}
		}
	case "field-options":
		messageName, fieldName, ok := splitMember(owner)
		if !ok {
			return fmt.Errorf("invalid field feature owner %q", owner)
		}
		m, found := findMessage(snapshot, messageName)
		if !found {
			return fmt.Errorf("field feature references missing message %q", messageName)
		}
		found = false
		for _, field := range m.Fields {
			if field.Name == fieldName {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("field feature references missing field %q", owner)
		}
		message = &descriptorpb.FieldOptions{}
	case "oneof-options":
		messageName, oneofName, ok := splitMember(owner)
		if !ok {
			return fmt.Errorf("invalid oneof feature owner %q", owner)
		}
		m, found := findMessage(snapshot, messageName)
		if !found {
			return fmt.Errorf("oneof feature references missing message %q", messageName)
		}
		found = false
		for _, field := range m.Fields {
			if field.Oneof == oneofName {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("oneof feature references missing declaration %q", owner)
		}
		message = &descriptorpb.OneofOptions{}
	case "enum-options", "enum-declarations":
		if _, found := findEnum(snapshot, owner); !found {
			return fmt.Errorf("enum feature references missing enum %q", owner)
		}
		if kind == "enum-options" {
			message = &descriptorpb.EnumOptions{}
		} else {
			message = &descriptorpb.EnumDescriptorProto{}
		}
	case "enum-value-options":
		enumName, valueName, ok := splitMember(owner)
		if !ok {
			return fmt.Errorf("invalid enum-value feature owner %q", owner)
		}
		enum, found := findEnum(snapshot, enumName)
		if !found {
			return fmt.Errorf("enum-value feature references missing enum %q", enumName)
		}
		found = false
		for _, value := range enum.Values {
			if value.Name == valueName {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("enum-value feature references missing value %q", owner)
		}
		message = &descriptorpb.EnumValueOptions{}
	default:
		return fmt.Errorf("unsupported descriptor feature %q", kind)
	}
	encoded, err := hex.DecodeString(feature.Value)
	if err != nil || hex.EncodeToString(encoded) != feature.Value {
		return fmt.Errorf("descriptor feature %q is not canonical hex", feature.Name)
	}
	if err := proto.Unmarshal(encoded, message); err != nil {
		return fmt.Errorf("invalid descriptor feature %q: %w", feature.Name, err)
	}
	if len(message.ProtoReflect().GetUnknown()) != 0 {
		return fmt.Errorf("unsupported unknown data in descriptor feature %q", feature.Name)
	}
	if field := message.ProtoReflect().Descriptor().Fields().ByName("uninterpreted_option"); field != nil && message.ProtoReflect().Has(field) && message.ProtoReflect().Get(field).List().Len() > 0 {
		return fmt.Errorf("unsupported uninterpreted option in descriptor feature %q", feature.Name)
	}
	canonical, err := (proto.MarshalOptions{Deterministic: true}).Marshal(message)
	if err != nil || !bytes.Equal(canonical, encoded) {
		return fmt.Errorf("descriptor feature %q is not deterministically encoded", feature.Name)
	}
	return nil
}

func findMessage(snapshot *Snapshot, name string) (Message, bool) {
	for _, message := range snapshot.Messages {
		if message.Name == name {
			return message, true
		}
	}
	return Message{}, false
}
func findEnum(snapshot *Snapshot, name string) (Enum, bool) {
	for _, value := range snapshot.Enums {
		if value.Name == name {
			return value, true
		}
	}
	return Enum{}, false
}

func enumAllowsAlias(features []Feature, name string) bool {
	for _, feature := range features {
		if feature.Name != "enum-options:"+name {
			continue
		}
		data, err := hex.DecodeString(feature.Value)
		if err != nil {
			return false
		}
		options := &descriptorpb.EnumOptions{}
		if proto.Unmarshal(data, options) != nil {
			return false
		}
		return options.GetAllowAlias()
	}
	return false
}

func validateVersion(value string) error {
	if !canonicalNumber.MatchString(value) {
		return fmt.Errorf("version %q must be canonical MAJOR.MINOR.PATCH", value)
	}
	if _, err := NextVersion(value, None); err != nil {
		return err
	}
	return nil
}

func versionParts(value string) ([3]uint64, error) {
	var parts [3]uint64
	if err := validateVersion(value); err != nil {
		return parts, err
	}
	for i, component := range strings.Split(value, ".") {
		var n uint64
		for _, ch := range component {
			n = n*10 + uint64(ch-'0')
		}
		parts[i] = n
	}
	return parts, nil
}
