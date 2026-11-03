import { get } from 'svelte/store';
import { targetRefForAgent, targetRefMatchesAgent } from './resource-id';
import { securityState } from './security';
import { relayStore } from './store';
import type { Agent } from './types';

export interface SessionDetailsData {
  session_id: string;
  title: string;
  cwd: string;
  todos: { content: string; status: string; priority: string }[];
  todos_available: boolean;
  subscriptions?: {
    available: boolean;
    rows: {
      label: string;
      provider: string;
      status: string;
      detail?: string;
      weeklyReset?: string;
      limits?: string;
      paid?: string;
      current?: boolean;
    }[];
    observed_at?: number;
  };
  reason?: string;
}

export async function loadSessionDetails(agent: Agent, signal: AbortSignal): Promise<SessionDetailsData> {
  const target = targetRefForAgent(agent);
  const current = () => {
    const connection = get(relayStore.connections).get(agent.relay_id);
    return !signal.aborted && !get(securityState).locked && target
      && connection?.status === 'connected' && connection.inventory.state === 'ready'
      && connection.capabilities.includes('session_details')
      && get(relayStore.agents).some((candidate) => targetRefMatchesAgent(target, candidate));
  };
  if (!current()) throw new Error('Session details are unavailable for this target.');
  const result = await relayStore.sendToAgent(agent, { type: 'get_session_details' }, 10_000, signal);
  if (!current()) throw new Error('The session details target changed.');
  const data = result.data as SessionDetailsData | undefined;
  if (!result.ok || !data || data.session_id !== (agent.agent_session_id || '')) {
    throw new Error('Session details did not match this session.');
  }
  return data;
}

export async function copySessionField(input: HTMLInputElement): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(input.value);
    return true;
  } catch {
    if (input.isConnected) {
      input.focus();
      input.select();
    }
    return false;
  }
}
