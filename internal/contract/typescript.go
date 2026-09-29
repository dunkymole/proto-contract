package contract

import "fmt"

// TypeScript preserves a clear migration error for callers of the prior
// lock-only helper. Strict TypeScript output requires request descriptors.
func TypeScript(_ *Snapshot) ([]byte, error) {
	return nil, fmt.Errorf("TypeScript generation requires validated descriptors; use TypeScriptBinding or protoc-gen-proto-contract")
}
