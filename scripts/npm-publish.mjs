#!/usr/bin/env node
// One-shot npm publish for @atlas-coder/atlas-agent.
// Reads token from ATLAS_AGENT_NPM_TOKEN env (write-only) to avoid
// leaking it into process listings, registry, or .npmrc.
//
// Usage:
//   ATLAS_AGENT_NPM_TOKEN=npm_xxx node scripts/npm-publish.mjs [tag]
//
// Defaults to latest tag. Pass `next` to publish a prerelease.

import { spawn } from 'node:child_process';
import { readFileSync, existsSync } from 'node:fs';
import { resolve } from 'node:path';

const token = process.env.ATLAS_AGENT_NPM_TOKEN;
if (!token) {
  console.error('ERROR: ATLAS_AGENT_NPM_TOKEN env var is required');
  process.exit(2);
}

// Resolve the package directory. We look relative to the repo root, which
// is two levels up from this script (scripts/ is at the repo root).
const scriptDir = new URL('.', import.meta.url).pathname.replace(/\/$/, '');
const repoRoot = resolve(scriptDir, '..');
const pkgPath = resolve(repoRoot, 'npm', 'cli', 'package.json');
if (!existsSync(pkgPath)) {
  console.error('ERROR: package.json not found at', pkgPath);
  process.exit(2);
}

const pkg = JSON.parse(readFileSync(pkgPath, 'utf8'));
const tag = process.argv[2] || 'latest';

console.log(`Publishing ${pkg.name}@${pkg.version} (tag: ${tag})`);

const args = [
  'publish',
  '--registry=https://registry.npmjs.org/',
  '--tag=' + tag,
  '--access=public',
  pkgPath,
];

const child = spawn('npm.cmd', args, {
  stdio: 'inherit',
  env: { ...process.env, NPM_TOKEN: token, NPM_CONFIG_TOKEN: token },
  shell: true,
});

child.on('exit', (code) => process.exit(code ?? 0));
child.on('error', (err) => {
  console.error('publish failed to launch:', err.message);
  process.exit(1);
});
