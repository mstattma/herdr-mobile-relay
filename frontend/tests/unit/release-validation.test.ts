import { createHash } from 'node:crypto';
import { mkdtempSync, readFileSync, readdirSync, rmSync, unlinkSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, join } from 'node:path';
import { brotliCompressSync } from 'node:zlib';
import { afterEach, describe, expect, it } from 'vitest';
import { validateReleaseFixture } from './release-validation-fixture';

const temporaryDirectories: string[] = [];

function validate(mutate?: (root: string) => void) {
  const root = mkdtempSync(join(tmpdir(), 'herdr-release-validation-'));
  temporaryDirectories.push(root);
  return validateReleaseFixture(root, mutate);
}

afterEach(() => {
  for (const root of temporaryDirectories.splice(0)) rmSync(root, { recursive: true, force: true });
});

// Recompress mutated JSON independently so validation reaches the intended identity branch.
function writeJson(root: string, name: string, value: unknown) {
  const source = Buffer.from(`${JSON.stringify(value)}\n`);
  writeFileSync(join(root, name), source);
  writeFileSync(join(root, `${name}.br`), brotliCompressSync(source));
}

function writeAsset(root: string, name: string, source: string) {
  const compressed = brotliCompressSync(Buffer.from(source));
  writeFileSync(join(root, name), source);
  writeFileSync(join(root, `${name}.br`), compressed);
  const sha256 = createHash('sha256').update(source).digest('hex');
  const brotli_sha256 = createHash('sha256').update(compressed).digest('hex');
  return {
    sha256, integrity: `sha256-${Buffer.from(sha256, 'hex').toString('base64')}`,
    brotli_sha256, brotli_integrity: `sha256-${Buffer.from(brotli_sha256, 'hex').toString('base64')}`,
  };
}

// Keep the finalized fixture's identities valid when adding an import graph.
function appendApplicationImports(root: string, imports: string) {
  const descriptor = JSON.parse(readFileSync(join(root, 'release.json'), 'utf8'));
  const previous = descriptor.files.javascript;
  const source = `${readFileSync(join(root, previous.path), 'utf8')}\n${imports}\n`;
  const path = `assets/app-${createHash('sha256').update(source).digest('hex')}.js`;
  descriptor.files.javascript = { path, ...writeAsset(root, path, source) };
  unlinkSync(join(root, previous.path));
  unlinkSync(join(root, `${previous.path}.br`));
  for (const name of readdirSync(join(root, 'assets'))) {
    if (!name.endsWith('.js') || name === basename(path)) continue;
    const chunk = readFileSync(join(root, 'assets', name), 'utf8');
    if (chunk.includes(basename(previous.path))) {
      writeAsset(root, `assets/${name}`, chunk.replaceAll(basename(previous.path), basename(path)));
    }
  }
  const entryPath = descriptor.files.entry.path;
  const entry = readFileSync(join(root, entryPath), 'utf8')
    .replaceAll(previous.path, path)
    .replaceAll(previous.integrity, descriptor.files.javascript.integrity);
  descriptor.files.entry = { path: entryPath, ...writeAsset(root, entryPath, entry) };
  const version = JSON.parse(readFileSync(join(root, 'version.json'), 'utf8'));
  writeJson(root, 'version.json', { ...version, script: `/${path}`, script_sha256: descriptor.files.javascript.sha256 });
  writeJson(root, 'release.json', descriptor);
}

describe('finalized release validation', () => {
  it('accepts the normally generated and compressed release', () => {
    const result = validate();
    expect(result.status, result.stderr).toBe(0);
    expect(result.stdout).toContain('Validated release structure');
  });

  it.each(['static', 'dynamic'])('accepts a reachable nested import graph with a %s root edge and cycles', (edge) => {
    const result = validate((root) => {
      const descriptor = JSON.parse(readFileSync(join(root, 'release.json'), 'utf8'));
      const entry = `fixture.entry-${descriptor.assets}.js`;
      const nested = `fixture_nested-${descriptor.assets}.js`;
      const shared = `fixture-shared-${descriptor.assets}.js`;
      const effect = `fixture-effect-${descriptor.assets}.js`;
      writeAsset(root, `assets/${entry}`, `import {shared} from "./${shared}";export const marker=shared;export const next=()=>import('./${nested}');`);
      writeAsset(root, `assets/${nested}`, `import './${effect}';import {marker} from './${entry}';import './${basename(descriptor.files.javascript.path)}';export default marker;`);
      writeAsset(root, `assets/${shared}`, 'export const shared=1;');
      writeAsset(root, `assets/${effect}`, 'export {};');
      appendApplicationImports(root, edge === 'static' ? `import {marker} from './${entry}';` : `import('./${entry}');`);
    });
    expect(result.status, result.stderr).toBe(0);
    expect(result.stdout).toContain('Validated release structure');
  });

  it.each(['static', 'dynamic'])('rejects a missing nested chunk referenced by a %s import', (edge) => {
    let missing = '';
    const result = validate((root) => {
      const { assets } = JSON.parse(readFileSync(join(root, 'release.json'), 'utf8'));
      const entry = `fixture-entry-${assets}.js`;
      missing = `fixture-missing-${assets}.js`;
      writeAsset(root, `assets/${entry}`, edge === 'static' ? `import {missing} from './${missing}';` : `import('./${missing}');`);
      appendApplicationImports(root, `import('./${entry}');`);
    });
    expect(result.status).not.toBe(0);
    expect(result.stderr).toContain(`Application references an invalid lazy chunk: ${missing}`);
  });

  it('rejects a nested import from a different asset namespace', () => {
    let stale = '';
    const result = validate((root) => {
      const { assets } = JSON.parse(readFileSync(join(root, 'release.json'), 'utf8'));
      const entry = `fixture-entry-${assets}.js`;
      stale = `fixture-stale-${assets + 1}.js`;
      writeAsset(root, `assets/${entry}`, `import {stale} from './${stale}';`);
      appendApplicationImports(root, `import('./${entry}');`);
    });
    expect(result.status).not.toBe(0);
    expect(result.stderr).toContain(`Application references an invalid lazy chunk: ${stale}`);
  });

  it('rejects a disconnected lazy cycle even though its chunks reference each other', () => {
    const result = validate((root) => {
      const { assets } = JSON.parse(readFileSync(join(root, 'release.json'), 'utf8'));
      writeAsset(root, `assets/fixture-orphan-a-${assets}.js`, `import './fixture-orphan-b-${assets}.js';`);
      writeAsset(root, `assets/fixture-orphan-b-${assets}.js`, `import('./fixture-orphan-a-${assets}.js');`);
    });
    expect(result.status).not.toBe(0);
    expect(result.stderr).toContain('Lazy chunks must be referenced by the application');
  });

  it('rejects a wrong decoded digest even when its SRI is consistent', () => {
    let path = '';
    const result = validate((root) => {
      const descriptor = JSON.parse(readFileSync(join(root, 'release.json'), 'utf8'));
      path = descriptor.files.entry.path;
      descriptor.files.entry.sha256 = '0'.repeat(64);
      descriptor.files.entry.integrity = `sha256-${Buffer.alloc(32).toString('base64')}`;
      writeJson(root, 'release.json', descriptor);
    });
    expect(result.status).not.toBe(0);
    expect(result.stderr).toContain(`entry digest does not match ${path}`);
  });

  it('rejects wrong SRI independently of the decoded digest', () => {
    const result = validate((root) => {
      const descriptor = JSON.parse(readFileSync(join(root, 'release.json'), 'utf8'));
      descriptor.files.entry.integrity = `sha256-${Buffer.alloc(32).toString('base64')}`;
      writeJson(root, 'release.json', descriptor);
    });
    expect(result.status).not.toBe(0);
    expect(result.stderr).toContain('release.json has an invalid entry descriptor');
  });

  it('rejects a wrong encoded digest even when its Brotli SRI is consistent', () => {
    let path = '';
    const result = validate((root) => {
      const descriptor = JSON.parse(readFileSync(join(root, 'release.json'), 'utf8'));
      path = descriptor.files.entry.path;
      descriptor.files.entry.brotli_sha256 = '0'.repeat(64);
      descriptor.files.entry.brotli_integrity = `sha256-${Buffer.alloc(32).toString('base64')}`;
      writeJson(root, 'release.json', descriptor);
    });
    expect(result.status).not.toBe(0);
    expect(result.stderr).toContain(`entry Brotli digest does not match ${path}.br`);
  });

  it('rejects mismatched version projection after consistent JSON recompression', () => {
    const result = validate((root) => {
      const version = JSON.parse(readFileSync(join(root, 'version.json'), 'utf8'));
      version.script_sha256 = '0'.repeat(64);
      writeJson(root, 'version.json', version);
    });
    expect(result.status).not.toBe(0);
    expect(result.stderr).toContain('version.json differs from release.json, herdr-plugin.toml, or build-versions.json');
  });

  it('requires a top-level 404 to disable implicit Pages SPA fallback', () => {
    const result = validate((root) => unlinkSync(join(root, '404.html')));
    expect(result.status).not.toBe(0);
    expect(result.stderr).toContain('Required release file is missing: 404.html');
  });

  it.each([
    '<script src="/herdr-bootstrap.js"></script>',
    '<meta http-equiv="refresh" content="0;url=/">',
    '<a href="/" onclick="location.replace(\'/\')">Open</a>',
  ])('rejects executable or automatic error-page navigation', (extra) => {
    const result = validate((root) => writeFileSync(join(root, '404.html'), `<h1>Not found</h1><a href="/">Home</a>${extra}`));
    expect(result.status).not.toBe(0);
    expect(result.stderr).toContain('404.html must be a static error page');
  });

  it('rejects an overlapping immutable rule even with the correct revalidation rules present', () => {
    const result = validate((root) => {
      const file = join(root, '_headers');
      writeFileSync(file, `${readFileSync(file, 'utf8')}\n/*\n  Cache-Control: public, max-age=31536000, immutable\n`);
    });
    expect(result.status).not.toBe(0);
    expect(result.stderr).toContain('without overlapping rules');
  });

  it('rejects extra SPA rewrites after finalization', () => {
    const result = validate((root) => {
      const file = join(root, '_redirects');
      writeFileSync(file, `${readFileSync(file, 'utf8')}\n/* /index.html 200\n`);
    });
    expect(result.status).not.toBe(0);
    expect(result.stderr).toContain('only stable bootstrap paths');
  });
});
