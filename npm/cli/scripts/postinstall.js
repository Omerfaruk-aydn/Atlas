#!/usr/bin/env node
// Postinstall: download the platform-specific binary from GitHub Releases.
const fs = require('fs');
const path = require('path');
const https = require('https');
const { URL } = require('url');

const PLATFORM_MAP = { win32: 'windows', darwin: 'darwin', linux: 'linux' };
const platform = PLATFORM_MAP[process.platform] || process.platform;
const arch = process.arch; // 'x64' | 'arm64'
const ext = process.platform === 'win32' ? '.exe' : '';
const assetName = `atlas-coder-${platform}-${arch}${ext}`;

const REPO = 'Omerfaruk-aydn/Atlas-Agent';
const VERSION = require('../package.json').version;
const TAG = `v${VERSION}`;

const binDir = path.join(__dirname, '..', 'bin');
fs.mkdirSync(binDir, { recursive: true });
const dest = path.join(binDir, assetName);

function follow(url, redirectsLeft = 5) {
  return new Promise((resolve, reject) => {
    https.get(url, (res) => {
      if ([301, 302, 303, 307, 308].includes(res.statusCode)) {
        if (redirectsLeft <= 0) return reject(new Error('too many redirects'));
        return follow(new URL(res.headers.location).toString(), redirectsLeft - 1).then(resolve, reject);
      }
      resolve(res);
    }).on('error', reject);
  });
}

async function download() {
  const url = `https://github.com/${REPO}/releases/download/${TAG}/${assetName}`;
  console.log(`Atlas Agent postinstall: downloading ${assetName} ...`);
  const res = await follow(url);
  if (res.statusCode !== 200) {
    throw new Error(`HTTP ${res.statusCode} for ${url}`);
  }
  const tmp = dest + '.part';
  await new Promise((resolve, reject) => {
    const f = fs.createWriteStream(tmp);
    res.pipe(f);
    f.on('finish', () => f.close(resolve));
    f.on('error', reject);
  });
  fs.renameSync(tmp, dest);
  if (process.platform !== 'win32') fs.chmodSync(dest, 0o755);
  const size = (fs.statSync(dest).size / 1024 / 1024).toFixed(1);
  console.log(`Atlas Agent postinstall: installed ${assetName} (${size} MB)`);
}

download().catch((err) => {
  console.error(`Atlas Agent postinstall FAILED: ${err.message}`);
  console.error('This is usually one of:');
  console.error('  - No release yet for this version. Run:');
  console.error(`    gh release create ${TAG} --title "${TAG}" --notes "release"`);
  console.error('    then upload the binary as ' + assetName);
  console.error('  - Network/registry blocked. Try again later.');
  // Don't fail install — the launcher will print a clear error if the binary is missing.
});
