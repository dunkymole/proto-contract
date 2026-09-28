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
	Name   string  `json:"name"`
	Fields []Field `json:"fields"`
}
type Field struct {
	Number         int32  `json:"number"`
	Name           string `json:"name"`
	Type           string `json:"type"`
	TypeName       string `json:"type_name,omitempty"`
	Label          string `json:"label"`
	Oneof          string `json:"oneof,omitempty"`
	Proto3Optional bool   `json:"proto3_optional,omitempty"`
}
type Enum struct {
	Name   string      `json:"name"`
	Values []EnumValue `json:"values"`
}
type EnumValue struct {
	Name   string `json:"name"`
	Number int32  `json:"number"`
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
}

func Build(set *descriptorpb.FileDescriptorSet, serviceName, api, version string) (*Snapshot, error) {
	idx := index{map[string]*descriptorpb.DescriptorProto{}, map[string]*descriptorpb.EnumDescriptorProto{}, map[string]*descriptorpb.ServiceDescriptorProto{}}
	for _, f := range set.File {
		pkg := f.GetPackage()
		for _, s := range f.Service {
			idx.services[join(pkg, s.GetName())] = s
		}
		for _, m := range f.MessageType {
			addMessage(&idx, pkg, m)
		}
		for _, e := range f.EnumType {
			idx.enums[join(pkg, e.GetName())] = e
		}
	}
	sd := idx.services[serviceName]
	if sd == nil {
		return nil, fmt.Errorf("service %q not found", serviceName)
	}
	s := &Snapshot{Format: 1, API: api, Version: version, Service: Service{Name: serviceName}, Messages: []Message{}, Enums: []Enum{}}
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
		m := Message{Name: name}
		for _, f := range md.Field {
			oneof := ""
			if f.OneofIndex != nil && int(f.GetOneofIndex()) < len(md.OneofDecl) {
				oneof = md.OneofDecl[f.GetOneofIndex()].GetName()
			}
			m.Fields = append(m.Fields, Field{f.GetNumber(), f.GetName(), f.GetType().String(), strings.TrimPrefix(f.GetTypeName(), "."), f.GetLabel().String(), oneof, f.GetProto3Optional()})
		}
		sort.Slice(m.Fields, func(i, j int) bool { return m.Fields[i].Number < m.Fields[j].Number })
		s.Messages = append(s.Messages, m)
	}
	for name := range neededEnums {
		ed := idx.enums[name]
		if ed == nil {
			return nil, fmt.Errorf("enum %q not found", name)
		}
		e := Enum{Name: name}
		for _, v := range ed.Value {
			e.Values = append(e.Values, EnumValue{v.GetName(), v.GetNumber()})
		}
		sort.Slice(e.Values, func(i, j int) bool {
			if e.Values[i].Number == e.Values[j].Number {
				return e.Values[i].Name < e.Values[j].Name
			}
			return e.Values[i].Number < e.Values[j].Number
		})
		s.Enums = append(s.Enums, e)
	}
	sort.Slice(s.Messages, func(i, j int) bool { return s.Messages[i].Name < s.Messages[j].Name })
	sort.Slice(s.Enums, func(i, j int) bool { return s.Enums[i].Name < s.Enums[j].Name })
	digest, err := calculateDigest(s)
	if err != nil {
		return nil, err
	}
	s.Digest = digest
	return s, nil
}

func addMessage(idx *index, prefix string, m *descriptorpb.DescriptorProto) {
	name := join(prefix, m.GetName())
	idx.messages[name] = m
	for _, n := range m.NestedType {
		addMessage(idx, name, n)
	}
	for _, e := range m.EnumType {
		idx.enums[join(name, e.GetName())] = e
	}
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
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	b, e := json.MarshalIndent(s, "", "  ")
	if e != nil {
		return e
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0644)
}
func Read(path string) (*Snapshot, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	s := new(Snapshot)
	if e = json.Unmarshal(b, s); e != nil {
		return nil, e
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
		compareNamed(a.Values, b.Values, func(v EnumValue) string { return fmt.Sprint(v.Number) }, func(x, y EnumValue) bool { return x == y }, "enum value in "+n, mark)
	}
	for n := range ce {
		if _, ok := oe[n]; !ok {
			mark(Minor, "enum added: "+n)
		}
	}
	sort.Strings(changes)
	return bump, changes
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
			mark(Minor, fmt.Sprintf("field added: %s.%s (%d)", b.Name, x.Name, n))
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
	p := strings.Split(v, ".")
	if len(p) != 3 {
		return "", fmt.Errorf("version must be MAJOR.MINOR.PATCH")
	}
	x := make([]int, 3)
	for i := range p {
		n, e := strconv.Atoi(p[i])
		if e != nil || n < 0 {
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
	return fmt.Sprintf("%d.%d.%d", x[0], x[1], x[2]), nil
}
