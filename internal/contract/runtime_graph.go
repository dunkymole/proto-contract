package contract

import (
	"encoding/base64"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

const runtimeGraphFormat = "protobuf-es.runtime-graph.v1"

// RuntimeGraph projects the descriptor semantics retained by Protobuf-ES.
// Source-only descriptor data remains covered by the full contract lock and
// must be validated against the same CodeGeneratorRequest before generation.
func RuntimeGraph(set *descriptorpb.FileDescriptorSet, serviceName string) (map[string]any, error) {
	if set == nil {
		return nil, fmt.Errorf("missing descriptor set")
	}
	files, err := protodesc.NewFiles(set)
	if err != nil {
		return nil, fmt.Errorf("build protobuf descriptors: %w", err)
	}
	serviceDesc, err := files.FindDescriptorByName(protoreflect.FullName(strings.TrimPrefix(serviceName, ".")))
	if err != nil {
		return nil, fmt.Errorf("find service descriptor: %w", err)
	}
	service, ok := serviceDesc.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil, fmt.Errorf("service %q not found in descriptor set", serviceName)
	}
	rawFields := make(map[protoreflect.FullName]map[protoreflect.FieldNumber]*descriptorpb.FieldDescriptorProto)
	for _, file := range set.File {
		for _, message := range file.GetMessageType() {
			indexRawMessageFields(rawFields, protoreflect.FullName(file.GetPackage()), message)
		}
	}

	messages := map[protoreflect.FullName]protoreflect.MessageDescriptor{}
	enums := map[protoreflect.FullName]protoreflect.EnumDescriptor{}
	var visitMessage func(protoreflect.MessageDescriptor) error
	visitType := func(kind protoreflect.Kind, message protoreflect.MessageDescriptor, enumeration protoreflect.EnumDescriptor) error {
		switch kind {
		case protoreflect.MessageKind, protoreflect.GroupKind:
			if message == nil {
				return fmt.Errorf("message descriptor is missing")
			}
			return visitMessage(message)
		case protoreflect.EnumKind:
			if enumeration == nil {
				return fmt.Errorf("enum descriptor is missing")
			}
			enums[enumeration.FullName()] = enumeration
		}
		return nil
	}
	visitMessage = func(message protoreflect.MessageDescriptor) error {
		if message == nil || messages[message.FullName()] != nil {
			return nil
		}
		// Synthetic map-entry messages do not appear in the Protobuf-ES graph.
		messages[message.FullName()] = message
		fields := message.Fields()
		for i := 0; i < fields.Len(); i++ {
			field := fields.Get(i)
			if field.IsMap() {
				value := field.MapValue()
				if err := visitType(value.Kind(), value.Message(), value.Enum()); err != nil {
					return fmt.Errorf("field %s: %w", field.FullName(), err)
				}
				continue
			}
			if field.Kind() == protoreflect.MessageKind || field.Kind() == protoreflect.GroupKind || field.Kind() == protoreflect.EnumKind {
				if err := visitType(field.Kind(), field.Message(), field.Enum()); err != nil {
					return fmt.Errorf("field %s: %w", field.FullName(), err)
				}
			}
		}
		return nil
	}
	methods := make([]map[string]any, 0, service.Methods().Len())
	for i := 0; i < service.Methods().Len(); i++ {
		method := service.Methods().Get(i)
		if err := visitMessage(method.Input()); err != nil {
			return nil, fmt.Errorf("method %s input: %w", method.FullName(), err)
		}
		if err := visitMessage(method.Output()); err != nil {
			return nil, fmt.Errorf("method %s output: %w", method.FullName(), err)
		}
		kind := "unary"
		switch {
		case method.IsStreamingClient() && method.IsStreamingServer():
			kind = "bidi_streaming"
		case method.IsStreamingClient():
			kind = "client_streaming"
		case method.IsStreamingServer():
			kind = "server_streaming"
		}
		localName := lowerInitial(method.Name())
		methods = append(methods, map[string]any{
			"name": string(method.Name()), "localName": safeObjectProperty(localName), "kind": kind,
			"input": string(method.Input().FullName()), "output": string(method.Output().FullName()),
			"idempotency": int(methodIdempotency(method)),
		})
	}
	sort.Slice(methods, func(i, j int) bool { return methods[i]["name"].(string) < methods[j]["name"].(string) })

	messageNames := sortedMessageNames(messages)
	messageGraph := make([]map[string]any, 0, len(messageNames))
	for _, name := range messageNames {
		message := messages[name]
		fields := message.Fields()
		fieldGraph := make([]map[string]any, 0, fields.Len())
		for i := 0; i < fields.Len(); i++ {
			field := fields.Get(i)
			fieldGraph = append(fieldGraph, projectField(field, rawFields[name][field.Number()]))
		}
		sort.Slice(fieldGraph, func(i, j int) bool { return fieldGraph[i]["number"].(int32) < fieldGraph[j]["number"].(int32) })
		oneofs := make([]map[string]any, 0, message.Oneofs().Len())
		for i := 0; i < message.Oneofs().Len(); i++ {
			oneof := message.Oneofs().Get(i)
			memberNumbers := make([]int, 0, oneof.Fields().Len())
			for j := 0; j < oneof.Fields().Len(); j++ {
				field := oneof.Fields().Get(j)
				if isSyntheticOneof(field) {
					continue
				}
				memberNumbers = append(memberNumbers, int(field.Number()))
			}
			if len(memberNumbers) == 0 {
				continue
			}
			sort.Ints(memberNumbers)
			oneofs = append(oneofs, map[string]any{"name": string(oneof.Name()), "localName": safeObjectProperty(protoCamelCase(string(oneof.Name()))), "fields": memberNumbers})
		}
		sort.Slice(oneofs, func(i, j int) bool { return oneofs[i]["name"].(string) < oneofs[j]["name"].(string) })
		messageGraph = append(messageGraph, map[string]any{"typeName": string(name), "fields": fieldGraph, "oneofs": oneofs})
	}

	enumNames := sortedEnumNames(enums)
	enumGraph := make([]map[string]any, 0, len(enumNames))
	for _, name := range enumNames {
		enumeration := enums[name]
		values := enumeration.Values()
		defaultName, defaultNumber := "", int32(0)
		if values.Len() != 0 {
			defaultName = string(values.Get(0).Name())
			defaultNumber = int32(values.Get(0).Number())
		}
		aliases := map[int32][]map[string]any{}
		sharedPrefix := enumSharedPrefix(enumeration)
		for i := 0; i < values.Len(); i++ {
			value := values.Get(i)
			aliases[int32(value.Number())] = append(aliases[int32(value.Number())], map[string]any{
				"name": string(value.Name()), "localName": safeObjectProperty(enumValueLocalName(sharedPrefix, value.Name())),
			})
		}
		numbers := make([]int, 0, len(aliases))
		for number := range aliases {
			numbers = append(numbers, int(number))
		}
		sort.Ints(numbers)
		valueGraph := make([]map[string]any, 0, len(numbers))
		for _, number := range numbers {
			valueGraph = append(valueGraph, map[string]any{"number": number, "aliases": aliases[int32(number)]})
		}
		enumGraph = append(enumGraph, map[string]any{
			"typeName": string(name), "open": enumIsOpen(enumeration), "defaultName": defaultName,
			"defaultNumber": defaultNumber, "values": valueGraph,
		})
	}
	return map[string]any{
		"format":   runtimeGraphFormat,
		"service":  map[string]any{"typeName": string(service.FullName()), "methods": methods},
		"messages": messageGraph,
		"enums":    enumGraph,
	}, nil
}

func indexRawMessageFields(index map[protoreflect.FullName]map[protoreflect.FieldNumber]*descriptorpb.FieldDescriptorProto, prefix protoreflect.FullName, message *descriptorpb.DescriptorProto) {
	name := protoreflect.FullName(join(string(prefix), message.GetName()))
	fields := make(map[protoreflect.FieldNumber]*descriptorpb.FieldDescriptorProto, len(message.GetField()))
	for _, field := range message.GetField() {
		fields[protoreflect.FieldNumber(field.GetNumber())] = field
	}
	index[name] = fields
	for _, nested := range message.GetNestedType() {
		indexRawMessageFields(index, name, nested)
	}
}

func projectField(field protoreflect.FieldDescriptor, raw *descriptorpb.FieldDescriptorProto) map[string]any {
	realOneof := field.ContainingOneof() != nil && !isSyntheticOneof(field)
	var oneof any
	localName := protoCamelCase(string(field.Name()))
	if realOneof {
		oneof = string(field.ContainingOneof().Name())
	} else {
		oneof = nil
		localName = safeObjectProperty(localName)
	}
	shape := projectFieldShape(field)
	var defaultValue any
	if field.HasDefault() {
		defaultValue = projectDefault(field, raw)
	}
	return map[string]any{
		"number": int32(field.Number()), "name": string(field.Name()), "localName": localName,
		"jsonName": field.JSONName(), "utf8Validation": utf8Validated(field),
		"presence": fieldPresence(field), "oneof": oneof, "default": defaultValue, "shape": shape,
	}
}

func projectFieldShape(field protoreflect.FieldDescriptor) map[string]any {
	if field.IsMap() {
		key, value := field.MapKey(), field.MapValue()
		return map[string]any{"kind": "map", "keyScalar": scalarName(key.Kind()), "value": elementShape(value, false), "delimitedEncoding": false}
	}
	if field.IsList() {
		return map[string]any{"kind": "list", "element": elementShape(field, true), "packed": field.IsPacked()}
	}
	return elementShape(field, true)
}

func elementShape(field protoreflect.FieldDescriptor, includeLongAsString bool) map[string]any {
	switch field.Kind() {
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return map[string]any{"kind": "message", "typeName": string(field.Message().FullName()), "delimitedEncoding": messageDelimited(field)}
	case protoreflect.EnumKind:
		return map[string]any{"kind": "enum", "typeName": string(field.Enum().FullName())}
	default:
		shape := map[string]any{"kind": "scalar", "scalar": scalarName(field.Kind()), "longAsString": false}
		if includeLongAsString {
			if options, ok := field.Options().(*descriptorpb.FieldOptions); ok {
				shape["longAsString"] = options.GetJstype() == descriptorpb.FieldOptions_JS_STRING
			}
		}
		return shape
	}
}

func projectDefault(field protoreflect.FieldDescriptor, raw *descriptorpb.FieldDescriptorProto) any {
	if field.Kind() == protoreflect.EnumKind {
		value := field.DefaultEnumValue()
		if value == nil {
			return map[string]any{"type": "enum", "value": int32(0)}
		}
		return map[string]any{"type": "enum", "value": int32(value.Number())}
	}
	value := field.Default()
	switch field.Kind() {
	case protoreflect.BoolKind:
		return map[string]any{"type": "bool", "value": value.Bool()}
	case protoreflect.StringKind:
		return map[string]any{"type": "string", "value": value.String()}
	case protoreflect.BytesKind:
		return map[string]any{"type": "bytes", "value": base64.StdEncoding.EncodeToString(value.Bytes())}
	case protoreflect.FloatKind, protoreflect.DoubleKind:
		// Protobuf-ES 2.15 parses descriptor default strings with parseFloat for
		// both FLOAT and DOUBLE. Keep the resulting float64 instead of rounding
		// FLOAT defaults through Go's float32 reflection value.
		text := "0"
		if raw != nil && raw.DefaultValue != nil {
			text = raw.GetDefaultValue()
		}
		switch text {
		case "inf":
			return map[string]any{"type": "number", "value": "Infinity"}
		case "-inf":
			return map[string]any{"type": "number", "value": "-Infinity"}
		case "nan":
			return map[string]any{"type": "number", "value": "NaN"}
		}
		number, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return map[string]any{"type": "number", "value": "NaN"}
		}
		if math.IsNaN(number) {
			return map[string]any{"type": "number", "value": "NaN"}
		}
		if math.IsInf(number, 1) {
			return map[string]any{"type": "number", "value": "Infinity"}
		}
		if math.IsInf(number, -1) {
			return map[string]any{"type": "number", "value": "-Infinity"}
		}
		if number == 0 && math.Signbit(number) {
			return map[string]any{"type": "number", "value": "-0"}
		}
		return map[string]any{"type": "number", "value": number}
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind, protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		if field.Kind() == protoreflect.Uint32Kind || field.Kind() == protoreflect.Fixed32Kind {
			return map[string]any{"type": "integer", "value": strconv.FormatUint(uint64(value.Uint()), 10)}
		}
		return map[string]any{"type": "integer", "value": strconv.FormatInt(value.Int(), 10)}
	default:
		if field.Kind() == protoreflect.Uint64Kind || field.Kind() == protoreflect.Fixed64Kind {
			return map[string]any{"type": "integer", "value": strconv.FormatUint(value.Uint(), 10)}
		}
		return map[string]any{"type": "integer", "value": strconv.FormatInt(value.Int(), 10)}
	}
}

func fieldPresence(field protoreflect.FieldDescriptor) string {
	if field.Cardinality() == protoreflect.Required {
		return "LEGACY_REQUIRED"
	}
	if field.IsList() || field.IsMap() {
		return "IMPLICIT"
	}
	if field.HasPresence() {
		return "EXPLICIT"
	}
	return "IMPLICIT"
}

func utf8Validated(field protoreflect.FieldDescriptor) bool {
	for desc := protoreflect.Descriptor(field); desc != nil; desc = desc.Parent() {
		if features := descriptorFeatures(desc); features != nil && features.Utf8Validation != nil && features.GetUtf8Validation() != descriptorpb.FeatureSet_UTF8_VALIDATION_UNKNOWN {
			return features.GetUtf8Validation() == descriptorpb.FeatureSet_VERIFY
		}
	}
	return field.Syntax() == protoreflect.Proto3
}

func enumIsOpen(desc protoreflect.EnumDescriptor) bool {
	for owner := protoreflect.Descriptor(desc); owner != nil; owner = owner.Parent() {
		if features := descriptorFeatures(owner); features != nil && features.EnumType != nil && features.GetEnumType() != descriptorpb.FeatureSet_ENUM_TYPE_UNKNOWN {
			return features.GetEnumType() == descriptorpb.FeatureSet_OPEN
		}
	}
	return desc.Syntax() == protoreflect.Proto3
}

func messageDelimited(field protoreflect.FieldDescriptor) bool {
	for owner := protoreflect.Descriptor(field); owner != nil; owner = owner.Parent() {
		if features := descriptorFeatures(owner); features != nil && features.MessageEncoding != nil && features.GetMessageEncoding() != descriptorpb.FeatureSet_MESSAGE_ENCODING_UNKNOWN {
			return features.GetMessageEncoding() == descriptorpb.FeatureSet_DELIMITED
		}
	}
	return false
}

func descriptorFeatures(desc protoreflect.Descriptor) *descriptorpb.FeatureSet {
	switch options := desc.Options().(type) {
	case *descriptorpb.FileOptions:
		return options.GetFeatures()
	case *descriptorpb.MessageOptions:
		return options.GetFeatures()
	case *descriptorpb.EnumOptions:
		return options.GetFeatures()
	case *descriptorpb.FieldOptions:
		return options.GetFeatures()
	case *descriptorpb.ServiceOptions:
		return options.GetFeatures()
	case *descriptorpb.MethodOptions:
		return options.GetFeatures()
	default:
		return nil
	}
}

func methodIdempotency(method protoreflect.MethodDescriptor) descriptorpb.MethodOptions_IdempotencyLevel {
	if options, ok := method.Options().(*descriptorpb.MethodOptions); ok {
		return options.GetIdempotencyLevel()
	}
	return descriptorpb.MethodOptions_IDEMPOTENCY_UNKNOWN
}

func isSyntheticOneof(field protoreflect.FieldDescriptor) bool {
	oneof := field.ContainingOneof()
	return oneof != nil && field.HasOptionalKeyword() && oneof.Fields().Len() == 1
}

func scalarName(kind protoreflect.Kind) string { return strings.ToUpper(kind.String()) }

func sortedMessageNames(messages map[protoreflect.FullName]protoreflect.MessageDescriptor) []protoreflect.FullName {
	names := make([]protoreflect.FullName, 0, len(messages))
	for name := range messages {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return names[i] < names[j] })
	return names
}

func sortedEnumNames(enums map[protoreflect.FullName]protoreflect.EnumDescriptor) []protoreflect.FullName {
	names := make([]protoreflect.FullName, 0, len(enums))
	for name := range enums {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return names[i] < names[j] })
	return names
}

func protoCamelCase(name string) string {
	var builder strings.Builder
	capNext := false
	for _, char := range name {
		switch {
		case char == '_':
			capNext = true
		case char >= '0' && char <= '9':
			builder.WriteRune(char)
			capNext = false
		default:
			if capNext {
				char = []rune(strings.ToUpper(string(char)))[0]
				capNext = false
			}
			builder.WriteRune(char)
		}
	}
	return builder.String()
}

func safeObjectProperty(name string) string {
	switch name {
	case "constructor", "toString", "toJSON", "valueOf":
		return name + "$"
	default:
		return name
	}
}

func lowerInitial(name protoreflect.Name) string {
	if name == "" {
		return ""
	}
	return strings.ToLower(string(name[:1])) + string(name[1:])
}

func enumSharedPrefix(enumeration protoreflect.EnumDescriptor) string {
	values := enumeration.Values()
	if values.Len() == 0 {
		return ""
	}
	prefix := camelToSnakeCase(string(enumeration.Name())) + "_"
	lowerPrefix := strings.ToLower(prefix)
	for i := 0; i < values.Len(); i++ {
		name := string(values.Get(i).Name())
		if !strings.HasPrefix(strings.ToLower(name), lowerPrefix) {
			return ""
		}
		suffix := name[len(prefix):]
		if suffix == "" || suffix[0] >= '0' && suffix[0] <= '9' {
			return ""
		}
	}
	return prefix
}

func enumValueLocalName(prefix string, valueName protoreflect.Name) string {
	if prefix == "" || !strings.HasPrefix(strings.ToLower(string(valueName)), strings.ToLower(prefix)) {
		return string(valueName)
	}
	suffix := string(valueName[len(prefix):])
	return suffix
}

func camelToSnakeCase(name string) string {
	var builder strings.Builder
	for i, char := range name {
		if i > 0 && char >= 'A' && char <= 'Z' {
			builder.WriteByte('_')
		}
		if char >= 'A' && char <= 'Z' {
			char += 'a' - 'A'
		}
		builder.WriteRune(char)
	}
	return builder.String()
}
