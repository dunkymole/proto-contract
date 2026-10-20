package contract

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

const customDefinitions = "custom-option-definitions:descriptors"

// Custom annotations are modeled as checked, opaque semantics. We cannot prove
// that a validation rule is relaxed or an HTTP route is compatible, so every
// addition, removal, value change, or definition change requires a major bump.
type optionCollector struct {
	set   *descriptorpb.FileDescriptorSet
	types *dynamicpb.Types
	files map[string]*descriptorpb.FileDescriptorProto
	used  map[string]bool
}

func (c *optionCollector) split(message proto.Message) (proto.Message, proto.Message, error) {
	standard := proto.Clone(message)
	if !standard.ProtoReflect().IsValid() {
		return standard, nil, nil
	}
	hasCustom := len(standard.ProtoReflect().GetUnknown()) != 0
	standard.ProtoReflect().Range(func(f protoreflect.FieldDescriptor, _ protoreflect.Value) bool {
		hasCustom = hasCustom || f.IsExtension()
		return true
	})
	if !hasCustom {
		return standard, nil, checkOptionMessage(standard.ProtoReflect(), 0)
	}
	if c.types == nil {
		files, err := protodesc.NewFiles(c.set)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve custom option definitions: %w", err)
		}
		if err := checkCustomExtensionNumbers(files); err != nil {
			return nil, nil, err
		}
		c.types = dynamicpb.NewTypes(files)
		c.files, c.used = map[string]*descriptorpb.FileDescriptorProto{}, map[string]bool{}
		for _, file := range c.set.File {
			c.files[file.GetName()] = file
		}
	}
	data, err := proto.Marshal(message)
	if err != nil {
		return nil, nil, err
	}
	proto.Reset(standard)
	if err := (proto.UnmarshalOptions{Resolver: c.types, RecursionLimit: 100}).Unmarshal(data, standard); err != nil {
		return nil, nil, fmt.Errorf("decode custom options: %w", err)
	}
	if err := checkOptionMessageUsing(standard.ProtoReflect(), 0, c.used); err != nil {
		return nil, nil, err
	}
	custom := proto.Clone(standard)
	var extensions []protoreflect.FieldDescriptor
	custom.ProtoReflect().Range(func(f protoreflect.FieldDescriptor, _ protoreflect.Value) bool {
		if f.IsExtension() {
			extensions = append(extensions, f)
			c.used[f.ParentFile().Path()] = true
		} else {
			custom.ProtoReflect().Clear(f)
		}
		return true
	})
	for _, field := range extensions {
		standard.ProtoReflect().Clear(field)
	}
	return standard, custom, nil
}

func (c *optionCollector) definitions() (*Feature, error) {
	if len(c.used) == 0 {
		return nil, nil
	}
	set := &descriptorpb.FileDescriptorSet{}
	seen := map[string]bool{}
	var visit func(string) error
	visit = func(name string) error {
		if seen[name] {
			return nil
		}
		seen[name] = true
		file := c.files[name]
		if file == nil {
			return fmt.Errorf("missing custom option dependency %q", name)
		}
		if err := validateCustomDefinitionFile(file); err != nil {
			return err
		}
		for _, dependency := range file.Dependency {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		copy := proto.Clone(file).(*descriptorpb.FileDescriptorProto)
		copy.SourceCodeInfo = nil
		set.File = append(set.File, copy)
		return nil
	}
	for name := range c.used {
		if err := visit(name); err != nil {
			return nil, err
		}
	}
	sort.Slice(set.File, func(i, j int) bool { return set.File[i].GetName() < set.File[j].GetName() })
	// Re-decode definitions too: nested custom options must resolve and canonicalize.
	data, err := proto.Marshal(set)
	if err != nil {
		return nil, err
	}
	if err := (proto.UnmarshalOptions{Resolver: c.types, RecursionLimit: 100}).Unmarshal(data, set); err != nil {
		return nil, err
	}
	if err := checkOptionMessage(set.ProtoReflect(), 0); err != nil {
		return nil, fmt.Errorf("custom option definitions: %w", err)
	}
	data, err = (proto.MarshalOptions{Deterministic: true}).Marshal(set)
	return &Feature{Name: customDefinitions, Value: hex.EncodeToString(data)}, err
}

func customTypes(snapshot *Snapshot) (*dynamicpb.Types, error) {
	for _, feature := range snapshot.Features {
		if feature.Name != customDefinitions {
			continue
		}
		data, err := canonicalFeatureBytes(feature)
		if err != nil {
			return nil, err
		}
		set := &descriptorpb.FileDescriptorSet{}
		if err := (proto.UnmarshalOptions{RecursionLimit: 100}).Unmarshal(data, set); err != nil {
			return nil, fmt.Errorf("invalid custom option definitions: %w", err)
		}
		if len(set.File) == 0 {
			return nil, fmt.Errorf("empty custom option definitions")
		}
		for i, file := range set.File {
			if file.SourceCodeInfo != nil || (i > 0 && set.File[i-1].GetName() >= file.GetName()) {
				return nil, fmt.Errorf("custom option definitions are not canonical")
			}
			if err := validateCustomDefinitionFile(file); err != nil {
				return nil, err
			}
		}
		files, err := protodesc.NewFiles(set)
		if err != nil {
			return nil, fmt.Errorf("invalid custom option definitions: %w", err)
		}
		if err := checkCustomExtensionNumbers(files); err != nil {
			return nil, err
		}
		types := dynamicpb.NewTypes(files)
		if err := (proto.UnmarshalOptions{Resolver: types, RecursionLimit: 100}).Unmarshal(data, set); err != nil {
			return nil, err
		}
		if err := checkOptionMessage(set.ProtoReflect(), 0); err != nil {
			return nil, err
		}
		canonical, err := (proto.MarshalOptions{Deterministic: true}).Marshal(set)
		if err != nil || !bytes.Equal(data, canonical) {
			return nil, fmt.Errorf("custom option definitions are not deterministically encoded")
		}
		return types, nil
	}
	return nil, nil
}

func validateCustomDefinitionFile(file *descriptorpb.FileDescriptorProto) error {
	if err := validateDescriptorFile(file); err != nil {
		return err
	}
	// Custom definition schemas currently have no resolved edition-feature
	// projection. Keep them closed even when service editions are supported.
	if file.GetSyntax() == "editions" {
		return fmt.Errorf("unsupported edition semantics in custom option definitions %s", file.GetName())
	}
	return nil
}

// NewFiles checks declaration names but permits two extensions with different
// names to claim the same extendee/number. A dynamic resolver would then choose
// one definition by iteration order. Reject ambiguity before decoding values,
// both when building a lock and when accepting an externally supplied lock.
func checkCustomExtensionNumbers(files *protoregistry.Files) error {
	type extensionKey struct {
		message protoreflect.FullName
		number  protoreflect.FieldNumber
	}
	seen := map[extensionKey]bool{}
	var checkExtensions func(protoreflect.ExtensionDescriptors) error
	checkExtensions = func(extensions protoreflect.ExtensionDescriptors) error {
		for i := 0; i < extensions.Len(); i++ {
			extension := extensions.Get(i)
			key := extensionKey{extension.ContainingMessage().FullName(), extension.Number()}
			if seen[key] {
				return fmt.Errorf("ambiguous custom option definitions: multiple extensions of %s use number %d", key.message, key.number)
			}
			seen[key] = true
		}
		return nil
	}
	var checkMessages func(protoreflect.MessageDescriptors) error
	checkMessages = func(messages protoreflect.MessageDescriptors) error {
		for i := 0; i < messages.Len(); i++ {
			message := messages.Get(i)
			if err := checkExtensions(message.Extensions()); err != nil {
				return err
			}
			if err := checkMessages(message.Messages()); err != nil {
				return err
			}
		}
		return nil
	}
	var result error
	files.RangeFiles(func(file protoreflect.FileDescriptor) bool {
		result = checkExtensions(file.Extensions())
		if result == nil {
			result = checkMessages(file.Messages())
		}
		return result == nil
	})
	return result
}

func canonicalFeatureBytes(feature Feature) ([]byte, error) {
	data, err := hex.DecodeString(feature.Value)
	if err != nil || hex.EncodeToString(data) != feature.Value {
		return nil, fmt.Errorf("descriptor feature %q is not canonical hex", feature.Name)
	}
	return data, nil
}

// Full source descriptors must match the actual protobuf-generator input after
// removing only source-retention data. An inconsistent request must not replace
// a changed wire shape or runtime annotation with a lock-matching source copy.
func checkSourceDescriptor(source, runtime *descriptorpb.FileDescriptorProto, types *dynamicpb.Types) error {
	decode := func(file *descriptorpb.FileDescriptorProto) (*descriptorpb.FileDescriptorProto, error) {
		data, err := proto.Marshal(file)
		if err != nil {
			return nil, err
		}
		copy := &descriptorpb.FileDescriptorProto{}
		if err := (proto.UnmarshalOptions{Resolver: types, RecursionLimit: 100}).Unmarshal(data, copy); err != nil {
			return nil, err
		}
		copy.SourceCodeInfo = nil
		return copy, nil
	}
	a, err := decode(source)
	if err != nil {
		return err
	}
	b, err := decode(runtime)
	if err != nil {
		return err
	}
	stripSourceOptions(a.ProtoReflect())
	if !proto.Equal(a, b) {
		return fmt.Errorf("full source and runtime descriptors differ beyond source-retention options")
	}
	return nil
}

func stripSourceOptions(message protoreflect.Message) {
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		options, _ := field.Options().(*descriptorpb.FieldOptions)
		if options.GetRetention() == descriptorpb.FieldOptions_RETENTION_SOURCE {
			message.Clear(field)
			return true
		}
		if field.Kind() != protoreflect.MessageKind {
			return true
		}
		if field.IsMap() {
			if field.MapValue().Kind() == protoreflect.MessageKind {
				value.Map().Range(func(_ protoreflect.MapKey, v protoreflect.Value) bool { stripSourceOptions(v.Message()); return true })
			}
		} else if field.IsList() {
			for i := 0; i < value.List().Len(); i++ {
				stripSourceOptions(value.List().Get(i).Message())
			}
		} else {
			child := value.Message()
			wasNonemptyOptions := strings.HasPrefix(string(child.Descriptor().FullName()), "google.protobuf.") && strings.HasSuffix(string(child.Descriptor().Name()), "Options") && proto.Size(child.Interface()) != 0
			stripSourceOptions(child)
			if wasNonemptyOptions && proto.Size(child.Interface()) == 0 {
				message.Clear(field)
			}
		}
		return true
	})
}

// Check recursively; checking only the outer options would silently retain
// unknown values in message-valued validation/HTTP annotations.
func checkOptionMessage(message protoreflect.Message, depth int) error {
	return checkOptionMessageUsing(message, depth, nil)
}

func checkOptionMessageUsing(message protoreflect.Message, depth int, used map[string]bool) error {
	if depth > 100 {
		return fmt.Errorf("custom option nesting exceeds 100")
	}
	if len(message.GetUnknown()) != 0 {
		return fmt.Errorf("unsupported unknown protobuf data in %s; include all custom option definitions", message.Descriptor().FullName())
	}
	var result error
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		if used != nil && field.IsExtension() {
			used[field.ParentFile().Path()] = true
		}
		if field.Name() == "uninterpreted_option" && strings.HasPrefix(string(field.ContainingMessage().FullName()), "google.protobuf.") {
			result = fmt.Errorf("unsupported uninterpreted descriptor option")
			return false
		}
		check := func(v protoreflect.Value) {
			if result != nil {
				return
			}
			switch field.Kind() {
			case protoreflect.GroupKind:
				result = fmt.Errorf("unsupported group in custom option %s", field.FullName())
			case protoreflect.MessageKind:
				result = checkOptionMessageUsing(v.Message(), depth+1, used)
			case protoreflect.EnumKind:
				if field.Enum().Values().ByNumber(v.Enum()) == nil {
					result = fmt.Errorf("unknown enum value in option %s", field.FullName())
				}
			}
		}
		if field.IsMap() {
			// Map values use the map-value descriptor rather than the entry message.
			value.Map().Range(func(_ protoreflect.MapKey, v protoreflect.Value) bool {
				if field.MapValue().Kind() == protoreflect.MessageKind {
					result = checkOptionMessageUsing(v.Message(), depth+1, used)
				} else if field.MapValue().Kind() == protoreflect.EnumKind && field.MapValue().Enum().Values().ByNumber(v.Enum()) == nil {
					result = fmt.Errorf("unknown enum map value in option %s", field.FullName())
				}
				return result == nil
			})
		} else if field.IsList() {
			for i := 0; i < value.List().Len(); i++ {
				check(value.List().Get(i))
			}
		} else {
			check(value)
		}
		return result == nil
	})
	return result
}
