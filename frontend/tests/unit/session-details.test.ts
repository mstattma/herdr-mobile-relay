import { tick } from 'svelte';
import { get } from 'svelte/store';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import SessionDetails from '$components/SessionDetails.svelte';
import { securityState } from '$lib/security';
import { copySessionField, loadSessionDetails, type SessionDetailsData } from '$lib/session-details';
import { relayStore } from '$lib/store';
import type { Agent, CommandResult } from '$lib/types';

const agent: Agent = {
  relay_id: 'fixture', relay_label: 'Fixture', raw_pane_id: 'pane', pane_id: 'fixture::pane',
  server_session_id: 'primary', terminal_id: 'terminal', generation: 1,
  agent: 'opencode', agent_session_id: 'ses_synthetic', session_name: 'Example task',
  cwd: '/workspace/example/project', status: 'working',
};
const details: SessionDetailsData = {
  session_id: 'ses_synthetic', title: 'Example task', cwd: '/workspace/example/project',
  todos_available: true, todos: [{ content: 'Review fixture', status: 'in_progress', priority: 'high' }],
  subscriptions: {
    available: true, observed_at: 1_700_000_000_000,
    rows: [{ label: 'Example plan', provider: 'Example provider', status: 'available', current: true, paid: 'Paid plan', limits: 'Weekly: 25% remaining' }],
  },
};
const result = (data = details): CommandResult => ({ type: 'command_result', request_id: 'fixture-request', ok: true, data });
let restore: () => void;

beforeEach(() => {
  const agents = get(relayStore.agents);
  const connections = get(relayStore.connections);
  const security = get(securityState);
  restore = () => {
    relayStore.agents.set(agents);
    relayStore.connections.set(connections);
    securityState.set(security);
  };
  securityState.update((state) => ({ ...state, locked: false }));
  relayStore.agents.set([agent]);
  relayStore.connections.set(new Map([[agent.relay_id, {
    status: 'connected', inventory: { state: 'ready' }, capabilities: ['session_details'],
  }]] as never));
});

afterEach(() => {
  cleanup();
  restore();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe('session details API and copy', () => {
  it('uses the exact-target API and native session ID', async () => {
    const send = vi.spyOn(relayStore, 'sendToAgent').mockResolvedValue(result());
    const controller = new AbortController();
    expect(await loadSessionDetails(agent, controller.signal)).toEqual(details);
    expect(send).toHaveBeenCalledExactlyOnceWith(agent, { type: 'get_session_details' }, 10_000, controller.signal);
  });

  it('discards a response after inventory switches to another session', async () => {
    let finish!: (value: CommandResult) => void;
    vi.spyOn(relayStore, 'sendToAgent').mockImplementation(() => new Promise((resolve) => { finish = resolve; }));
    const pending = loadSessionDetails(agent, new AbortController().signal);
    relayStore.agents.set([{ ...agent, agent_session_id: 'ses_other' }]);
    finish(result());
    await expect(pending).rejects.toThrow('target changed');
  });

  it('rejects a mismatched native ID and never dispatches while locked', async () => {
    const send = vi.spyOn(relayStore, 'sendToAgent').mockResolvedValue(result({ ...details, session_id: 'primary' }));
    await expect(loadSessionDetails(agent, new AbortController().signal)).rejects.toThrow('did not match');
    securityState.update((state) => ({ ...state, locked: true }));
    await expect(loadSessionDetails(agent, new AbortController().signal)).rejects.toThrow('unavailable');
    expect(send).toHaveBeenCalledTimes(1);
  });

  it.each([true, false])('copies or selects the read-only field (clipboard available: %s)', async (available) => {
    const writeText = vi.fn().mockImplementation(() => available ? Promise.resolve() : Promise.reject(new Error('Denied')));
    vi.stubGlobal('navigator', { clipboard: { writeText } });
    const input = document.createElement('input');
    input.readOnly = true;
    input.value = 'ses_synthetic';
    document.body.append(input);
    try {
      expect(await copySessionField(input)).toBe(available);
      expect(writeText).toHaveBeenCalledWith('ses_synthetic');
      if (!available) {
        expect(input).toHaveFocus();
        expect(input.selectionStart).toBe(0);
        expect(input.selectionEnd).toBe(input.value.length);
      }
    } finally { input.remove(); }
  });
});

describe('session details drawer', () => {
  it('restores the explicit touch trigger even if it was never focused', async () => {
    const trigger = document.createElement('button');
    document.body.append(trigger);
    vi.spyOn(relayStore, 'sendToAgent').mockResolvedValue(result());
    try {
      render(SessionDetails, { open: true, agent, returnFocus: trigger });
      await waitFor(() => expect(screen.getByRole('button', { name: 'Close' })).toHaveFocus());
      await fireEvent.click(screen.getByRole('button', { name: 'Close' }));
      await waitFor(() => expect(trigger).toHaveFocus());
    } finally { trigger.remove(); }
  });

  it('requests account observations for a sessionless target without using the server ID', async () => {
    const sessionless = { ...agent, agent_session_id: '' };
    relayStore.agents.set([sessionless]);
    const send = vi.spyOn(relayStore, 'sendToAgent').mockResolvedValue(result({ ...details, session_id: '', title: '', todos_available: false, todos: [] }));
    render(SessionDetails, { open: true, agent: sessionless });
    await tick();
    expect(screen.getByRole('dialog', { name: 'Session details' })).toBeInTheDocument();
    expect(screen.getByText('Not created yet')).toBeInTheDocument();
    expect(screen.getByText('Todos unavailable.')).toBeInTheDocument();
    expect(screen.queryByDisplayValue('primary')).not.toBeInTheDocument();
    await screen.findByText('Example plan');
    expect(send).toHaveBeenCalledWith(sessionless, { type: 'get_session_details' }, 10_000, expect.any(AbortSignal));
  });

  it('offers readers read-only details, current subscriptions and selectable native ID', async () => {
    vi.spyOn(relayStore, 'deviceCredential').mockReturnValue({ role: 'reader' } as NonNullable<ReturnType<typeof relayStore.deviceCredential>>);
    vi.spyOn(relayStore, 'sendToAgent').mockResolvedValue(result());
    render(SessionDetails, { open: true, agent });
    expect(await screen.findByText('Review fixture')).toBeInTheDocument();
    expect(screen.getByLabelText('Native session ID')).toHaveAttribute('readonly');
    expect(screen.getByLabelText('Native session ID')).toHaveValue('ses_synthetic');
    expect(screen.getByText('/workspace/example/project')).toBeInTheDocument();
    expect(screen.getByText('Current')).toBeInTheDocument();
    expect(screen.getByText(/Usage is account-wide/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Copy session ID' })).toBeEnabled();
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument();
  });

  it('closes and discards late replies on an exact-target switch', async () => {
    let finish!: (value: CommandResult) => void;
    let signal: AbortSignal | undefined;
    vi.spyOn(relayStore, 'sendToAgent').mockImplementation((_agent, _payload, _timeout, requestSignal) => {
      signal = requestSignal;
      return new Promise((resolve) => { finish = resolve; });
    });
    const view = render(SessionDetails, { open: true, agent });
    await tick();
    const next = { ...agent, agent_session_id: 'ses_other', session_name: 'Other task' };
    relayStore.agents.set([next]);
    await view.rerender({ agent: next });
    expect(signal?.aborted).toBe(true);
    finish(result());
    await tick();
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(screen.queryByText('Review fixture')).not.toBeInTheDocument();
  });

  it('closes on lock and clears displayed details', async () => {
    vi.spyOn(relayStore, 'sendToAgent').mockResolvedValue(result());
    render(SessionDetails, { open: true, agent });
    await screen.findByText('Review fixture');
    securityState.update((state) => ({ ...state, locked: true }));
    await tick();
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(screen.queryByText('Review fixture')).not.toBeInTheDocument();
  });

  it('wraps Tab and Shift+Tab at the drawer boundaries without intercepting interior navigation or Escape', async () => {
    vi.spyOn(relayStore, 'sendToAgent').mockResolvedValue(result());
    render(SessionDetails, { open: true, agent });
    const close = screen.getByRole('button', { name: 'Close' });
    const copy = screen.getByRole('button', { name: 'Copy session ID' });
    const id = screen.getByLabelText('Native session ID');
    await waitFor(() => expect(close).toHaveFocus());
    const backwards = new KeyboardEvent('keydown', { key: 'Tab', shiftKey: true, bubbles: true, cancelable: true });
    await fireEvent(close, backwards);
    expect(backwards.defaultPrevented).toBe(true);
    expect(copy).toHaveFocus();
    const forwards = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true });
    await fireEvent(copy, forwards);
    expect(forwards.defaultPrevented).toBe(true);
    expect(close).toHaveFocus();
    id.focus();
    const interior = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true });
    await fireEvent(id, interior);
    expect(interior.defaultPrevented).toBe(false);
    const escape = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true });
    await fireEvent(id, escape);
    expect(escape.defaultPrevented).toBe(false);
  });

  it('keeps both Tab directions on Close when a sessionless drawer has no copy field', async () => {
    const sessionless = { ...agent, agent_session_id: '' };
    relayStore.agents.set([sessionless]);
    vi.spyOn(relayStore, 'sendToAgent').mockResolvedValue(result({ ...details, session_id: '', todos_available: false, todos: [] }));
    render(SessionDetails, { open: true, agent: sessionless });
    const close = screen.getByRole('button', { name: 'Close' });
    await waitFor(() => expect(close).toHaveFocus());
    for (const shiftKey of [false, true]) {
      const event = new KeyboardEvent('keydown', { key: 'Tab', shiftKey, bubbles: true, cancelable: true });
      await fireEvent(close, event);
      expect(event.defaultPrevented).toBe(true);
      expect(close).toHaveFocus();
    }
  });

  it('does not trap or restore focus after locking', async () => {
    const trigger = document.createElement('button');
    const unlock = document.createElement('button');
    document.body.append(trigger, unlock);
    trigger.focus();
    try {
      vi.spyOn(relayStore, 'sendToAgent').mockResolvedValue(result());
      render(SessionDetails, { open: true, agent });
      await waitFor(() => expect(screen.getByRole('button', { name: 'Close' })).toHaveFocus());
      const copy = screen.getByRole('button', { name: 'Copy session ID' });
      copy.focus();
      securityState.update((state) => ({ ...state, locked: true }));
      const event = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true });
      copy.dispatchEvent(event);
      expect(event.defaultPrevented).toBe(false);
      expect(copy).toHaveFocus();
      unlock.focus();
      await tick();
      await tick();
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
      expect(unlock).toHaveFocus();
    } finally { trigger.remove(); unlock.remove(); }
  });

  it.each(['close', 'cancel'])('restores focus after %s', async (action) => {
    const trigger = document.createElement('button');
    trigger.textContent = 'Open details';
    document.body.append(trigger);
    trigger.focus();
    try {
      vi.spyOn(relayStore, 'sendToAgent').mockResolvedValue(result());
      render(SessionDetails, { open: true, agent });
      await waitFor(() => expect(screen.getByRole('button', { name: 'Close' })).toHaveFocus());
      if (action === 'close') await fireEvent.click(screen.getByRole('button', { name: 'Close' }));
      else await fireEvent(screen.getByRole('dialog'), new Event('cancel', { cancelable: true }));
      await waitFor(() => expect(trigger).toHaveFocus());
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    } finally { trigger.remove(); }
  });

  it('polls only while open and stops after the route removes its agent', async () => {
    vi.useFakeTimers();
    const send = vi.spyOn(relayStore, 'sendToAgent').mockResolvedValue(result());
    const view = render(SessionDetails, { open: false, agent });
    await vi.advanceTimersByTimeAsync(30_000);
    expect(send).not.toHaveBeenCalled();
    await view.rerender({ open: true });
    await vi.advanceTimersByTimeAsync(0);
    expect(send).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(15_000);
    expect(send).toHaveBeenCalledTimes(2);
    await view.rerender({ agent: null });
    await vi.advanceTimersByTimeAsync(30_000);
    expect(send).toHaveBeenCalledTimes(2);
  });
});
