// Ensure every model whose "id" contains "m3" (case-insensitive) has
// supports_attachments: true. If the field is missing, insert it. If it
// is false, flip to true. If it is already true, leave it alone.
//
// Operates on every JSON file in the catwalk configs directory.
// Safe to re-run; idempotent.

import { readFile, writeFile, readdir } from 'node:fs/promises';
import { join } from 'node:path';

const root = process.argv[2];
if (!root) {
  console.error('Usage: node fix-m3-attachments.mjs <configs-dir>');
  process.exit(2);
}

const entries = await readdir(root, { withFileTypes: true });
let scanned = 0;
let touched = 0;
let alreadyTrue = 0;
let flippedFromFalse = 0;
let insertedMissing = 0;
const problems = [];

for (const e of entries) {
  if (!e.isFile() || !e.name.endsWith('.json')) continue;
  const path = join(root, e.name);
  scanned++;

  let cfg;
  try {
    cfg = JSON.parse(await readFile(path, 'utf8'));
  } catch (err) {
    problems.push(`  parse error in ${e.name}: ${err.message}`);
    continue;
  }

  if (!Array.isArray(cfg?.models)) continue;

  let fileChanged = false;
  for (const m of cfg.models) {
    if (typeof m?.id !== 'string' || !/m3/i.test(m.id)) continue;
    if (m.supports_attachments === true) {
      alreadyTrue++;
      continue;
    }
    if (m.supports_attachments === false) {
      m.supports_attachments = true;
      flippedFromFalse++;
      fileChanged = true;
    } else {
      m.supports_attachments = true;
      insertedMissing++;
      fileChanged = true;
    }
  }

  if (fileChanged) {
    // Preserve key order: insert supports_attachments right after
    // default_max_tokens when present, otherwise at the end.
    const out = [];
    for (const m of cfg.models) {
      if (typeof m?.id !== 'string' || !/m3/i.test(m.id)) {
        out.push(m);
        continue;
      }
      const copy = {};
      let added = false;
      for (const k of Object.keys(m)) {
        copy[k] = m[k];
        if (!added && k === 'default_max_tokens') {
          if (!('supports_attachments' in m)) copy.supports_attachments = true;
          added = true;
        }
      }
      if (!added) copy.supports_attachments = m.supports_attachments ?? true;
      out.push(copy);
    }
    cfg.models = out;
    await writeFile(path, JSON.stringify(cfg, null, 2) + '\n', 'utf8');
    touched++;
  }
}

console.log(`Files scanned:    ${scanned}`);
console.log(`Files touched:    ${touched}`);
console.log(`Already true:     ${alreadyTrue}`);
console.log(`Flipped (false→): ${flippedFromFalse}`);
console.log(`Inserted (none):  ${insertedMissing}`);
if (problems.length) {
  console.log('Problems:');
  for (const p of problems) console.log(p);
}
