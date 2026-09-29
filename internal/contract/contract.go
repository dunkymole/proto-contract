package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

type Snapshot struct {
	Format   int       `json:"format"`
	API      string    `json:"api"`
	Version  string    `json:"version"`
	Digest   string    `json:"digest"`
	Service  Service   `json:"service"`
	Messages []Message `json:"messages"`
	Enums    []Enum    `json:"enums"`
	Features []Feature `json:"features,omitempty"`
}
type Service struct {
	Name    string   `json:"name"`
	Methods []Method `json:"methods"`
}
type Method struct {
	Name            string `json:"name"`
	Input           string `json:"input"`
	Output          string `json:"output"`
	ClientStreaming bool   `json:"client_streaming"`
	ServerStreaming bool   `json:"server_streaming"`
}
type Message struct {
	Name     string  `json:"name"`
	MapEntry bool    `json:"map_entry,omitempty"`
	Syntax   string  `json:"syntax,omitempty"`
	Fields   []Field `json:"fields"`
}
type Field struct {
	Number         int32  `json:"number"`
	Name           string `json:"name"`
	Type           string `json:"type"`
	TypeName       string `json:"type_name,omitempty"`
	JSONName       string `json:"json_name,omitempty"`
	Label          string `json:"label"`
	Oneof          string `json:"oneof,omitempty"`
	Proto3Optional bool   `json:"proto3_optional,omitempty"`
	DefaultValue   string `json:"default_value,omitempty"`
	HasDefault     bool   `json:"has_default,omitempty"`
}
type Enum struct {
	Name        string      `json:"name"`
	DefaultName string      `json:"default_name,omitempty"`
	Syntax      string      `json:"syntax,omitempty"`
	Values      []EnumValue `json:"values"`
}
type Feature struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
type EnumValue struct {
	Name       string `json:"name"`
	Number     int32  `json:"number"`
	AliasOrder int    `json:"alias_order,omitempty"`
}

func Compile(protoc, protoPath, protoFile, service, api, version string) (*Snapshot, error) {
	tmp, err := os.CreateTemp("", "proto-contract-*.pb")
	if err != nil {
		return nil, err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())
	cmd := exec.Command(protoc, "--proto_path="+protoPath, "--include_imports", "--descriptor_set_out="+tmp.Name(), protoFile)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("protoc failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	b, err := os.ReadFile(tmp.Name())
	if err != nil {
		return nil, err
	}
	set := new(descriptorpb.FileDescriptorSet)
	if err := proto.Unmarshal(b, set); err != nil {
		return nil, err
	}
	return Build(set, strings.TrimPrefix(service, "."), api, version)
}

type index struct {
	messages map[string]*descriptorpb.DescriptorProto
	enums    map[string]*descriptorpb.EnumDescriptorProto
	services map[string]*descriptorpb.ServiceDescriptorProto
	files    map[string]*descriptorpb.FileDescriptorProto
}

func Build(set *descriptorpb.FileDescriptorSet, serviceName, api, version string) (*Snapshot, error) {
	idx := index{map[string]*descriptorpb.DescriptorProto{}, map[string]*descriptorpb.EnumDescriptorProto{}, map[string]*descriptorpb.ServiceDescriptorProto{}, map[string]*descriptorpb.FileDescriptorProto{}}
	for _, f := range set.File {
		pkg := f.GetPackage()
		for _, s := range f.Service {
			name := join(pkg, s.GetName())
			idx.services[name] = s
			idx.files[name] = f
		}
		for _, m := range f.MessageType {
			addMessage(&idx, pkg, m, f)
		}
		for _, e := range f.EnumType {
			name := join(pkg, e.GetName())
			idx.enums[name] = e
			idx.files[name] = f
		}
	}
	sd := idx.services[serviceName]
	if sd == nil {
		return nil, fmt.Errorf("service %q not found", serviceName)
	}
	usedFiles := map[*descriptorpb.FileDescriptorProto]bool{idx.files[serviceName]: true}
	s := &Snapshot{Format: 2, API: api, Version: version, Service: Service{Name: serviceName, Methods: []Method{}}, Messages: []Message{}, Enums: []Enum{}}
	addFeature := func(name string, message proto.Message) error {
		if message == nil {
			return nil
		}
		if len(message.ProtoReflect().GetUnknown()) != 0 {
			return fmt.Errorf("unsupported protobuf feature %q; custom or unrecognized descriptor options require a registered classifier", name)
		}
		if field := message.ProtoReflect().Descriptor().Fields().ByName("uninterpreted_option"); field != nil && message.ProtoReflect().Has(field) && message.ProtoReflect().Get(field).List().Len() != 0 {
			return fmt.Errorf("unsupported protobuf feature %q; uninterpreted descriptor options require a registered classifier", name)
		}
		if proto.Size(message) == 0 {
			return nil
		}
		b, err := (proto.MarshalOptions{Deterministic: true}).Marshal(message)
		if err != nil {
			return err
		}
		s.Features = append(s.Features, Feature{Name: name, Value: hex.EncodeToString(b)})
		return nil
	}
	if err := addFeature("service-options:"+serviceName, sd.GetOptions()); err != nil {
		return nil, err
	}
	for _, method := range sd.Method {
		if err := addFeature("method-options:"+serviceName+"."+method.GetName(), method.GetOptions()); err != nil {
			return nil, err
		}
	}
	neededMessages, neededEnums := map[string]bool{}, map[string]bool{}
	var visitMessage func(string)
	visitMessage = func(name string) {
		name = strings.TrimPrefix(name, ".")
		if neededMessages[name] {
			return
		}
		neededMessages[name] = true
		m := idx.messages[name]
		if m == nil {
			return
		}
		for _, f := range m.Field {
			tn := strings.TrimPrefix(f.GetTypeName(), ".")
			if f.GetType() == descriptorpb.FieldDescriptorProto_TYPE_MESSAGE {
				visitMessage(tn)
			}
			if f.GetType() == descriptorpb.FieldDescriptorProto_TYPE_ENUM {
				neededEnums[tn] = true
			}
		}
	}
	for _, m := range sd.Method {
		in, out := strings.TrimPrefix(m.GetInputType(), "."), strings.TrimPrefix(m.GetOutputType(), ".")
		s.Service.Methods = append(s.Service.Methods, Method{m.GetName(), in, out, m.GetClientStreaming(), m.GetServerStreaming()})
		visitMessage(in)
		visitMessage(out)
	}
	sort.Slice(s.Service.Methods, func(i, j int) bool { return s.Service.Methods[i].Name < s.Service.Methods[j].Name })
	for name := range neededMessages {
		md := idx.messages[name]
		if md == nil {
			return nil, fmt.Errorf("message %q not found", name)
		}
		usedFiles[idx.files[name]] = true
		if err := validateMessageSemantics(md, func(feature string) error {
			return fmt.Errorf("unsupported protobuf feature %q; remove it or extend the contract format and classifier", feature)
		}); err != nil {
			return nil, err
		}
		m := Message{Name: name, MapEntry: md.GetOptions().GetMapEntry(), Syntax: descriptorSyntax(idx.files[name]), Fields: []Field{}}
		if err := addFeature("message-options:"+name, md.GetOptions()); err != nil {
			return nil, err
		}
		metadata := proto.Clone(md).(*descriptorpb.DescriptorProto)
		metadata.Name, metadata.Field, metadata.NestedType, metadata.EnumType, metadata.OneofDecl = nil, nil, nil, nil, nil
		metadata.Options = nil
		sort.Slice(metadata.ReservedRange, func(i, j int) bool {
			if metadata.ReservedRange[i].GetStart() == metadata.ReservedRange[j].GetStart() {
				return metadata.ReservedRange[i].GetEnd() < metadata.ReservedRange[j].GetEnd()
			}
			return metadata.ReservedRange[i].GetStart() < metadata.ReservedRange[j].GetStart()
		})
		sort.Strings(metadata.ReservedName)
		if err := addFeature("message-declarations:"+name, metadata); err != nil {
			return nil, err
		}
		for _, f := range md.Field {
			if err := addFeature("field-options:"+name+"."+f.GetName(), f.GetOptions()); err != nil {
				return nil, err
			}
			oneof := ""
			if f.OneofIndex != nil && int(f.GetOneofIndex()) < len(md.OneofDecl) {
				oneof = md.OneofDecl[f.GetOneofIndex()].GetName()
			}
			field := Field{Number: f.GetNumber(), Name: f.GetName(), Type: f.GetType().String(), TypeName: strings.TrimPrefix(f.GetTypeName(), "."), JSONName: f.GetJsonName(), Label: f.GetLabel().String(), Oneof: oneof, Proto3Optional: f.GetProto3Optional()}
			if f.DefaultValue != nil {
				field.DefaultValue = f.GetDefaultValue()
				field.HasDefault = true
			}
			m.Fields = append(m.Fields, field)
		}
		for i, oneof := range md.OneofDecl {
			if err := addFeature(fmt.Sprintf("oneof-options:%s.%s", name, oneof.GetName()), oneof.GetOptions()); err != nil {
				return nil, err
			}
			_ = i
		}
		sort.Slice(m.Fields, func(i, j int) bool { return m.Fields[i].Number < m.Fields[j].Number })
		s.Messages = append(s.Messages, m)
	}
	for name := range neededEnums {
		ed := idx.enums[name]
		if ed == nil {
			return nil, fmt.Errorf("enum %q not found", name)
		}
		usedFiles[idx.files[name]] = true
		if err := validateEnumSemantics(ed); err != nil {
			return nil, err
		}
		e := Enum{Name: name, Syntax: descriptorSyntax(idx.files[name]), Values: []EnumValue{}}
		if len(ed.Value) > 0 {
			e.DefaultName = ed.Value[0].GetName()
		}
		if err := addFeature("enum-options:"+name, ed.GetOptions()); err != nil {
			return nil, err
		}
		metadata := proto.Clone(ed).(*descriptorpb.EnumDescriptorProto)
		metadata.Name, metadata.Value, metadata.Options = nil, nil, nil
		sort.Slice(metadata.ReservedRange, func(i, j int) bool {
			if metadata.ReservedRange[i].GetStart() == metadata.ReservedRange[j].GetStart() {
				return metadata.ReservedRange[i].GetEnd() < metadata.ReservedRange[j].GetEnd()
			}
			return metadata.ReservedRange[i].GetStart() < metadata.ReservedRange[j].GetStart()
		})
		sort.Strings(metadata.ReservedName)
		if err := addFeature("enum-declarations:"+name, metadata); err != nil {
			return nil, err
		}
		aliasOrders := map[int32]int{}
		for _, v := range ed.Value {
			if err := addFeature("enum-value-options:"+name+"."+v.GetName(), v.GetOptions()); err != nil {
				return nil, err
			}
			aliasOrder := aliasOrders[v.GetNumber()]
			aliasOrders[v.GetNumber()]++
			e.Values = append(e.Values, EnumValue{Name: v.GetName(), Number: v.GetNumber(), AliasOrder: aliasOrder})
		}
		sort.Slice(e.Values, func(i, j int) bool {
			if e.Values[i].Number == e.Values[j].Number {
				return e.Values[i].AliasOrder < e.Values[j].AliasOrder
			}
			return e.Values[i].Number < e.Values[j].Number
		})
		s.Enums = append(s.Enums, e)
	}
	for file := range usedFiles {
		if err := validateDescriptorFile(file); err != nil {
			return nil, err
		}
		if err := addFeature("file-options:"+file.GetName(), file.GetOptions()); err != nil {
			return nil, err
		}
	}
	for _, file := range set.File {
		for _, extension := range file.Extension {
			if neededMessages[strings.TrimPrefix(extension.GetExtendee(), ".")] || neededEnums[strings.TrimPrefix(extension.GetExtendee(), ".")] {
				return nil, fmt.Errorf("unsupported protobuf feature %q in %s; remove it or extend the contract format and classifier", "extensions targeting reachable declarations", file.GetName())
			}
		}
	}
	sort.Slice(s.Features, func(i, j int) bool { return s.Features[i].Name < s.Features[j].Name })
	sort.Slice(s.Messages, func(i, j int) bool { return s.Messages[i].Name < s.Messages[j].Name })
	sort.Slice(s.Enums, func(i, j int) bool { return s.Enums[i].Name < s.Enums[j].Name })
	digest, err := calculateDigest(s)
	if err != nil {
		return nil, err
	}
	s.Digest = digest
	if err := Validate(s); err != nil {
		return nil, err
	}
	return s, nil
}

// The classifier intentionally models only proto2 and proto3 descriptors and
// the declaration data stored in format 2 locks. Reject descriptor semantics
// that could affect generated or wire behavior instead of digesting an
// incomplete view of them.
func validateMessageSemantics(m *descriptorpb.DescriptorProto, unsupported func(string) error) error {
	if len(m.ExtensionRange) != 0 || len(m.Extension) != 0 {
		return unsupported("message extensions in " + m.GetName())
	}
	for _, f := range m.Field {
		if f.GetType() == descriptorpb.FieldDescriptorProto_TYPE_GROUP {
			return unsupported("group field " + m.GetName() + "." + f.GetName())
		}
	}
	return nil
}

func validateEnumSemantics(*descriptorpb.EnumDescriptorProto) error { return nil }

func addMessage(idx *index, prefix string, m *descriptorpb.DescriptorProto, file *descriptorpb.FileDescriptorProto) {
	name := join(prefix, m.GetName())
	idx.messages[name] = m
	idx.files[name] = file
	for _, n := range m.NestedType {
		addMessage(idx, name, n, file)
	}
	for _, e := range m.EnumType {
		enumName := join(name, e.GetName())
		idx.enums[enumName] = e
		idx.files[enumName] = file
	}
}

func validateDescriptorFile(file *descriptorpb.FileDescriptorProto) error {
	if file == nil {
		return fmt.Errorf("descriptor file for reachable declaration not found")
	}
	syntax := file.GetSyntax()
	if syntax == "" {
		syntax = "proto2"
	}
	if syntax != "proto2" && syntax != "proto3" || file.GetEdition() != descriptorpb.Edition_EDITION_UNKNOWN {
		return fmt.Errorf("unsupported protobuf feature %q in %s; remove it or extend the contract format and classifier", "syntax "+file.GetSyntax(), file.GetName())
	}
	return nil
}

func descriptorSyntax(file *descriptorpb.FileDescriptorProto) string {
	if file == nil || file.GetSyntax() == "" {
		return "proto2"
	}
	return file.GetSyntax()
}
func join(a, b string) string {
	if a == "" {
		return b
	}
	return a + "." + b
}
func calculateDigest(s *Snapshot) (string, error) {
	c := *s
	c.Version = ""
	c.Digest = ""
	b, e := json.Marshal(c)
	if e != nil {
		return "", e
	}
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:]), nil
}
func Write(path string, s *Snapshot) error {
	if err := Validate(s); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	b, e := json.MarshalIndent(s, "", "  ")
	if e != nil {
		return e
	}
	b = append(b, '\n')
	if len(b) > MaxLockBytes {
		return fmt.Errorf("lock exceeds maximum size of %d bytes", MaxLockBytes)
	}
	return os.WriteFile(path, b, 0644)
}
func Read(path string) (*Snapshot, error) {
	b, e := readBoundedFile(path)
	if e != nil {
		return nil, e
	}
	if len(b) > MaxLockBytes {
		return nil, fmt.Errorf("lock exceeds maximum size of %d bytes", MaxLockBytes)
	}
	s := new(Snapshot)
	if e = decodeStrictJSON(b, s); e != nil {
		return nil, e
	}
	if err := Validate(s); err != nil {
		return nil, fmt.Errorf("invalid lock %q: %w", path, err)
	}
	return s, nil
}

type Bump int

const (
	None Bump = iota
	Minor
	Major
)

func (b Bump) String() string { return [...]string{"none", "minor", "major"}[b] }

func ParseBump(value string) (Bump, error) {
	switch value {
	case "none":
		return None, nil
	case "minor":
		return Minor, nil
	case "major":
		return Major, nil
	default:
		return None, fmt.Errorf("bump must be none, minor, or major")
	}
}

func Compare(old, cur *Snapshot) (Bump, []string) {
	bump := None
	changes := []string{}
	mark := func(b Bump, msg string) {
		if b > bump {
			bump = b
		}
		changes = append(changes, b.String()+": "+msg)
	}
	if old.Service.Name != cur.Service.Name {
		mark(Major, "service changed")
	}
	compareNamed(old.Service.Methods, cur.Service.Methods, func(m Method) string { return m.Name }, func(a, b Method) bool { return a == b }, "method", mark)
	om := map[string]Message{}
	for _, m := range old.Messages {
		om[m.Name] = m
	}
	cm := map[string]Message{}
	for _, m := range cur.Messages {
		cm[m.Name] = m
	}
	for n, a := range om {
		b, ok := cm[n]
		if !ok {
			mark(Major, "message removed: "+n)
			continue
		}
		if a.MapEntry != b.MapEntry || a.Syntax != b.Syntax {
			mark(Major, "message semantics changed: "+n)
		}
		compareFields(a, b, mark)
	}
	for n := range cm {
		if _, ok := om[n]; !ok {
			mark(Minor, "message added: "+n)
		}
	}
	oe := map[string]Enum{}
	for _, e := range old.Enums {
		oe[e.Name] = e
	}
	ce := map[string]Enum{}
	for _, e := range cur.Enums {
		ce[e.Name] = e
	}
	for n, a := range oe {
		b, ok := ce[n]
		if !ok {
			mark(Major, "enum removed: "+n)
			continue
		}
		if a.Syntax != b.Syntax {
			mark(Major, "enum syntax changed: "+n)
		}
		if a.DefaultName != b.DefaultName {
			mark(Major, "enum default value changed: "+n)
		}
		oldValues := map[string]EnumValue{}
		newValues := map[string]EnumValue{}
		for _, value := range a.Values {
			oldValues[value.Name] = value
		}
		for _, value := range b.Values {
			newValues[value.Name] = value
		}
		for name, value := range oldValues {
			if current, ok := newValues[name]; !ok {
				mark(Major, "enum value in "+n+" removed: "+name)
			} else if current != value {
				mark(Major, "enum value in "+n+" changed: "+name)
			}
		}
		additionBump := Minor
		if enumUsedByRequiredField(old, n) || enumUsedByRequiredField(cur, n) {
			additionBump = Major
		}
		for name := range newValues {
			if _, ok := oldValues[name]; !ok {
				valueBump := additionBump
				for _, oldValue := range a.Values {
					if oldValue.Number == newValues[name].Number {
						valueBump = Major
						break
					}
				}
				mark(valueBump, "enum value in "+n+" added: "+name)
			}
		}
	}
	for n := range ce {
		if _, ok := oe[n]; !ok {
			mark(Minor, "enum added: "+n)
		}
	}
	oldFeatures, curFeatures := map[string]string{}, map[string]string{}
	for _, feature := range old.Features {
		oldFeatures[feature.Name] = feature.Value
	}
	for _, feature := range cur.Features {
		curFeatures[feature.Name] = feature.Value
	}
	for name, value := range oldFeatures {
		if current, ok := curFeatures[name]; !ok || current != value {
			mark(Major, "descriptor feature changed: "+name)
		}
	}
	for name := range curFeatures {
		if _, ok := oldFeatures[name]; !ok {
			mark(featureAdditionBump(old, cur, name), "descriptor feature added: "+name)
		}
	}
	sort.Strings(changes)
	return bump, changes
}

func featureAdditionBump(old, cur *Snapshot, feature string) Bump {
	parts := strings.SplitN(feature, ":", 2)
	if len(parts) != 2 {
		return Major
	}
	owner := parts[1]
	switch parts[0] {
	case "message-options", "message-declarations":
		for _, message := range old.Messages {
			if message.Name == owner {
				return Major
			}
		}
		return Minor
	case "field-options":
		messageName, fieldName, ok := splitMember(owner)
		if !ok {
			return Major
		}
		for _, message := range old.Messages {
			if message.Name != messageName {
				continue
			}
			for _, field := range message.Fields {
				if field.Name == fieldName {
					return Major
				}
			}
			return Minor
		}
		return Minor
	case "enum-options", "enum-declarations":
		for _, enum := range old.Enums {
			if enum.Name == owner {
				return Major
			}
		}
		return Minor
	case "enum-value-options":
		enumName, valueName, ok := splitMember(owner)
		if !ok {
			return Major
		}
		for _, enum := range old.Enums {
			if enum.Name != enumName {
				continue
			}
			for _, value := range enum.Values {
				if value.Name == valueName {
					return Major
				}
			}
			return Minor
		}
		return Minor
	case "method-options":
		for _, method := range old.Service.Methods {
			if owner == old.Service.Name+"."+method.Name {
				return Major
			}
		}
		return Minor
	case "oneof-options":
		messageName, oneofName, ok := splitMember(owner)
		if !ok {
			return Major
		}
		for _, message := range old.Messages {
			if message.Name != messageName {
				continue
			}
			for _, field := range message.Fields {
				if field.Oneof == oneofName {
					return Major
				}
			}
			return Minor
		}
		return Minor
	default:
		return Major
	}
}

func splitMember(name string) (string, string, bool) {
	i := strings.LastIndexByte(name, '.')
	if i <= 0 || i == len(name)-1 {
		return "", "", false
	}
	return name[:i], name[i+1:], true
}

func enumUsedByRequiredField(s *Snapshot, enum string) bool {
	for _, message := range s.Messages {
		for _, field := range message.Fields {
			if field.Type == descriptorpb.FieldDescriptorProto_TYPE_ENUM.String() && field.TypeName == enum && field.Label == descriptorpb.FieldDescriptorProto_LABEL_REQUIRED.String() {
				return true
			}
		}
	}
	return false
}

func compareFields(a, b Message, mark func(Bump, string)) {
	am := map[int32]Field{}
	for _, f := range a.Fields {
		am[f.Number] = f
	}
	bm := map[int32]Field{}
	for _, f := range b.Fields {
		bm[f.Number] = f
	}
	for n, x := range am {
		y, ok := bm[n]
		if !ok {
			mark(Major, fmt.Sprintf("field removed: %s.%s (%d)", a.Name, x.Name, n))
		} else if x != y {
			mark(Major, fmt.Sprintf("field changed: %s.%s (%d)", a.Name, x.Name, n))
		}
	}
	for n, x := range bm {
		if _, ok := am[n]; !ok {
			bump := Minor
			if x.Label == descriptorpb.FieldDescriptorProto_LABEL_REQUIRED.String() {
				bump = Major
			}
			mark(bump, fmt.Sprintf("field added: %s.%s (%d)", b.Name, x.Name, n))
		}
	}
}
func compareNamed[T any](a, b []T, key func(T) string, equal func(T, T) bool, label string, mark func(Bump, string)) {
	am := map[string]T{}
	for _, x := range a {
		am[key(x)] = x
	}
	bm := map[string]T{}
	for _, x := range b {
		bm[key(x)] = x
	}
	for n, x := range am {
		y, ok := bm[n]
		if !ok {
			mark(Major, label+" removed: "+n)
		} else if !equal(x, y) {
			mark(Major, label+" changed: "+n)
		}
	}
	for n := range bm {
		if _, ok := am[n]; !ok {
			mark(Minor, label+" added: "+n)
		}
	}
}
func NextVersion(v string, b Bump) (string, error) {
	if !canonicalNumber.MatchString(v) {
		return "", fmt.Errorf("version must be canonical MAJOR.MINOR.PATCH")
	}
	if b != None && b != Minor && b != Major {
		return "", fmt.Errorf("invalid semantic version bump")
	}
	p := strings.Split(v, ".")
	if len(p) != 3 {
		return "", fmt.Errorf("version must be MAJOR.MINOR.PATCH")
	}
	x := make([]int, 3)
	for i := range p {
		n, e := strconv.Atoi(p[i])
		if e != nil || n < 0 || n > 2147483647 || (len(p[i]) > 1 && p[i][0] == '0') {
			return "", fmt.Errorf("invalid version %q", v)
		}
		x[i] = n
	}
	if b == Major {
		x[0]++
		x[1] = 0
		x[2] = 0
	} else if b == Minor {
		x[1]++
		x[2] = 0
	}
	if x[0] > 2147483647 || x[1] > 2147483647 || x[2] > 2147483647 {
		return "", fmt.Errorf("version component exceeds maximum value in %q", v)
	}
	return fmt.Sprintf("%d.%d.%d", x[0], x[1], x[2]), nil
}
