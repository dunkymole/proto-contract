// Test the installed compiler and plugin on a native host without Go or protoc.
'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');

const artifact = path.resolve(process.argv[2]);
const root = path.resolve(__dirname, '..');
const temporary = fs.mkdtempSync(path.join(os.tmpdir(), 'npm native smoke '));
const nodeDirectory = path.dirname(process.execPath);
const npmCli = [path.join(nodeDirectory, 'node_modules/npm/bin/npm-cli.js'),
  path.resolve(nodeDirectory, '../lib/node_modules/npm/bin/npm-cli.js')].find(fs.existsSync);
assert.ok(npmCli, 'Node distribution must include npm');
try {
  fs.writeFileSync(path.join(temporary, 'package.json'), JSON.stringify({ name: 'native-smoke', version: '1.0.0', private: true }));
  const installed = spawnSync(process.execPath, [npmCli, 'install', '--ignore-scripts', '--no-audit', '--no-fund', '--package-lock=false', artifact], { cwd: temporary, encoding: 'utf8' });
  assert.equal(installed.status, 0, installed.stderr);
  const bin = path.join(temporary, 'node_modules/@dunkymole/proto-contract/bin');
  const compiler = spawnSync(process.execPath, [path.join(bin, 'proto-contract.cjs'), 'version', '--lock', path.join(root, 'contracts/demo.echo.json')], { encoding: 'utf8' });
  assert.equal(compiler.status, 0, compiler.stderr);
  assert.equal(compiler.stdout.trim(), JSON.parse(fs.readFileSync(path.join(root, 'contracts/demo.echo.json'))).version);
  const plugin = spawnSync(process.execPath, [path.join(bin, 'protoc-gen-proto-contract.cjs')], { input: Buffer.alloc(0) });
  assert.equal(plugin.status, 0, plugin.stderr.toString());
  assert.equal(plugin.stdout[0], 0x0a, 'response begins with CodeGeneratorResponse.error');
  assert.match(plugin.stdout.toString(), /plugin options must/);
  assert.equal(plugin.stderr.length, 0);
  const malformed = spawnSync(process.execPath, [path.join(bin, 'protoc-gen-proto-contract.cjs')], { input: Buffer.from([0xff]) });
  assert.equal(malformed.status, 1);
  assert.equal(malformed.stdout.length, 0);
  assert.match(malformed.stderr.toString(), /decode CodeGeneratorRequest/);
  console.log(`PASS: installed compiler/plugin native smoke on ${process.platform}-${process.arch}`);
} finally {
  fs.rmSync(temporary, { recursive: true, force: true });
}
