#!/usr/bin/env node
// One-shot: write a secret to a GitHub repo using the public-key encryption
// the Actions Secrets API requires.
//
// Usage:
//   GITHUB_PAT=ghp_xxx node scripts/github-set-secret.mjs <owner/repo> <name> <value>

import { Buffer } from 'node:buffer';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const sealedbox = require('tweetnacl-sealedbox-js');
const naclUtil = require('tweetnacl-util');

const [, , repoArg, nameArg, valueArg] = process.argv;
const pat = process.env.GITHUB_PAT;
if (!pat || !repoArg || !nameArg || !valueArg) {
  console.error('Usage: GITHUB_PAT=ghp_xxx node scripts/github-set-secret.mjs <owner/repo> <name> <value>');
  process.exit(2);
}

async function api(method, path, body) {
  const headers = {
    'Authorization': `Bearer ${pat}`,
    'Accept': 'application/vnd.github+json',
    'X-GitHub-Api-Version': '2022-11-28',
    'User-Agent': 'atlas-agent-secret-set',
  };
  if (body) headers['Content-Type'] = 'application/json';
  const r = await fetch(`https://api.github.com${path}`, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
  });
  const text = await r.text();
  if (!r.ok) {
    throw new Error(`${method} ${path} -> ${r.status}: ${text}`);
  }
  return text ? JSON.parse(text) : null;
}

const key = await api('GET', `/repos/${repoArg}/actions/secrets/public-key`);
console.log(`Got public key (id=${key.key_id}, ${key.key.length} chars base64)`);

const publicKey = Buffer.from(key.key, 'base64');
const message = naclUtil.decodeUTF8(valueArg);
const ciphertext = sealedbox.seal(message, publicKey);
const encrypted = Buffer.from(ciphertext).toString('base64');

const result = await api('PUT', `/repos/${repoArg}/actions/secrets/${nameArg}`, {
  encrypted_value: encrypted,
  key_id: key.key_id,
});
console.log(`Secret '${nameArg}' updated.`);
if (result) console.log(JSON.stringify(result, null, 2));
