#!/usr/bin/env node
// postinstall: verify the platform binary is present and executable.
// The launcher (bin/atlas-agent.js) does the platform detection at runtime,
// so this script is just a sanity check + chmod for the unix binary.
const fs = require('fs');
const path = require('path');

const PLATFORM_MAP = { win32: 'windows', darwin: 'darwin', linux: 'linux' };
const platform = PLATFORM_MAP[process.platform] || process.platform;
const arch = process.arch;
const ext = process.platform === 'win32' ? '.exe' : '';
const name = `atlas-agent-${platform}-${arch}${ext}`;
const binary = path.join(__dirname, '..', 'binary', name);

if (!fs.existsSync(binary)) {
  console.error(`Atlas Agent postinstall: no binary for ${platform}/${arch} (${name})`);
  console.error('This platform is not yet supported. Please open an issue.');
  process.exit(1);
}

if (platform !== 'win32') {
  try {
    fs.chmodSync(binary, 0o755);
  } catch (e) {
    console.error(`Atlas Agent postinstall: failed to chmod ${binary}: ${e.message}`);
    process.exit(1);
  }
}

console.log(`Atlas Agent installed (${platform}/${arch}, ${(fs.statSync(binary).size / 1024 / 1024).toFixed(1)} MB)`);
