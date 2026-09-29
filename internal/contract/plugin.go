package contract

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

const maxBindingsSize = 1 << 20

// ServiceBinding associates one service in the CodeGeneratorRequest with its
// lock and generated output. Paths in this file are intentionally explicit so
// a protoc invocation cannot accidentally bind the wrong service or package.
type ServiceBinding struct {
	Service       string `json:"service"`
	Lock          string `json:"lock"`
	Output        string `json:"output"`
	Package       string `json:"package,omitempty"`
	ServiceImport string `json:"service_import,omitempty"`
	ServiceExport string `json:"service_export,omitempty"`
}

type bindingFile struct {
	Bindings []ServiceBinding `json:"bindings"`
}

// GeneratePluginResponse validates each lock against the exact descriptors
// supplied to protoc and returns deterministic generated files. bindingsPath
// is resolved by the plugin process; lock paths are relative to that file.
func GeneratePluginResponse(req *pluginpb.CodeGeneratorRequest, language, bindingsPath string) (*pluginpb.CodeGeneratorResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("missing CodeGeneratorRequest")
	}
	bindings, err := readBindings(bindingsPath)
	if err != nil {
		return nil, err
	}
	if len(bindings) == 0 {
		return nil, fmt.Errorf("bindings file must contain at least one service binding")
	}
	sort.Slice(bindings, func(i, j int) bool { return bindings[i].Service < bindings[j].Service })
	set := &descriptorpb.FileDescriptorSet{File: req.ProtoFile}
	files, err := protodesc.NewFiles(set)
	if err != nil {
		return nil, fmt.Errorf("invalid protoc descriptors: %w", err)
	}
	requested := make(map[string]bool, len(req.FileToGenerate))
	for _, name := range req.FileToGenerate {
		requested[name] = true
	}
	seenServices, seenOutputs := map[string]bool{}, map[string]bool{}
	response := &pluginpb.CodeGeneratorResponse{}
	for _, binding := range bindings {
		if err := validateBinding(binding, language); err != nil {
			return nil, fmt.Errorf("service binding %q: %w", binding.Service, err)
		}
		if seenServices[binding.Service] {
			return nil, fmt.Errorf("duplicate service binding %q", binding.Service)
		}
		seenServices[binding.Service] = true
		if seenOutputs[binding.Output] {
			return nil, fmt.Errorf("duplicate output path %q", binding.Output)
		}
		seenOutputs[binding.Output] = true

		descriptor, err := files.FindDescriptorByName(protoreflect.FullName(binding.Service))
		if err != nil {
			return nil, fmt.Errorf("service %q is absent from the protoc descriptor request", binding.Service)
		}
		service, ok := descriptor.(protoreflect.ServiceDescriptor)
		if !ok {
			return nil, fmt.Errorf("descriptor %q is not a service", binding.Service)
		}
		if !requested[service.ParentFile().Path()] {
			return nil, fmt.Errorf("service %q is not declared by a file_to_generate entry", binding.Service)
		}
		lockPath := binding.Lock
		if !filepath.IsAbs(lockPath) {
			lockPath = filepath.Join(filepath.Dir(bindingsPath), lockPath)
		}
		lock, err := Read(lockPath)
		if err != nil {
			return nil, fmt.Errorf("read lock for %s: %w", binding.Service, err)
		}
		if err := ValidateSnapshotAgainst(lock, set, binding.Service); err != nil {
			return nil, fmt.Errorf("validate lock for %s: %w", binding.Service, err)
		}

		var generated []byte
		if language == "typescript" {
			generated, err = TypeScriptBinding(lock, set, binding.ServiceImport, binding.ServiceExport)
			if err != nil {
				return nil, fmt.Errorf("generate TypeScript binding for %s: %w", binding.Service, err)
			}
		} else {
			generated, err = Generate(lock, language, binding.Package)
			if err != nil {
				return nil, fmt.Errorf("generate %s binding for %s: %w", language, binding.Service, err)
			}
		}
		name := proto.String(binding.Output)
		content := string(generated)
		response.File = append(response.File, &pluginpb.CodeGeneratorResponse_File{Name: name, Content: &content})
	}
	return response, nil
}

// ValidateSnapshotAgainst checks a lock against the exact imported descriptor
// set supplied to a plugin or standalone generation invocation.
func ValidateSnapshotAgainst(snapshot *Snapshot, set *descriptorpb.FileDescriptorSet, serviceName string) error {
	if err := Validate(snapshot); err != nil {
		return err
	}
	serviceName = strings.TrimPrefix(serviceName, ".")
	if snapshot.Service.Name != serviceName {
		return fmt.Errorf("lock is for service %q, not %q", snapshot.Service.Name, serviceName)
	}
	current, err := Build(set, serviceName, snapshot.API, snapshot.Version)
	if err != nil {
		return err
	}
	if current.Digest != snapshot.Digest {
		return fmt.Errorf("lock is stale for the exact protoc descriptors (lock %s, descriptors %s)", snapshot.Digest, current.Digest)
	}
	return nil
}

func readBindings(path string) ([]ServiceBinding, error) {
	if path == "" {
		return nil, fmt.Errorf("bindings=PATH is required")
	}
	data, err := readBoundedFile(path)
	if err != nil {
		return nil, fmt.Errorf("read bindings file: %w", err)
	}
	if len(data) > maxBindingsSize {
		return nil, fmt.Errorf("bindings file exceeds %d bytes", maxBindingsSize)
	}
	var file bindingFile
	if err := decodeStrictJSON(data, &file); err != nil {
		return nil, fmt.Errorf("decode bindings file: %w", err)
	}
	return file.Bindings, nil
}

var protoServiceName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$`)
var tsIdentifier = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)

func validateBinding(binding ServiceBinding, language string) error {
	if !protoServiceName.MatchString(binding.Service) {
		return fmt.Errorf("invalid protobuf service name")
	}
	if binding.Lock == "" {
		return fmt.Errorf("lock is required")
	}
	if !safeOutputPath(binding.Output) {
		return fmt.Errorf("output must be a relative path without '..' components")
	}
	if language != "typescript" {
		if binding.ServiceImport != "" || binding.ServiceExport != "" {
			return fmt.Errorf("service_import and service_export are only valid for TypeScript")
		}
		return nil
	}
	if binding.Package != "" {
		return fmt.Errorf("package is not used for TypeScript")
	}
	if binding.ServiceImport == "" || binding.ServiceExport == "" {
		return fmt.Errorf("service_import and service_export are required for TypeScript")
	}
	if strings.ContainsAny(binding.ServiceImport, "\r\n\x00") || strings.Contains(binding.ServiceImport, "\\") {
		return fmt.Errorf("service_import must be a module specifier using forward slashes")
	}
	if strings.HasPrefix(binding.ServiceImport, "/") || regexp.MustCompile(`^[A-Za-z]:`).MatchString(binding.ServiceImport) {
		return fmt.Errorf("service_import must be a package specifier or relative module path")
	}
	if !tsIdentifier.MatchString(binding.ServiceExport) {
		return fmt.Errorf("service_export must be a TypeScript identifier")
	}
	return nil
}

func safeOutputPath(path string) bool {
	if path == "" || strings.ContainsAny(path, "\\:\r\n\x00") || filepath.IsAbs(path) || strings.HasPrefix(path, "/") {
		return false
	}
	for _, component := range strings.Split(path, "/") {
		if component == ".." || component == "" {
			return false
		}
	}
	return filepath.ToSlash(filepath.Clean(path)) == path
}

// TypeScriptBinding validates the lock against the exact descriptor set and
// emits a strict grpc-bridge artifact bound to its Protobuf-ES service graph.
func TypeScriptBinding(s *Snapshot, set *descriptorpb.FileDescriptorSet, serviceImport, serviceExport string) ([]byte, error) {
	if err := validateGeneration(s); err != nil {
		return nil, err
	}
	if err := ValidateSnapshotAgainst(s, set, s.Service.Name); err != nil {
		return nil, fmt.Errorf("descriptor/lock validation failed: %w", err)
	}
	if serviceImport == "" || serviceExport == "" || !tsIdentifier.MatchString(serviceExport) {
		return nil, fmt.Errorf("a service import and valid service export are required")
	}
	graph, err := RuntimeGraph(set, s.Service.Name)
	if err != nil {
		return nil, err
	}
	graphJSON, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		return nil, err
	}
	quote := func(value string) string { b, _ := json.Marshal(value); return string(b) }
	fingerprint := strings.TrimPrefix(s.Digest, "sha256:")
	var out strings.Builder
	fmt.Fprintf(&out, "// Code generated by proto-contract from validated protoc descriptors and a contract lock. DO NOT EDIT.\n")
	fmt.Fprintf(&out, "import { defineContract } from \"@dunkymole/grpc-bridge/codegen\";\n")
	fmt.Fprintf(&out, "import type { RuntimeGraph } from \"@dunkymole/grpc-bridge/codegen\";\n")
	fmt.Fprintf(&out, "import { %s as serviceDescriptor } from %s;\n\n", serviceExport, quote(serviceImport))
	fmt.Fprintf(&out, "export const runtimeGraph: RuntimeGraph = %s;\n\n", graphJSON)
	fmt.Fprintf(&out, "export const contract = defineContract({\n  service: serviceDescriptor,\n  api: %s,\n  version: %s,\n  fingerprint: %s,\n  graph: runtimeGraph,\n});\n", quote(s.API), quote(s.Version), quote(fingerprint))
	return []byte(out.String()), nil
}
