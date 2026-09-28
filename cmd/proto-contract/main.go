package main

import (
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
	fmt.Fprintln(os.Stderr, "usage: proto-contract <snapshot|check|update|version|generate> [options]")
	os.Exit(2)
}

func generate(args []string) error {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	lock := fs.String("lock", "", "contract lock used to build this client")
	language := fs.String("lang", "typescript", "output language: typescript, python, go, java, dotnet")
	packageName := fs.String("package", "", "generated Go/Java package or .NET namespace")
	out := fs.String("out", "", "generated module path")
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
	source, err := contract.Generate(s, *language, *packageName)
	if err != nil {
		return err
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
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *proto == "" || *service == "" || *api == "" || *ver == "" || *out == "" {
		return fmt.Errorf("--proto, --service, --api, --version and --out are required")
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
	if err := fs.Parse(args); err != nil {
		return err
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
	if bump == contract.None {
		fmt.Printf("%s %s: unchanged\n", old.API, old.Version)
		return nil
	}
	next, _ := contract.NextVersion(old.Version, bump)
	for _, c := range changes {
		fmt.Println(c)
	}
	return fmt.Errorf("contract changed; minimum bump is %s (%s -> %s)", bump, old.Version, next)
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
