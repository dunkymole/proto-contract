package contract

import (
	"fmt"
	"go/format"
	"go/token"
	"regexp"
	"strconv"
	"strings"
)

// GenerateBound adds protobuf-aware helpers to a lock-bound native module.
// Explicit language bindings avoid guessing symbols from protobuf identifiers.
func GenerateBound(s *Snapshot, language, packageName, serviceImport, serviceExport, target string) ([]byte, error) {
	if err := validateNativeBinding(language, serviceImport, serviceExport, target); err != nil {
		return nil, err
	}
	source, err := Generate(s, language, packageName)
	if err != nil {
		return nil, err
	}
	if serviceImport == "" && serviceExport == "" {
		return source, nil
	}
	if target == "" {
		target = "both"
	}
	out := string(source)
	switch language {
	case "go":
		out = strings.Replace(out, "import (", "import (\n pb "+strconv.Quote(serviceImport), 1)
		out = strings.Replace(out, "func NewClient[T any]", "func NewClientWithFactory[T any]", 1)
		out = strings.Replace(out, "func RegisterServer(", "func RegisterServerWithCallback(", 1)
		out += fmt.Sprintf(`
func NewClient(conn grpc.ClientConnInterface) pb.%[1]sClient {
 return pb.New%[1]sClient(Client().Bind(conn))
}
func RegisterServer(builder *protocontract.ServerBuilder, implementation pb.%[1]sServer) error {
 return builder.Register(Server(), func(r grpc.ServiceRegistrar) { pb.Register%[1]sServer(r, implementation) })
}
`, serviceExport)
		return format.Source([]byte(out))
	case "python":
		out = strings.Replace(out, "import grpc as _grpc", "import grpc as _grpc\nimport "+serviceImport+" as _pb", 1)
		out = strings.Replace(out, "def client(channel, stub_factory):", "def client_with_factory(channel, stub_factory):", 1)
		out = strings.Replace(out, "def add_to_server(builder, implementation, register):", "def add_to_server_with_callback(builder, implementation, register):", 1)
		out += fmt.Sprintf(`
def client(channel) -> _pb.%[1]sStub:
    return client_with_factory(channel, _pb.%[1]sStub)

def add_to_server(builder, implementation: _pb.%[1]sServicer):
    return add_to_server_with_callback(builder, implementation, _pb.add_%[1]sServicer_to_server)
`, serviceExport)
	case "java":
		qualified := serviceImport + "." + serviceExport + "Grpc"
		out = strings.Replace(out, "ContractServerBuilder builder, BindableService implementation", "ContractServerBuilder builder, "+qualified+"."+serviceExport+"ImplBase implementation", 1)
		helpers := fmt.Sprintf(`
  public static %[1]s.%[2]sBlockingStub blockingClient(Channel channel) {
    return client(channel, %[1]s::newBlockingStub);
  }
  public static %[1]s.%[2]sStub client(Channel channel) {
    return client(channel, %[1]s::newStub);
  }
  public static %[1]s.%[2]sFutureStub futureClient(Channel channel) {
    return client(channel, %[1]s::newFutureStub);
  }
`, qualified, serviceExport)
		pos := strings.LastIndex(out, "}")
		out = out[:pos] + helpers + out[pos:]
	case "dotnet":
		qualified := "global::" + serviceExport
		if serviceImport != "" {
			qualified = "global::" + serviceImport + "." + serviceExport
		}
		var helpers string
		if target != "server" {
			helpers += fmt.Sprintf(`
    public static %[1]s.%[2]sClient CreateClient(global::Grpc.Core.ChannelBase channel) =>
        CreateClient(channel, invoker => new %[1]s.%[2]sClient(invoker));
`, qualified, serviceExport)
		}
		if target != "client" {
			helpers += fmt.Sprintf(`
    public static global::Microsoft.AspNetCore.Builder.GrpcServiceEndpointConventionBuilder MapService<TService>(
        global::Microsoft.AspNetCore.Routing.IEndpointRouteBuilder app) where TService : %[1]s.%[2]sBase =>
        global::ProtoContract.ContractGrpcExtensions.MapContractGrpcService<TService>(app, new ServerInterceptor());
`, qualified, serviceExport)
		}
		pos := strings.LastIndex(out, "}")
		out = out[:pos] + helpers + out[pos:]
	}
	return []byte(out), nil
}

func validateNativeBinding(language, serviceImport, serviceExport, target string) error {
	if target != "" && target != "both" && target != "client" && target != "server" {
		return fmt.Errorf("target must be client, server or both")
	}
	if target != "" && language != "dotnet" {
		return fmt.Errorf("target is only supported for .NET")
	}
	if serviceImport == "" && serviceExport == "" {
		if target != "" {
			return fmt.Errorf("target requires a protobuf binding")
		}
		return nil
	}
	if (serviceImport == "" && language != "dotnet") || serviceExport == "" {
		return fmt.Errorf("service_import and service_export must be supplied together")
	}
	if !regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(serviceExport) {
		return fmt.Errorf("service_export must be a language service identifier")
	}
	switch language {
	case "go":
		if !token.IsIdentifier(serviceExport) || token.Lookup(serviceExport).IsKeyword() {
			return fmt.Errorf("invalid Go service type")
		}
		if !regexp.MustCompile(`^[A-Za-z0-9._~/-]+$`).MatchString(serviceImport) || strings.HasPrefix(serviceImport, "/") || strings.Contains(serviceImport, "//") {
			return fmt.Errorf("invalid Go protobuf import path")
		}
		for _, part := range strings.Split(serviceImport, "/") {
			if part == "." || part == ".." {
				return fmt.Errorf("invalid Go protobuf import path")
			}
		}
	case "python":
		if !protoServiceName.MatchString(serviceImport) {
			return fmt.Errorf("service_import must be a Python module name")
		}
		for _, part := range strings.Split(serviceImport+"."+serviceExport, ".") {
			if strings.Contains(" False None True and as assert async await break class continue def del elif else except finally for from global if import in is lambda nonlocal not or pass raise return try while with yield ", " "+part+" ") {
				return fmt.Errorf("invalid Python identifier")
			}
		}
	case "java", "dotnet":
		qualified := serviceExport
		if language == "java" {
			qualified += "Grpc"
		}
		if serviceImport != "" {
			qualified = serviceImport + "." + qualified
		}
		if err := validateNativeQualified(qualified, language); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported native binding language %q", language)
	}
	return nil
}

// Native bindings follow their own language's rules. The conservative shared
// subset used for generated contract package names would reject valid C# types
// such as "package" and "_", and valid Java package segments such as "namespace".
func validateNativeQualified(name, language string) error {
	keywords := " abstract assert boolean break byte case catch char class const continue default do double else enum extends final finally float for goto if implements import instanceof int interface long native new package private protected public return short static strictfp super switch synchronized this throw throws transient try void volatile while true false null _ "
	if language == "dotnet" {
		keywords = " abstract as base bool break byte case catch char checked class const continue decimal default delegate do double else enum event explicit extern false finally fixed float for foreach goto if implicit in int interface internal is lock long namespace new null object operator out override params private protected public readonly ref return sbyte sealed short sizeof stackalloc static string struct switch this throw true try typeof uint ulong unchecked unsafe ushort using virtual void volatile while "
	}
	for _, part := range strings.Split(name, ".") {
		if !regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(part) || strings.Contains(keywords, " "+part+" ") {
			return fmt.Errorf("invalid %s protobuf binding identifier %q", language, name)
		}
	}
	return nil
}
