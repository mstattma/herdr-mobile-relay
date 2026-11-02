// @vitest-environment node
import { execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { brotliDecompressSync } from 'node:zlib';
import { expect, it } from 'vitest';

it('stamps the core browser version and full fork deployment identity', () => {
  const root = mkdtempSync(join(tmpdir(), 'fork-version-'));
  try {
    const filename = join(root, 'version.json');
    writeFileSync(filename, JSON.stringify({ version: '0.24.0', assets: 485001 }));
    execFileSync(process.execPath, [
      fileURLToPath(new URL('../../../scripts/stamp-web-version.mjs', import.meta.url)),
      filename, '0.24.0+mst.1', 'synthetic-commit',
    ]);
    const stamped = readFileSync(filename, 'utf8');
    expect(JSON.parse(stamped)).toEqual({
      version: '0.24.0', release_version: '0.24.0+mst.1',
      revision: 'synthetic-commit', assets: 485001,
    });
    expect(brotliDecompressSync(readFileSync(`${filename}.br`)).toString()).toBe(stamped);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});
