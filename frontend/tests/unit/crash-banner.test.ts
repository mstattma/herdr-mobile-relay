import { expect, it } from 'vitest';
import { compactStack } from '$lib/crash-banner';

it('keeps every frame position while dropping origin and asset hash', () => {
  const hash = 'a'.repeat(64);
  const stack = `Error: https://svelte.dev/e/effect_orphan\n    at Fe (https://host.example:8443/assets/app-${hash}.js:1:2705)\n    at _n (https://host.example:8443/assets/app-${hash}.js:1:17000)\n    at Sn (https://host.example:8443/assets/app-${hash}.js:1:17690)`;
  expect(compactStack(stack)).toBe('Error: https://svelte.dev/e/effect_orphan\n    at Fe (app:1:2705)\n    at _n (app:1:17000)\n    at Sn (app:1:17690)');
});
