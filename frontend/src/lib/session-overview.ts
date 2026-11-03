import { displayName, sessionName, tabName } from './agents';
import type { Agent } from './types';
import { homeRelativePath } from './workspaces';

export function sessionTitle(agent: Partial<Agent>): string {
  if (agent.agent?.toLowerCase().replace(/[ _-]/gu, '') === 'opencode' && !agent.agent_session_id) return 'New session';
  return [sessionName(agent), tabName(agent), displayName(agent)].find((name) => {
    if (!name || name === agent.agent_session_id || name === agent.server_session_id) return false;
    return !/^(?:ses_[\w-]+|[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12})$/i.test(name);
  }) || 'Session';
}

export function overviewPath(path: string, home = ''): string {
  const relative = homeRelativePath(path, home);
  const prefix = relative.match(/^(?:~[\\/]|[a-z]:[\\/]|[\\/]+)/i)?.[0] || '';
  const parts = relative.slice(prefix.length).split(/[\\/]/).filter(Boolean);
  if (parts.length <= 4) return relative;
  const separator = relative.includes('\\') ? '\\' : '/';
  return prefix + [...parts.slice(0, 2), '...', ...parts.slice(-2)].join(separator);
}
