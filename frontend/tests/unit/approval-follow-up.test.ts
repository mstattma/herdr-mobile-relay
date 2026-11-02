import { get } from 'svelte/store';
import { afterEach, expect, it, vi } from 'vitest';
import { relayStore } from '$lib/store';
import type { Agent } from '$lib/types';

// OpenCode can follow one approval with another dialog while the pane stays
// blocked (the "Always allow" confirmation); its buttons must not stay
// disabled until the 10 s response timeout.
const agent = (fingerprint: string): Agent => ({
  relay_id: 'follow-up', relay_label: 'Fixture', pane_id: 'follow-up::pane', raw_pane_id: 'pane',
  server_session_id: 'server', terminal_id: 'terminal', generation: 1, agent: 'opencode', status: 'blocked',
  cwd: '/fixture', attention_kind: 'approval', approval_fingerprint: fingerprint, event_id: 'event-1',
} as Agent);

type Internals = { agentsValue: Agent[]; reconcileResponding(): void };
const internals = relayStore as unknown as Internals;

afterEach(() => {
  relayStore.clearResponding('follow-up::pane');
  internals.agentsValue = [];
  vi.useRealTimers();
});

it('keeps waiting while the answered dialog is still shown', () => {
  vi.useFakeTimers();
  internals.agentsValue = [agent('first')];
  relayStore.markResponding('follow-up::pane', 'first');
  internals.reconcileResponding();
  expect(get(relayStore.responding).has('follow-up::pane')).toBe(true);
});

it('stops waiting as soon as a follow-up dialog replaces the answered one', () => {
  vi.useFakeTimers();
  internals.agentsValue = [agent('first')];
  relayStore.markResponding('follow-up::pane', 'first');
  internals.agentsValue = [agent('confirmation')];
  internals.reconcileResponding();
  expect(get(relayStore.responding).has('follow-up::pane')).toBe(false);
});
