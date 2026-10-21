'use strict';

const path = require('node:path');
const { spawnSync } = require('node:child_process');

const commands = new Set(['proto-contract', 'protoc-gen-proto-contract']);
const platforms = new Set(['linux', 'darwin', 'win32']);
const architectures = new Set(['x64', 'arm64']);

function resolveBinary(command, platform = process.platform, arch = process.arch) {
  if (!commands.has(command)) throw new Error(`Unknown Proto Contract command: ${command}`);
  if (!platforms.has(platform) || !architectures.has(arch)) {
    throw new Error(`Unsupported Proto Contract platform: ${platform}/${arch}. Supported: Linux, macOS, and Windows on x64 or arm64.`);
  }
  return path.join(__dirname, '..', 'native', `${platform}-${arch}`, command + (platform === 'win32' ? '.exe' : ''));
}

function run(command) {
  let executable;
  try {
    executable = resolveBinary(command);
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
    return;
  }
  // Inherited streams keep the plugin's binary protobuf protocol byte-for-byte
  // and avoid buffering limits. No shell interprets arguments or file paths.
  const child = spawnSync(executable, process.argv.slice(2), { stdio: 'inherit' });
  if (child.error) {
    console.error(`Unable to launch ${command}: ${child.error.message}. Reinstall @dunkymole/proto-contract for a complete package.`);
    process.exitCode = 1;
    return;
  }
  if (child.signal) {
    // Match the native process signal on Unix; retain failure on platforms
    // where resending that signal is unavailable.
    try { process.kill(process.pid, child.signal); } catch { process.exitCode = 1; }
    return;
  }
  process.exitCode = child.status === null ? 1 : child.status;
}

module.exports = { resolveBinary, run };
