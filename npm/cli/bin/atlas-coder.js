#!/usr/bin/env node
// Launcher: exec the platform binary that was downloaded by postinstall.
const path = require('path');
const fs = require('fs');
const { spawn } = require('child_process');

const PLATFORM_MAP = { win32: 'windows', darwin: 'darwin', linux: 'linux' };

function binaryPath() {
  const platform = PLATFORM_MAP[process.platform] || process.platform;
  const arch = process.arch;
  const ext = process.platform === 'win32' ? '.exe' : '';
  const name = `atlas-agent-${platform}-${arch}${ext}`;
  return path.join(__dirname, '..', 'bin', name);
}

const bin = binaryPath();
if (!fs.existsSync(bin)) {
  console.error(`Atlas Agent: binary missing at ${bin}`);
  console.error('Try reinstalling: npm install -g @atlas-coder/atlas-agent --force');
  process.exit(1);
}

const child = spawn(bin, process.argv.slice(2), { stdio: 'inherit' });
child.on('exit', (code) => process.exit(code ?? 0));
child.on('error', (err) => {
  console.error(`Atlas Agent: failed to launch: ${err.message}`);
  process.exit(1);
});
