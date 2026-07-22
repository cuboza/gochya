import { existsSync, readdirSync, readFileSync } from 'node:fs';
import { dirname, extname, resolve } from 'node:path';

const root = process.cwd();
const ignoredDirectories = new Set(['.git', 'node_modules', 'build', 'dist', 'vendor']);

function markdownFiles(directory) {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    if (ignoredDirectories.has(entry.name)) return [];
    const absolute = resolve(directory, entry.name);
    if (entry.isDirectory()) return markdownFiles(absolute);
    return extname(entry.name) === '.md' ? [absolute] : [];
  });
}

const failures = [];
const markdownLink = /\[[^\]]*\]\(([^)]+)\)/g;

for (const file of markdownFiles(root)) {
  const source = readFileSync(file, 'utf8');
  for (const match of source.matchAll(markdownLink)) {
    const target = match[1].trim();
    if (/^(https?:\/\/|mailto:|#)/.test(target)) continue;

    const pathWithoutAnchor = target.split('#', 1)[0];
    if (!pathWithoutAnchor) continue;

    const resolvedTarget = resolve(dirname(file), decodeURIComponent(pathWithoutAnchor));
    if (!existsSync(resolvedTarget)) {
      const line = source.slice(0, match.index).split('\n').length;
      failures.push(`${file.slice(root.length + 1)}:${line} -> ${target}`);
    }
  }
}

if (failures.length > 0) {
  console.error('Broken local Markdown links:');
  for (const failure of failures) console.error(`- ${failure}`);
  process.exitCode = 1;
} else {
  console.log('All local Markdown links resolve.');
}
