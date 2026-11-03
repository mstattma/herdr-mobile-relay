import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import PromptComposer from '$components/PromptComposer.svelte';
import { relayStore } from '$lib/store';
import { targetRefForAgent } from '$lib/resource-id';
import { terminalInputDelivery } from '$lib/terminal-input-delivery.svelte';
import { securityState } from '$lib/security';
import type { Agent, CommandResult, TerminalFrame } from '$lib/types';

const agent: Agent = {
  relay_id: 'cycle', relay_label: 'Cycle', raw_pane_id: 'pane', pane_id: 'cycle::pane',
  server_session_id: 'primary', terminal_id: 'terminal', generation: 1,
  agent_session_id: '', agent: 'opencode', status: 'idle',
};
const completed: CommandResult = { type: 'command_result', request_id: 'cycle', ok: true, phase: 'sent', data: { sent: true } };
const nativePrompt = [
  'Historical response about questions and permissions.', '',
  '  \u2503', '  \u2503  ', '  \u2503', '  \u2503  Plan \u00b7 Example Model ExampleProvider \u00b7 high',
  `  \u2579${'\u2580'.repeat(96)}`, '   /work/example  12.0K (4%)  ctrl+p commands', '',
].join('\n');
const freshFrame = (current = agent): TerminalFrame => ({
  paneId: current.pane_id, content: nativePrompt, format: 'plain',
  target: targetRefForAgent(current), inputTarget: targetRefForAgent(current),
  receivedAt: 100_000, readStartedAt: 100_000, noEcho: false,
});

beforeEach(() => {
  vi.spyOn(performance, 'now').mockReturnValue(100_000);
  relayStore.connections.set(new Map([[agent.relay_id, {
    status: 'connected', capabilities: ['agent_cycle'], inventory: { state: 'ready', stale: false },
  }]] as never));
  vi.spyOn(relayStore, 'deviceCredential').mockReturnValue({ role: 'controller' } as never);
  vi.spyOn(relayStore, 'readPane').mockImplementation(() => {});
  vi.spyOn(relayStore, 'showToast').mockImplementation(() => {});
  vi.spyOn(relayStore, 'sendToAgent').mockResolvedValue(completed);
});

afterEach(() => {
  terminalInputDelivery(agent).finish(false);
  terminalInputDelivery(agent).inspect(100_001);
  relayStore.connections.set(new Map());
  securityState.update((state) => ({ ...state, locked: false }));
  vi.restoreAllMocks();
});

describe('OpenCode agent cycling', () => {
  it('sends one guarded cycle command, not the draft, and reports Tab rather than a selected mode', async () => {
    let finish!: (result: CommandResult) => void;
    vi.mocked(relayStore.sendToAgent).mockImplementation(() => new Promise((resolve) => { finish = resolve; }));
    const view = render(PromptComposer, { agent });
    expect(screen.getByRole('button', { name: 'Switch agent' })).toBeDisabled();
    await view.rerender({ frame: freshFrame() });
    const button = screen.getByRole('button', { name: 'Switch agent' });
    expect(button).toBeEnabled();
    await fireEvent.input(screen.getByLabelText('Prompt'), { target: { value: 'keep this draft' } });
    await fireEvent.click(button);
    await fireEvent.click(button);
    expect(relayStore.sendToAgent).toHaveBeenCalledExactlyOnceWith(agent, { type: 'cycle_agent' });
    expect(screen.getByLabelText('Prompt')).toHaveValue('keep this draft');
    expect(button).toBeDisabled();
    finish(completed);
    await waitFor(() => expect(relayStore.showToast).toHaveBeenCalledWith('Tab sent to OpenCode. Check the terminal for the active agent.', false));
    expect(screen.getByLabelText('Prompt')).toHaveValue('keep this draft');
    expect(button).toBeDisabled();
    await view.rerender({ frame: freshFrame() });
    expect(button).toBeEnabled();
    expect(button).toHaveAttribute('title', expect.stringContaining('Custom keybindings are not supported'));
  });

  it.each(['reader', 'locked', 'disconnected', 'inventory', 'capability', 'hidden-input', 'stale', 'wrong-target', 'picker', 'approval', 'question', 'loading', 'pending', 'uncertain'] as const)('blocks cycling for %s', async (blocked) => {
    const current = { ...agent,
      ...(blocked === 'approval' ? { status: 'blocked', attention_kind: 'approval' as const } : {}),
      ...(blocked === 'question' ? { status: 'blocked', attention_kind: 'question' as const } : {}),
      ...(blocked === 'loading' ? { status: 'starting' } : {}),
    };
    const view = render(PromptComposer, { agent: current, readOnly: blocked === 'reader', locked: blocked === 'locked' });
    const frame = freshFrame(current);
    if (blocked === 'hidden-input') frame.noEcho = true;
    if (blocked === 'stale') frame.receivedAt = 96_000;
    if (blocked === 'wrong-target') frame.inputTarget = targetRefForAgent({ ...current, terminal_id: 'replacement' });
    if (blocked === 'picker') frame.content = 'Type to filter | Enter to select | Esc to close';
    if (blocked === 'disconnected' || blocked === 'inventory' || blocked === 'capability') {
      relayStore.connections.set(new Map([[agent.relay_id, {
        status: blocked === 'disconnected' ? 'disconnected' : 'connected',
        capabilities: blocked === 'capability' ? [] : ['agent_cycle'],
        inventory: { state: blocked === 'inventory' ? 'starting' : 'ready', stale: false },
      }]] as never));
    }
    if (blocked === 'pending') terminalInputDelivery(agent).begin();
    if (blocked === 'uncertain') terminalInputDelivery(agent).finish(true);
    await view.rerender({ frame });
    const button = screen.getByRole('button', { name: 'Switch agent' });
    expect(button).toBeDisabled();
    await fireEvent.click(button);
    expect(relayStore.sendToAgent).not.toHaveBeenCalled();
  });

  it('does not expose the action for other agents', () => {
    render(PromptComposer, { agent: { ...agent, agent: 'codex' } });
    expect(screen.queryByRole('button', { name: 'Switch agent' })).not.toBeInTheDocument();
  });

  it('checks controller credentials and page visibility independently of the parent props', async () => {
    const view = render(PromptComposer, { agent });
    vi.mocked(relayStore.deviceCredential).mockReturnValue({ role: 'reader' } as never);
    await view.rerender({ frame: freshFrame() });
    expect(screen.getByRole('button', { name: 'Switch agent' })).toBeDisabled();
    vi.mocked(relayStore.deviceCredential).mockReturnValue({ role: 'controller' } as never);
    await view.rerender({ frame: freshFrame() });
    expect(screen.getByRole('button', { name: 'Switch agent' })).toBeEnabled();
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden');
    await fireEvent(document, new Event('visibilitychange'));
    expect(screen.getByRole('button', { name: 'Switch agent' })).toBeDisabled();
  });

  it('requires the normal native prompt rather than any fresh terminal text', async () => {
    const view = render(PromptComposer, { agent });
    await view.rerender({ frame: { ...freshFrame(), content: 'Loading OpenCode...' } });
    expect(screen.getByRole('button', { name: 'Switch agent' })).toBeDisabled();
  });

  it('keeps a native draft blocked until the user clears it without changing the browser draft', async () => {
    const view = render(PromptComposer, { agent });
    await fireEvent.input(screen.getByLabelText('Prompt'), { target: { value: 'local browser draft' } });
    await view.rerender({ frame: { ...freshFrame(), content: nativePrompt.replace('\n  \u2503  \n', '\n  \u2503  Unsent native prompt\n') } });
    const button = screen.getByRole('button', { name: 'Switch agent' });
    expect(button).toBeDisabled();
    await fireEvent.click(button);
    expect(relayStore.sendToAgent).not.toHaveBeenCalled();
    expect(screen.getByLabelText('Prompt')).toHaveValue('local browser draft');
    await view.rerender({ frame: freshFrame() });
    expect(button).toBeEnabled();
    expect(screen.getByLabelText('Prompt')).toHaveValue('local browser draft');
  });

  it('keeps uncertain delivery blocked without retrying or claiming an agent selection', async () => {
    vi.mocked(relayStore.sendToAgent).mockRejectedValue(new Error('Connection lost'));
    const view = render(PromptComposer, { agent });
    await view.rerender({ frame: freshFrame() });
    await fireEvent.click(screen.getByRole('button', { name: 'Switch agent' }));
    await waitFor(() => expect(terminalInputDelivery(agent).uncertain).toBe(true));
    expect(relayStore.sendToAgent).toHaveBeenCalledTimes(1);
    expect(screen.getByRole('button', { name: 'Switch agent' })).toBeDisabled();
    expect(relayStore.showToast).toHaveBeenCalledWith('Connection lost Check Terminal before trying again.', true);
  });
});
