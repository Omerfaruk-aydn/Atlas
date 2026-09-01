#!/usr/bin/env node
// Minimal launcher: resolve the platform-specific binary next to this file and exec it.
const path = require('path');
const fs = require('fs');
const { spawn } = require('child_process');

const PLATFORM_MAP = { win32: 'windows', darwin: 'darwin', linux: 'linux' };

function detectBinary() {
  const platform = PLATFORM_MAP[process.platform] || process.platform;
  const arch = process.arch; // 'x64' | 'arm64'
  const ext = process.platform === 'win32' ? '.exe' : '';
  const name = `atlas-agent-${platform}-${arch}${ext}`;
  const binary = path.join(__dirname, '..', 'binary', name);
  if (!fs.existsSync(binary)) {
    console.error(`Atlas Agent: no binary for ${platform}/${arch} (expected ${name})`);
    process.exit(1);
  }
  return binary;
}

const bin = detectBinary();
const child = spawn(bin, process.argv.slice(2), { stdio: 'inherit' });
child.on('exit', (code) => process.exit(code ?? 0));
child.on('error', (err) => {
  console.error(`Atlas Agent: failed to launch binary: ${err.message}`);
  process.exit(1);
});
