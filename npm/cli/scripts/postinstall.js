#!/usr/bin/env node
// Postinstall: download the platform-specific binary from GitHub Releases.
// Errors are printed to stderr and exit 1 so npm install surfaces them.
const fs = require('fs');
const path = require('path');
const https = require('https');
const { URL } = require('url');

const PLATFORM_MAP = { win32: 'windows', darwin: 'darwin', linux: 'linux' };
const platform = PLATFORM_MAP[process.platform] || process.platform;
const arch = process.arch;
const ext = process.platform === 'win32' ? '.exe' : '';
const assetName = `atlas-agent-${platform}-${arch}${ext}`;

const REPO = 'Omerfaruk-aydn/Atlas-Agent';
const VERSION = require('../package.json').version;
const TAG = `v${VERSION}`;

const binDir = path.join(__dirname, '..', 'bin');
fs.mkdirSync(binDir, { recursive: true });
const dest = path.join(binDir, assetName);

function download(url, redirectsLeft) {
  if (redirectsLeft == null) redirectsLeft = 5;
  return new Promise((resolve, reject) => {
    const req = https.get(url, { headers: { 'User-Agent': 'atlas-agent-installer' } }, (res) => {
      // Follow redirects
      if ([301, 302, 303, 307, 308].includes(res.statusCode)) {
        if (redirectsLeft <= 0) return reject(new Error('Too many redirects for ' + url));
        const next = res.headers.location;
        if (!next) return reject(new Error('Redirect with no Location header'));
        res.resume();
        return resolve(download(new URL(next, url).toString(), redirectsLeft - 1));
      }
      if (res.statusCode !== 200) {
        return reject(new Error('HTTP ' + res.statusCode + ' for ' + url));
      }
      const tmp = dest + '.part';
      const out = fs.createWriteStream(tmp);
      res.pipe(out);
      out.on('finish', () => out.close(() => {
        try { fs.renameSync(tmp, dest); resolve(); }
        catch (e) { reject(e); }
      }));
      out.on('error', reject);
      res.on('error', reject);
    });
    req.on('error', reject);
    req.setTimeout(60000, () => { req.destroy(new Error('Download timeout (60s)')); });
  });
}

(async () => {
  const url = `https://github.com/${REPO}/releases/download/${TAG}/${assetName}`;
  console.log(`Atlas Agent postinstall: downloading ${assetName} from ${url}`);
  await download(url);
  if (process.platform !== 'win32') {
    fs.chmodSync(dest, 0o755);
  }
  const size = (fs.statSync(dest).size / 1024 / 1024).toFixed(1);
  console.log(`Atlas Agent postinstall: installed ${assetName} (${size} MB)`);
})().catch((err) => {
  console.error('Atlas Agent postinstall FAILED: ' + err.message);
  console.error('URL: https://github.com/' + REPO + '/releases/download/' + TAG + '/' + assetName);
  console.error('You can manually download it from:');
  console.error('  https://github.com/' + REPO + '/releases/tag/' + TAG);
  console.error('and place it at:');
  console.error('  ' + dest);
  process.exit(1);
});
