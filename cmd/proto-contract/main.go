package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dunkymole/proto-contract/internal/contract"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "snapshot":
		err = snapshot(os.Args[2:])
	case "check":
		err = check(os.Args[2:])
	case "update":
		err = update(os.Args[2:])
	case "release-check":
		err = releaseCheck(os.Args[2:])
	case "version":
		err = version(os.Args[2:])
	case "generate":
		err = generate(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "proto-contract:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: proto-contract <snapshot|check|update|release-check|version|generate> [options]")
	os.Exit(2)
}

func generate(args []string) error {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	lock := fs.String("lock", "", "contract lock used to build this client or server")
	language := fs.String("lang", "typescript", "output language: typescript, python, go, java, dotnet")
	packageName := fs.String("package", "", "generated Go/Java package or .NET namespace")
	out := fs.String("out", "", "generated module path")
	protoFile := fs.String("proto", "", "root .proto file (required for strict TypeScript generation)")
	protoPath := fs.String("proto-path", ".", "protoc import root")
	service := fs.String("service", "", "fully-qualified protobuf service (required for TypeScript)")
	protoc := fs.String("protoc", "protoc", "protoc executable")
	serviceImport := fs.String("service-import", "", "TypeScript module specifier for the generated Protobuf-ES service")
	serviceExport := fs.String("service-export", "", "TypeScript service descriptor export name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *lock == "" || *out == "" {
		return fmt.Errorf("--lock and --out are required")
	}
	lockPath, err := filepath.Abs(*lock)
	if err != nil {
		return err
	}
	outPath, err := filepath.Abs(*out)
	if err != nil {
		return err
	}
	if lockPath == outPath {
		return fmt.Errorf("output must not overwrite the contract lock")
	}
	s, err := contract.Read(*lock)
	if err != nil {
		return err
	}
	var source []byte
	if *language == "typescript" {
		if *packageName != "" {
			return fmt.Errorf("--package is only valid for native generation; TypeScript uses a service import and export")
		}
		if *protoFile == "" || *service == "" || *serviceImport == "" || *serviceExport == "" {
			return fmt.Errorf("strict TypeScript generation requires --proto, --service, --service-import and --service-export; use protoc-gen-proto-contract for multiple services")
		}
		set, err := contract.CompileDescriptorSet(*protoc, *protoPath, *protoFile)
		if err != nil {
			return err
		}
		if err := contract.ValidateSnapshotAgainst(s, set, *service); err != nil {
			return fmt.Errorf("descriptor/lock validation failed: %w", err)
		}
		source, err = contract.TypeScriptBinding(s, set, *serviceImport, *serviceExport)
		if err != nil {
			return err
		}
	} else {
		if *protoFile != "" || *service != "" || *serviceImport != "" || *serviceExport != "" {
			return fmt.Errorf("--proto, --service, --service-import and --service-export are only valid for strict TypeScript generation")
		}
		source, err = contract.Generate(s, *language, *packageName)
		if err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0755); err != nil {
		return err
	}
	return os.WriteFile(*out, source, 0644)
}

func update(args []string) error {
	fs, protoFile, path, service, protoc := common("update", args)
	lock := fs.String("lock", "", "checked-in contract lock")
	requested := fs.String("bump", "auto", "optional higher bump for semantic changes: minor or major")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *protoFile == "" || *service == "" || *lock == "" {
		return fmt.Errorf("--proto, --service and --lock are required")
	}
	old, err := contract.Read(*lock)
	if err != nil {
		return err
	}
	current, err := contract.Compile(*protoc, *path, *protoFile, *service, old.API, old.Version)
	if err != nil {
		return err
	}
	minimum, changes := contract.Compare(old, current)
	wanted := minimum
	if *requested != "auto" {
		wanted, err = contract.ParseBump(*requested)
		if err != nil {
			return err
		}
	}
	if wanted < minimum {
		return fmt.Errorf("requested %s bump is below the calculated %s minimum", wanted, minimum)
	}
	next, err := contract.NextVersion(old.Version, wanted)
	if err != nil {
		return err
	}
	current.Version = next
	if err := contract.Write(*lock, current); err != nil {
		return err
	}
	for _, change := range changes {
		fmt.Println(change)
	}
	fmt.Printf("%s: %s -> %s (%s)\n", old.API, old.Version, next, wanted)
	return nil
}

func common(name string, args []string) (*flag.FlagSet, *string, *string, *string, *string) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	proto := fs.String("proto", "", "root .proto file")
	path := fs.String("proto-path", ".", "protoc import root")
	service := fs.String("service", "", "fully-qualified service name")
	protoc := fs.String("protoc", "protoc", "protoc executable")
	return fs, proto, path, service, protoc
}

func snapshot(args []string) error {
	fs, proto, path, service, protoc := common("snapshot", args)
	api := fs.String("api", "", "stable API identifier")
	ver := fs.String("version", "", "contract version")
	out := fs.String("out", "", "output lock file")
	force := fs.Bool("force", false, "replace an existing lock file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *proto == "" || *service == "" || *api == "" || *ver == "" || *out == "" {
		return fmt.Errorf("--proto, --service, --api, --version and --out are required")
	}
	if _, err := os.Lstat(*out); err == nil && !*force {
		return fmt.Errorf("output %q already exists; pass --force to replace it", *out)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	s, err := contract.Compile(*protoc, *path, *proto, *service, *api, *ver)
	if err != nil {
		return err
	}
	if err := contract.Write(*out, s); err != nil {
		return err
	}
	fmt.Printf("%s %s %s\n", s.API, s.Version, s.Digest)
	return nil
}

func check(args []string) error {
	fs, proto, path, service, protoc := common("check", args)
	lock := fs.String("lock", "", "checked-in contract lock")
	format := fs.String("format", "text", "report format: text or json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *format != "text" && *format != "json" {
		return fmt.Errorf("unsupported check report format %q (expected text or json)", *format)
	}
	if *proto == "" || *service == "" || *lock == "" {
		return fmt.Errorf("--proto, --service and --lock are required")
	}
	old, err := contract.Read(*lock)
	if err != nil {
		return err
	}
	current, err := contract.Compile(*protoc, *path, *proto, *service, old.API, old.Version)
	if err != nil {
		return err
	}
	bump, changes := contract.Compare(old, current)
	next := old.Version
	if bump != contract.None {
		next, err = contract.NextVersion(old.Version, bump)
		if err != nil {
			return fmt.Errorf("cannot calculate next contract version: %w", err)
		}
	}
	if *format == "json" {
		report := contractCheckReport{
			API: old.API, Status: "unchanged", Version: old.Version,
			NextVersion: next, Bump: bump.String(), Changes: changes,
		}
		if bump != contract.None {
			report.Status = "changed"
		}
		encoded, err := encodeCheckReport(report)
		if err != nil {
			return err
		}
		fmt.Println(string(encoded))
	}
	if bump == contract.None {
		if *format == "text" {
			fmt.Printf("%s %s: unchanged\n", old.API, old.Version)
		}
		return nil
	}
	if *format == "text" {
		for _, c := range changes {
			fmt.Println(c)
		}
	}
	return fmt.Errorf("contract changed; minimum bump is %s (%s -> %s)", bump, old.Version, next)
}

type contractCheckReport struct {
	API         string   `json:"api"`
	Status      string   `json:"status"`
	Version     string   `json:"version"`
	NextVersion string   `json:"next_version"`
	Bump        string   `json:"bump"`
	Changes     []string `json:"changes"`
}

func encodeCheckReport(report contractCheckReport) ([]byte, error) {
	return json.Marshal(report)
}

func version(args []string) error {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	lock := fs.String("lock", "", "contract lock")
	if err := fs.Parse(args); err != nil {
		return err
	}
	s, err := contract.Read(*lock)
	if err != nil {
		return err
	}
	fmt.Println(s.Version)
	return nil
}

func releaseCheck(args []string) error {
	fs := flag.NewFlagSet("release-check", flag.ContinueOnError)
	basePath := fs.String("base-lock", "", "trusted base lock")
	lockPath := fs.String("lock", "", "proposed lock")
	baseHistoryPath := fs.String("base-history", "", "trusted base release manifest (may be absent only during bootstrap)")
	historyPath := fs.String("history", "", "proposed append-only release manifest")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *basePath == "" || *lockPath == "" || *historyPath == "" {
		return fmt.Errorf("--base-lock, --lock and --history are required")
	}
	base, err := contract.Read(*basePath)
	if err != nil {
		return err
	}
	proposed, err := contract.Read(*lockPath)
	if err != nil {
		return err
	}
	baseHistory := &contract.ReleaseManifest{Format: 1, Releases: []contract.Release{}}
	if *baseHistoryPath != "" {
		if _, statErr := os.Stat(*baseHistoryPath); statErr == nil {
			baseHistory, err = contract.ReadReleaseManifest(*baseHistoryPath)
			if err != nil {
				return err
			}
		} else if !os.IsNotExist(statErr) {
			return statErr
		}
	}
	history, err := contract.ReadReleaseManifest(*historyPath)
	if err != nil {
		return err
	}
	if err := contract.CheckReleaseTransition(base, proposed, baseHistory, history); err != nil {
		return err
	}
	fmt.Printf("%s %s release assignment is valid\n", proposed.API, proposed.Version)
	return nil
}
