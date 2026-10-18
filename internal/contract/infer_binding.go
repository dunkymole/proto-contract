package contract

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

// GenerateNativeFromDescriptors validates the lock and infers missing language
// bindings from the same descriptors used by protobuf generation.
func GenerateNativeFromDescriptors(s *Snapshot, set *descriptorpb.FileDescriptorSet, language, packageName, importOverride, exportOverride, target string) ([]byte, error) {
	if s == nil {
		return nil, fmt.Errorf("missing contract lock")
	}
	if err := ValidateSnapshotAgainst(s, set, s.Service.Name); err != nil {
		return nil, err
	}
	files, err := protodesc.NewFiles(set)
	if err != nil {
		return nil, err
	}
	descriptor, err := files.FindDescriptorByName(protoreflect.FullName(s.Service.Name))
	if err != nil {
		return nil, err
	}
	service, ok := descriptor.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil, fmt.Errorf("%s is not a protobuf service", s.Service.Name)
	}
	imp, exp, err := inferNativeBinding(service, language, importOverride, exportOverride)
	if err != nil {
		return nil, err
	}
	return GenerateBound(s, language, packageName, imp, exp, target)
}

func inferNativeBinding(service protoreflect.ServiceDescriptor, language, imp, exp string) (string, string, error) {
	file := service.ParentFile()
	opts, _ := file.Options().(*descriptorpb.FileOptions)
	if opts == nil {
		opts = &descriptorpb.FileOptions{}
	}
	if exp == "" {
		exp = string(service.Name()) // Java, Python and gRPC C# preserve the service name.
		if language == "go" {
			// Use the public protobuf generator API rather than copying GoCamelCase.
			// A minimal descriptor isolates symbol naming from unrelated import mappings.
			request := &pluginpb.CodeGeneratorRequest{FileToGenerate: []string{"naming.proto"}, ProtoFile: []*descriptorpb.FileDescriptorProto{{
				Name: proto.String("naming.proto"), Syntax: proto.String("proto3"),
				Options: &descriptorpb.FileOptions{GoPackage: proto.String("inference.invalid/naming")},
				Service: []*descriptorpb.ServiceDescriptorProto{{Name: proto.String(exp)}},
			}}}
			generated, err := (protogen.Options{}).New(request)
			if err != nil {
				return "", "", fmt.Errorf("infer Go service name: %w", err)
			}
			exp = generated.Files[0].Services[0].GoName
		}
	}
	if imp == "" {
		switch language {
		case "go":
			imp, _, _ = strings.Cut(opts.GetGoPackage(), ";")
			if imp == "" {
				return "", "", fmt.Errorf("%s: cannot infer Go import without go_package; set service_import (or --service-import) to the protobuf import path", file.Path())
			}
		case "python":
			path := file.Path()
			if !safeOutputPath(path) || !strings.HasSuffix(path, ".proto") {
				return "", "", fmt.Errorf("%s: cannot infer Python module; set service_import", path)
			}
			base := strings.TrimSuffix(path, ".proto")
			if strings.Contains(base, ".") {
				return "", "", fmt.Errorf("%s: dots inside Python output path components require an explicit service_import and matching relocated module", path)
			}
			imp = strings.ReplaceAll(strings.ReplaceAll(base, "-", "_"), "/", ".") + "_pb2_grpc"
		case "java":
			imp = string(file.Package())
			if opts.JavaPackage != nil {
				imp = opts.GetJavaPackage()
			}
			if imp == "" {
				return "", "", fmt.Errorf("%s: Java default-package services cannot be referenced from a named contract package; set java_package or service_import", file.Path())
			}
		case "dotnet":
			imp = csharpNamespace(string(file.Package()))
			if opts.CsharpNamespace != nil {
				imp = opts.GetCsharpNamespace()
			}
		default:
			return "", "", fmt.Errorf("unsupported native language %q", language)
		}
	}
	if err := validateNativeBinding(language, imp, exp, ""); err != nil {
		return "", "", fmt.Errorf("%s: inferred/overridden %s binding is unusable: %w; set service_import/service_export overrides where needed", file.Path(), language, err)
	}
	return imp, exp, nil
}

// C# protobuf's namespace conversion preserves case, capitalizes after digits
// and separators, and preserves dots. Service names themselves are not converted.
// See protobuf v30.2 compiler/csharp/names.cc (UnderscoresToCamelCase).
func csharpNamespace(name string) string {
	var out strings.Builder
	capitalize := true
	for i := 0; i < len(name); i++ {
		ch := name[i]
		switch {
		case ch >= 'a' && ch <= 'z':
			if capitalize {
				ch -= 'a' - 'A'
			}
			out.WriteByte(ch)
			capitalize = false
		case ch >= 'A' && ch <= 'Z':
			out.WriteByte(ch)
			capitalize = false
		case ch >= '0' && ch <= '9':
			out.WriteByte(ch)
			capitalize = true
		default:
			capitalize = true
			if ch == '.' {
				out.WriteByte(ch)
			}
		}
	}
	result := out.String()
	if len(result) > 0 && result[0] >= '0' && result[0] <= '9' && strings.HasPrefix(name, "_") {
		result = "_" + result
	}
	return result
}

// Partial overrides are validated before descriptor resolution. Full validation
// runs again after inference; neither omission means "use the generic API" here.
func validateNativeOverrides(language, imp, exp, target string) error {
	if imp == "" {
		imp = "example.pb"
	}
	if exp == "" {
		exp = "Service"
	}
	return validateNativeBinding(language, imp, exp, target)
}
