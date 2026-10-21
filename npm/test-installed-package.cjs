#!/usr/bin/env node
"use strict";

// Exercise the delivered archive as an npm consumer, outside the source tree.
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { spawnSync } = require("node:child_process");

assert.ok(process.argv[2], "usage: node npm/test-installed-package.cjs artifact.tgz");
const archive = path.resolve(process.argv[2]);
const work = fs.mkdtempSync(path.join(os.tmpdir(), "proto contract npm consumer "));
const win = process.platform === "win32";
function run(command, args, expected = 0) {
  const result = spawnSync(command, args, { cwd: work, encoding: "utf8", timeout: 120000, env: { ...process.env, PATH: path.join(work, "node_modules", ".bin") + path.delimiter + process.env.PATH } });
  assert.ifError(result.error);
  assert.equal(result.status, expected, `${command} ${args.join(" ")}\n${result.stdout}\n${result.stderr}`);
  return result;
}
function npm(args) {
  // npm's own JS entry point avoids requiring a shell for .cmd on Windows.
  const cli = process.env.npm_execpath || path.join(path.dirname(process.execPath), "node_modules", "npm", "bin", "npm-cli.js");
  if (fs.existsSync(cli)) return run(process.execPath, [cli, ...args]);
  assert.ok(!win, "set npm_execpath to npm-cli.js when npm is not next to node.exe");
  return run("npm", args);
}
try {
  fs.writeFileSync(path.join(work, "package.json"), JSON.stringify({ name: "installed-contract-consumer", version: "1.0.0", private: true }));
  npm(["install", "--ignore-scripts", "--no-audit", "--no-fund", "--package-lock=false", archive]);
  const installed = path.join(work, "node_modules", "@dunkymole", "proto-contract");
  const pkg = JSON.parse(fs.readFileSync(path.join(installed, "package.json"), "utf8"));
  assert.equal(pkg.name, "@dunkymole/proto-contract");
  assert.ok(!pkg.scripts?.install && !pkg.scripts?.preinstall && !pkg.scripts?.postinstall, "package must not run installation hooks");
  assert.ok(fs.readFileSync(path.join(installed, "LICENSE"), "utf8").includes("MIT License"));
  const { resolveBinary } = require(path.join(installed, "bin", "launcher.cjs"));
  assert.throws(() => resolveBinary("proto-contract", "unsupported", "x64"), /unsupported/i);
  fs.writeFileSync(path.join(work, "service.proto"), 'syntax = "proto3";\npackage npm_consumer;\nmessage Request { string text = 1; }\nmessage Response {}\nservice API { rpc Call(Request) returns (Response); }\n');
  const schema = ["--proto", "service.proto", "--proto-path", ".", "--service", "npm_consumer.API"];
  // npm exec exercises the installed .bin shims, including cmd shims on Windows.
  npm(["exec", "--offline", "--", "proto-contract", "snapshot", ...schema, "--api", "npm_consumer", "--version", "1.0.0", "--out", "lock.json"]);
  npm(["exec", "--offline", "--", "proto-contract", "check", ...schema, "--lock", "lock.json"]);
  npm(["exec", "--offline", "--", "proto-contract", "generate", ...schema, "--lock", "lock.json", "--lang", "python", "--out", "cli.py"]);
  fs.writeFileSync(path.join(work, "bindings.json"), JSON.stringify({ bindings: [{ service: "npm_consumer.API", lock: "lock.json", output: "plugin.py" }] }));
  // PATH discovery is essential on Windows: protoc wraps a discovered npm .cmd
  // shim with cmd.exe; an explicit --plugin=.cmd would bypass that handling.
  run("protoc", ["-I", ".", "--proto-contract_out=.", "--proto-contract_opt=lang=python,bindings=bindings.json", "service.proto"]);
  assert.equal(fs.readFileSync(path.join(work, "plugin.py"), "utf8"), fs.readFileSync(path.join(work, "cli.py"), "utf8"));
  const failure = spawnSync(process.execPath, [path.join(installed, "bin", "proto-contract.cjs"), "check", "--lock", "missing.json"], { cwd: work, encoding: "utf8" });
  assert.ifError(failure.error);
  assert.ok(Number.isInteger(failure.status) && failure.status !== 0, "native CLI failure must propagate through launcher");
  console.log(`PASS installed npm tarball ${pkg.version}: compiler, real protoc plugin, path with spaces, unsupported platform and exit propagation`);
} finally {
  fs.rmSync(work, { recursive: true, force: true });
}
