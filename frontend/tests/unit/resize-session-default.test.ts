import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, expect, it, vi } from 'vitest';
import TerminalView from '$components/TerminalView.svelte';
import { readTerminalResizeSession, terminalHeightLease, terminalResizeSession } from '$lib/preferences';
import { relayStore } from '$lib/store';
import type { Agent } from '$lib/types';

const agent: Agent = {
  relay_id: 'resize-default', relay_label: 'Synthetic', raw_pane_id: 'pane',
  pane_id: 'resize-default::pane', server_session_id: 'primary', terminal_id: 'terminal',
  generation: 1, agent_session_id: 'session', agent: 'codex', project: 'Synthetic', status: 'working',
};

afterEach(() => {
  vi.restoreAllMocks();
  relayStore.connections.set(new Map());
  terminalResizeSession.set(false);
  terminalHeightLease.set(false);
});

it.each([
  { name: 'unsupported controller', readOnly: false, capabilities: [] },
  { name: 'legacy-only controller', readOnly: false, capabilities: ['pane_size_lease'] },
  { name: 'capable reader', readOnly: true, capabilities: ['pane_size_opt_in', 'pane_size_opt_in_rows'] },
])('keeps the $name terminal usable and quiet with the resize default on', async ({ readOnly, capabilities }) => {
  terminalResizeSession.set(readTerminalResizeSession());
  expect(get(terminalResizeSession)).toBe(true);
  expect(get(terminalHeightLease)).toBe(false);
  relayStore.connections.set(new Map([[agent.relay_id, { status: 'connected', capabilities }]] as never));
  const lease = vi.spyOn(relayStore, 'leasePaneSize');
  const release = vi.spyOn(relayStore, 'releasePaneSize');
  const toast = vi.spyOn(relayStore, 'showToast');
  for (const method of ['readPane', 'watchPane', 'unwatchPane'] as const) vi.spyOn(relayStore, method).mockImplementation(() => {});
  const view = render(TerminalView, {
    agent, allAgents: [agent], responding: new Set<string>(), readOnly,
    frame: { paneId: agent.pane_id, format: 'plain', content: 'Synthetic terminal output' },
  });
  await waitFor(() => expect(screen.getByRole('log')).toHaveTextContent('Synthetic terminal output'));
  expect(lease).not.toHaveBeenCalled();
  expect(release).not.toHaveBeenCalled();
  expect(toast).not.toHaveBeenCalled();
  expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  view.unmount();
});

it.each([false, true])('keeps OpenCode in a strict grid without leasing (reader=%s)', async (readOnly) => {
  terminalResizeSession.set(true);
  terminalHeightLease.set(true);
  relayStore.connections.set(new Map([[agent.relay_id, {
    status: 'connected', capabilities: ['pane_size_opt_in', 'pane_size_opt_in_rows'],
  }]] as never));
  const lease = vi.spyOn(relayStore, 'leasePaneSize');
  const release = vi.spyOn(relayStore, 'releasePaneSize');
  for (const method of ['readPane', 'watchPane', 'unwatchPane'] as const) vi.spyOn(relayStore, method).mockImplementation(() => {});
  const fixedAgent = { ...agent, agent: 'opencode' };
  const rows = ['Main'.padEnd(80) + 'Sidebar  ', '', '', ' '.repeat(90), 'Synthetic footer'];
  const view = render(TerminalView, {
    agent: fixedAgent, allAgents: [fixedAgent], responding: new Set<string>(), readOnly,
    frame: { paneId: agent.pane_id, format: 'ansi', content: rows.join('\n') },
  });
  await waitFor(() => expect(screen.getByRole('log').querySelectorAll('.terminal-grid-line')).toHaveLength(rows.length));
  const terminal = screen.getByRole('log');
  expect(terminal).toHaveClass('strict-grid');
  expect(terminal).not.toHaveClass('resize-layout');
  expect(terminal).not.toHaveClass('resize-pending');
  expect([...terminal.querySelectorAll('.ansi-line')].map((row) => row.textContent)).toEqual(rows);
  expect(screen.getByText(/Fixed terminal layout; scroll sideways/)).toBeInTheDocument();
  await fireEvent(window, new Event('resize'));
  expect(relayStore.readPane).toHaveBeenCalled();
  expect(lease).not.toHaveBeenCalled();
  view.unmount();
  expect(release).not.toHaveBeenCalled();
});

it('releases an existing lease and rerenders identical content when entering OpenCode fixed mode', async () => {
  terminalResizeSession.set(true);
  relayStore.connections.set(new Map([[agent.relay_id, {
    status: 'connected', capabilities: ['pane_size_opt_in'],
  }]] as never));
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue(new DOMRect(0, 0, 80, 18));
  vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(320);
  const lease = vi.spyOn(relayStore, 'leasePaneSize').mockResolvedValue({ columns: 40, rows: 40 });
  const release = vi.spyOn(relayStore, 'releasePaneSize').mockResolvedValue(undefined);
  for (const method of ['readPane', 'watchPane', 'unwatchPane'] as const) vi.spyOn(relayStore, method).mockImplementation(() => {});
  const view = render(TerminalView, {
    agent, allAgents: [agent], responding: new Set<string>(),
    frame: { paneId: agent.pane_id, format: 'ansi', content: 'Same physical row   ' },
  });
  await waitFor(() => expect(lease).toHaveBeenCalled());
  const leaseCount = lease.mock.calls.length;
  await view.rerender({ agent: { ...agent, agent: 'opencode' } });
  await waitFor(() => expect(release).toHaveBeenCalledTimes(1));
  await waitFor(() => expect(screen.getByRole('log').querySelector('.terminal-grid-line')?.textContent).toBe('Same physical row   '));
  await fireEvent(window, new Event('resize'));
  expect(lease).toHaveBeenCalledTimes(leaseCount);
  view.unmount();
  expect(release).toHaveBeenCalledTimes(1);
});
