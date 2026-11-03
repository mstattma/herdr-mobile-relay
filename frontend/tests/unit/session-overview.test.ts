import { describe, expect, it } from 'vitest';
import { overviewPath, sessionTitle } from '$lib/session-overview';

describe('session overview labels', () => {
  it.each([
    ['/mnt/c/Users/example/dev/GitHub/project', '', '/mnt/c/.../GitHub/project'],
    ['/home/example/code/team/area/packages/client', '/home/example', '~/code/team/.../packages/client'],
    ['~/code/team/area/packages/client', '', '~/code/team/.../packages/client'],
    ['C:\\Users\\example\\dev\\GitHub\\project', '', 'C:\\Users\\example\\...\\GitHub\\project'],
    ['C:\\work\\project', '', 'C:\\work\\project'],
    ['/a/b/c/d', '', '/a/b/c/d'],
    ['/home/example', '/home/example', '~'],
    ['relative/a/b/c/d', '', 'relative/a/.../c/d'],
    ['/', '', '/'],
    ['', '', ''],
  ])('abbreviates %s without losing its root', (path, home, expected) => {
    expect(overviewPath(path, home)).toBe(expected);
  });

  it('prefers session name, then tab, then display name', () => {
    expect(sessionTitle({ session_name: '  Fix tests ', tab_label: 'Tab', project: 'project' })).toBe('Fix tests');
    expect(sessionTitle({ session_name: '', tab_label: 'Tab', project: 'project' })).toBe('Tab');
    expect(sessionTitle({ session_name: '', project: 'project' })).toBe('project');
  });

  it('never uses a native ID or server ID as the title', () => {
    expect(sessionTitle({ session: 'ses_synthetic', tab_label: 'Tab' })).toBe('Tab');
    expect(sessionTitle({ session_name: 'native-fixture', agent_session_id: 'native-fixture', project: 'project' })).toBe('project');
    expect(sessionTitle({ session_name: 'primary', server_session_id: 'primary', project: 'project' })).toBe('project');
    expect(sessionTitle({ session_name: '12345678-1234-1234-1234-123456789abc', project: 'project' })).toBe('project');
  });

  it('labels sessionless OpenCode independently of its tab or workspace', () => {
    expect(sessionTitle({ agent: 'opencode', tab_label: 'Old tab', project: 'root' })).toBe('New session');
    expect(sessionTitle({ agent: 'open_code', tab_label: 'Old tab' })).toBe('New session');
    expect(sessionTitle({ agent: 'opencode', agent_session_id: 'ses_synthetic', session_name: 'New task' })).toBe('New task');
  });
});
