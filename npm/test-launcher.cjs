#!/usr/bin/env node
'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { test } = require('node:test');
const vm = require('node:vm');
const { resolveBinary } = require('./bin/launcher.cjs');

function harness(result, overrides = {}) {
  const calls = [], diagnostics = [], signals = [];
  const runtime = { platform: 'linux', arch: 'x64', argv: ['node', 'wrapper', 'space value', '$(literal)'], pid: 123, ...overrides };
  runtime.kill = (pid, signal) => signals.push([pid, signal]);
  const exports = {};
  const context = {
    __dirname: path.join(__dirname, 'bin'), module: { exports }, process: runtime,
    console: { error: message => diagnostics.push(message) },
    require: name => name === 'node:child_process' ? { spawnSync: (...args) => { calls.push(args); return result; } } : require(name),
  };
  vm.runInNewContext(fs.readFileSync(path.join(__dirname, 'bin/launcher.cjs'), 'utf8'), context);
  context.module.exports.run('proto-contract');
  return { calls, diagnostics, signals, runtime };
}

test('selects only six bundled OS/architecture pairs and two commands', () => {
  for (const platform of ['linux', 'darwin', 'win32']) {
    for (const arch of ['x64', 'arm64']) {
      for (const command of ['proto-contract', 'protoc-gen-proto-contract']) {
        assert.equal(resolveBinary(command, platform, arch), path.join(__dirname, 'native', `${platform}-${arch}`, command + (platform === 'win32' ? '.exe' : '')));
      }
    }
  }
  assert.throws(() => resolveBinary('../outside', 'linux', 'x64'), /Unknown/);
  assert.throws(() => resolveBinary('proto-contract', 'freebsd', 'x64'), /Unsupported.*freebsd\/x64/);
  assert.throws(() => resolveBinary('proto-contract', 'linux', 'ia32'), /Unsupported.*linux\/ia32/);
});

test('passes arguments literally, inherits streams, preserves exact exit status', () => {
  const result = harness({ status: 23 });
  assert.equal(result.runtime.exitCode, 23);
  assert.equal(result.calls.length, 1);
  assert.deepEqual(Array.from(result.calls[0][1]), ['space value', '$(literal)']);
  assert.equal(result.calls[0][2].stdio, 'inherit');
  assert.equal(result.calls[0][2].shell, undefined);
  assert.deepEqual(result.diagnostics, []);
});

test('missing executable produces a useful diagnostic and nonzero status', () => {
  const result = harness({ error: new Error('ENOENT') });
  assert.equal(result.runtime.exitCode, 1);
  assert.match(result.diagnostics[0], /Unable to launch proto-contract.*ENOENT.*Reinstall/);
});

test('unsupported runtime fails before starting a process', () => {
  const result = harness({ status: 0 }, { platform: 'freebsd' });
  assert.equal(result.runtime.exitCode, 1);
  assert.equal(result.calls.length, 0);
  assert.match(result.diagnostics[0], /Unsupported.*freebsd\/x64/);
});

test('preserves native termination signals', () => {
  const result = harness({ status: null, signal: 'SIGTERM' });
  assert.deepEqual(result.signals, [[123, 'SIGTERM']]);
});
