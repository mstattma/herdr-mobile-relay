import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import ConversationHistory from '$components/ConversationHistory.svelte';
import * as cache from '$lib/conversation-cache';
import { relayStore } from '$lib/store';
import { targetRefForAgent } from '$lib/resource-id';
import type { Agent } from '$lib/types';

const agent: Agent = {
  relay_id: 'start', relay_label: 'Start', raw_pane_id: 'pane', pane_id: 'start::pane',
  server_session_id: 'primary', terminal_id: 'terminal', generation: 1,
  agent_session_id: '', agent: 'opencode', status: 'idle', conversation_history_available: false,
};

beforeEach(() => {
  vi.spyOn(performance, 'now').mockReturnValue(100_000);
  relayStore.connections.set(new Map([[agent.relay_id, {
    status: 'connected', capabilities: ['conversation_history'], inventory: { state: 'ready', stale: false },
  }]] as never));
  vi.spyOn(relayStore, 'readPane').mockImplementation(() => {});
  vi.spyOn(relayStore, 'getConversationHistory').mockResolvedValue({ available: true, reason: '', entries: [], hasMore: false, total: 0 });
  vi.spyOn(relayStore, 'getConversationUpdates').mockImplementation(() => { throw new Error('Unexpected subscription'); });
  vi.spyOn(cache, 'getConversationPreview');
  vi.spyOn(cache, 'putConversationPreview');
  vi.spyOn(relayStore, 'sendToAgent').mockResolvedValue({ type: 'command_result', request_id: 'prompt', ok: true });
});

afterEach(() => {
  relayStore.connections.set(new Map());
  vi.restoreAllMocks();
});

describe('sessionless OpenCode conversation', () => {
  it('shows a real start state without history requests, subscriptions, cache, or a fake successful page', async () => {
    const onInitialPage = vi.fn();
    render(ConversationHistory, { agent, onInitialPage });
    expect(screen.getByText('No conversation started yet.')).toBeVisible();
    expect(screen.getByLabelText('Prompt')).toHaveAttribute('placeholder', 'Type your first prompt…');
    await waitFor(() => expect(relayStore.readPane).toHaveBeenCalledWith(agent, true, 30));
    expect(relayStore.getConversationHistory).not.toHaveBeenCalled();
    expect(relayStore.getConversationUpdates).not.toHaveBeenCalled();
    expect(cache.getConversationPreview).not.toHaveBeenCalled();
    expect(cache.putConversationPreview).not.toHaveBeenCalled();
    expect(onInitialPage).not.toHaveBeenCalled();
  });

  it('keeps guarded submission and does not raw-send when the native agent is not ready', async () => {
    vi.mocked(relayStore.sendToAgent).mockRejectedValue(Object.assign(new Error('agent_not_ready'), { data: { not_started: true } }));
    const view = render(ConversationHistory, { agent });
    await fireEvent.input(screen.getByLabelText('Prompt'), { target: { value: 'first prompt' } });
    expect(screen.getByRole('button', { name: 'Send prompt' })).toBeDisabled();
    await view.rerender({ frame: {
      paneId: agent.pane_id, content: 'Ask anything', format: 'plain', target: targetRefForAgent(agent),
      inputTarget: targetRefForAgent(agent), noEcho: false, receivedAt: 100_000, readStartedAt: 100_000,
    } });
    await fireEvent.click(screen.getByRole('button', { name: 'Send prompt' }));
    await waitFor(() => expect(screen.getByLabelText('Prompt')).toHaveValue('first prompt'));
    expect(relayStore.sendToAgent).toHaveBeenCalledExactlyOnceWith(agent, { type: 'submit_prompt', text: 'first prompt' });
  });

  it('loads only the discovered native target and retains unavailable-history fallback', async () => {
    vi.mocked(relayStore.getConversationHistory).mockResolvedValue({ available: false, reason: 'Native history missing', entries: [], hasMore: false, total: 0 });
    const onInitialPage = vi.fn();
    const view = render(ConversationHistory, { agent, onInitialPage });
    const discovered = { ...agent, generation: 2, agent_session_id: 'native-session', conversation_history_available: true };
    await view.rerender({ agent: discovered });
    await waitFor(() => expect(onInitialPage).toHaveBeenCalledWith(expect.objectContaining({ available: false })));
    expect(relayStore.getConversationHistory).toHaveBeenCalledExactlyOnceWith(discovered, expect.anything());
    expect(screen.queryByText('No conversation started yet.')).not.toBeInTheDocument();
    expect(screen.getByText('Native history missing')).toBeVisible();
  });

  it('does not present an existing session error as a new conversation', async () => {
    vi.mocked(relayStore.getConversationHistory).mockRejectedValue(new Error('History read failed'));
    const onInitialPage = vi.fn();
    render(ConversationHistory, { agent: { ...agent, agent_session_id: 'existing-session' }, onInitialPage });
    await waitFor(() => expect(screen.getByText('History read failed')).toBeVisible());
    expect(screen.queryByText('No conversation started yet.')).not.toBeInTheDocument();
    expect(onInitialPage).not.toHaveBeenCalled();
  });
});
