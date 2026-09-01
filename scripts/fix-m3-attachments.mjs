// Flip supports_attachments to true for any model whose "id" contains
// "m3" (case-insensitive). Operates on every JSON file in the catwalk
// configs directory. Safe to re-run.

import { readFile, writeFile } from 'node:fs/promises';
import { readdir } from 'node:fs/promises';
import { join } from 'node:path';

const root = process.argv[2];
if (!root) {
  console.error('Usage: node fix-m3-attachments.mjs <configs-dir>');
  process.exit(2);
}

// Match any JSON object that:
//   1. has an "id" string containing "m3" (case-insensitive)
//   2. has a "supports_attachments" field set to false
// and rewrite the value to true. We do not add the field if missing;
// we only flip existing false -> true.
const pattern = /("id"\s*:\s*"[^"]*m3[^"]*"(?:(?!}).)*?"supports_attachments"\s*:\s*)false/gis;

const entries = await readdir(root, { withFileTypes: true });
let changedFiles = 0;
let changedEntries = 0;
let stillFalse = 0;

for (const e of entries) {
  if (!e.isFile() || !e.name.endsWith('.json')) continue;
  const path = join(root, e.name);
  const text = await readFile(path, 'utf8');
  let n = 0;
  const updated = text.replace(pattern, (_, prefix) => {
    n++;
    return prefix + 'true';
  });
  if (n > 0) {
    await writeFile(path, updated, 'utf8');
    changedFiles++;
    changedEntries += n;
    console.log(`  ${e.name}: ${n} entries fixed`);
  }
}

// Re-scan to count any remaining false
for (const e of entries) {
  if (!e.isFile() || !e.name.endsWith('.json')) continue;
  const path = join(root, e.name);
  const text = await readFile(path, 'utf8');
  // Reset regex because /g state
  const re = /("id"\s*:\s*"[^"]*m3[^"]*"(?:(?!}).)*?"supports_attachments"\s*:\s*)false/gis;
  const m = text.match(re);
  if (m && m.length > 0) {
    stillFalse += m.length;
    console.log(`  STILL FALSE: ${e.name}: ${m.length}`);
  }
}

console.log('');
console.log(`Files updated: ${changedFiles}`);
console.log(`Entries fixed: ${changedEntries}`);
console.log(`Still false:   ${stillFalse}`);
