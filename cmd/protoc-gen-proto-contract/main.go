// protoc-gen-proto-contract validates contract locks against the exact
// CodeGeneratorRequest descriptors and emits service-scoped runtime bindings.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/dunkymole/proto-contract/internal/contract"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/pluginpb"
)

const maxRequestSize = 128 << 20

func main() {
	requestBytes, err := io.ReadAll(io.LimitReader(os.Stdin, maxRequestSize+1))
	if err != nil {
		fatal(err)
	}
	if len(requestBytes) > maxRequestSize {
		fatal(fmt.Errorf("CodeGeneratorRequest exceeds %d bytes", maxRequestSize))
	}
	request := new(pluginpb.CodeGeneratorRequest)
	if err := proto.Unmarshal(requestBytes, request); err != nil {
		fatal(fmt.Errorf("decode CodeGeneratorRequest: %w", err))
	}
	language, bindingsPath, err := parseParameter(request.GetParameter())
	var response *pluginpb.CodeGeneratorResponse
	if err == nil {
		response, err = contract.GeneratePluginResponse(request, language, bindingsPath)
	}
	if err != nil {
		response = &pluginpb.CodeGeneratorResponse{Error: proto.String(err.Error())}
	} else {
		response.SupportedFeatures = proto.Uint64(uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL))
	}
	encoded, err := proto.Marshal(response)
	if err != nil {
		fatal(err)
	}
	if _, err := os.Stdout.Write(encoded); err != nil {
		fatal(err)
	}
}

func parseParameter(parameter string) (language, bindingsPath string, err error) {
	seen := map[string]bool{}
	for _, item := range strings.Split(parameter, ",") {
		key, value, ok := strings.Cut(item, "=")
		if !ok || key == "" || value == "" {
			return "", "", fmt.Errorf("plugin options must be lang=LANG,bindings=PATH")
		}
		if seen[key] {
			return "", "", fmt.Errorf("duplicate plugin option %q", key)
		}
		seen[key] = true
		switch key {
		case "lang":
			language = value
		case "bindings":
			bindingsPath = value
		default:
			return "", "", fmt.Errorf("unknown plugin option %q", key)
		}
	}
	if !seen["lang"] || !seen["bindings"] {
		return "", "", fmt.Errorf("plugin options must include lang=LANG and bindings=PATH")
	}
	switch language {
	case "typescript", "go", "python", "java", "dotnet":
	default:
		return "", "", fmt.Errorf("unsupported language %q", language)
	}
	return language, bindingsPath, nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "protoc-gen-proto-contract:", err)
	os.Exit(1)
}
