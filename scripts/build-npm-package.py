"""Build a self-contained, unpublished npm compiler package from checked sources."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]
COMMANDS = ("proto-contract", "protoc-gen-proto-contract")
TARGETS = (("linux", "linux"), ("darwin", "darwin"), ("win32", "windows"))
ARCHES = (("x64", "amd64"), ("arm64", "arm64"))
LICENSE_NAMES = ("LICENSE", "LICENSE.txt", "LICENSE.md", "COPYING", "NOTICE", "NOTICE.txt", "PATENTS")


def output(args, **kwargs):
    return subprocess.check_output(args, cwd=ROOT, text=True, **kwargs).strip()


def json_stream(text):
    decoder = json.JSONDecoder()
    position = 0
    while position < len(text):
        while position < len(text) and text[position].isspace():
            position += 1
        if position == len(text):
            break
        value, position = decoder.raw_decode(text, position)
        yield value


def license_text(directory, title):
    pieces = []
    for name in LICENSE_NAMES:
        file = directory / name
        if file.is_file():
            pieces.append(f"### {title} — {name}\n\n```text\n{file.read_text(encoding='utf-8').rstrip()}\n```\n")
    if not any((directory / name).is_file() for name in LICENSE_NAMES[:4]):
        raise ValueError(f"missing license for linked dependency {title}")
    return "\n".join(pieces)


def notices(environment):
    goroot = Path(output(["go", "env", "GOROOT"], env=environment))
    sections = ["# Third-party notices\n\nThese notices cover the Go standard library and external dependency modules linked into the bundled commands.\n", license_text(goroot, "Go standard library")]
    dependencies = {}
    # Enumerate both executable dependency graphs on every shipped target.
    # This includes modules selected only by platform-specific build tags.
    for _, goos in TARGETS:
        for _, goarch in ARCHES:
            target_env = {**environment, "GOOS": goos, "GOARCH": goarch}
            data = output(["go", "list", "-deps", "-json", *[f"./cmd/{command}" for command in COMMANDS]], env=target_env)
            for package in json_stream(data):
                module = package.get("Module")
                if module and not module.get("Main"):
                    if module.get("Replace"):
                        raise ValueError("replacement modules are unsupported in release compiler builds")
                    dependencies[(module["Path"], module["Version"])] = Path(module["Dir"])
                elif package.get("ImportPath", "").startswith("vendor/"):
                    # Standard-library vendored modules have no Module record.
                    directory = Path(package["Dir"])
                    boundary = goroot / "src" / "vendor"
                    while directory != boundary and not any((directory / name).is_file() for name in LICENSE_NAMES[:4]):
                        directory = directory.parent
                    if directory == boundary:
                        raise ValueError(f"missing license for Go vendored package {package['ImportPath']}")
                    name = directory.relative_to(boundary).as_posix()
                    dependencies[(name, "vendored with Go")] = directory
    for (name, version), directory in sorted(dependencies.items()):
        sections.append(license_text(directory, f"{name} ({version})"))
    return "\n".join(sections)


def build(version, destination, source_revision):
    if not re.fullmatch(r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)", version):
        raise ValueError("expected a numeric package version")
    if not re.fullmatch(r"[0-9a-f]{40}", source_revision):
        raise ValueError("source revision must be a lowercase full Git commit ID")
    package = json.loads((ROOT / "npm/package.json").read_text(encoding="utf-8-sig"))
    if package["version"] != version:
        raise ValueError("npm package version differs from requested release version")
    destination.mkdir(parents=True, exist_ok=True)
    artifact = destination / f"dunkymole-proto-contract-{version}.tgz"
    if artifact.exists():
        raise ValueError("npm artifact already exists; choose a fresh output directory")
    # Override host cross-compilation knobs so identical source/toolchains build
    # the same baseline CPU targets on the pinned release builder.
    environment = {key: value for key, value in os.environ.items() if not key.startswith(("GO", "CGO")) or key in ("GOPATH", "GOMODCACHE", "GOCACHE", "GOPROXY", "GOSUMDB")}
    environment.update({"CGO_ENABLED": "0", "GOAMD64": "v1", "GOARM64": "v8.0", "GOTOOLCHAIN": "local", "GOENV": "off", "GOWORK": "off", "SOURCE_DATE_EPOCH": "315532800"})
    with tempfile.TemporaryDirectory(prefix="proto-contract-npm-") as temporary:
        staging = Path(temporary)
        for name in ("package.json", "README.md"):
            # Normalize checked-in Windows line endings for deterministic archives.
            (staging / name).write_text((ROOT / "npm" / name).read_text(encoding="utf-8-sig"), encoding="utf-8", newline="\n")
        (staging / "LICENSE").write_bytes((ROOT / "LICENSE").read_bytes().replace(b"\r\n", b"\n"))
        (staging / "bin").mkdir()
        for source in sorted((ROOT / "npm/bin").glob("*.cjs")):
            target = staging / "bin" / source.name
            target.write_bytes(source.read_bytes().replace(b"\r\n", b"\n"))
            target.chmod(0o755)
        (staging / "THIRD_PARTY_NOTICES.md").write_text(notices(environment), encoding="utf-8", newline="\n")
        checksums = {}
        for platform, goos in TARGETS:
            for arch, goarch in ARCHES:
                directory = staging / "native" / f"{platform}-{arch}"
                directory.mkdir(parents=True)
                for command in COMMANDS:
                    binary = directory / (command + (".exe" if goos == "windows" else ""))
                    subprocess.run(["go", "build", "-trimpath", "-buildvcs=false", "-ldflags=-s -w -buildid=", "-o", str(binary), f"./cmd/{command}"], cwd=ROOT, env={**environment, "GOOS": goos, "GOARCH": goarch}, check=True)
                    binary.chmod(0o755)
                    checksums[binary.relative_to(staging).as_posix()] = hashlib.sha256(binary.read_bytes()).hexdigest()
        manifest = {"version": version, "sourceRevision": source_revision, "goVersion": output(["go", "version"], env=environment), "nodeVersion": output(["node", "--version"]), "npmVersion": output(["npm", "--version"]), "files": checksums}
        (staging / "native-manifest.json").write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8", newline="\n")
        # npm's pack format sets stable tar timestamps. Disable all lifecycle
        # scripts and use an isolated cache so packaging cannot mutate sources.
        npm_env = {**os.environ, "npm_config_cache": str(staging / ".npm-cache"), "npm_config_update_notifier": "false", "SOURCE_DATE_EPOCH": "315532800"}
        subprocess.run(["npm", "pack", "--ignore-scripts", "--json", "--pack-destination", str(destination.resolve())], cwd=staging, env=npm_env, check=True)
    if not artifact.is_file():
        raise ValueError("npm pack did not produce the expected artifact")
    return artifact


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", default=json.loads((ROOT / "release.json").read_text(encoding="utf-8-sig"))["version"])
    parser.add_argument("--out", required=True, type=Path)
    parser.add_argument("--source-revision", default=os.environ.get("SOURCE_REVISION"))
    args = parser.parse_args()
    revision = args.source_revision or output(["git", "rev-parse", "HEAD"])
    print(build(args.version, args.out, revision))


if __name__ == "__main__":
    main()
