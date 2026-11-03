import { describe, expect, it } from 'vitest';
import { openCodeCyclePrompt } from '$lib/opencode-agent-cycle';

const footer = '/work/example  12.0K (4%)  ctrl+p commands';
function prompt(input = '', hint = footer, sidebar = false): string {
  const rows = [
    'Historical response about questions and permissions.', '',
    '  \u2503', `  \u2503  ${input}`, '  \u2503',
    '  \u2503  Plan \u00b7 Example Model ExampleProvider \u00b7 high',
    `  \u2579${'\u2580'.repeat(96)}`, `   ${hint}`, '',
  ];
  const labels = ['Example session', 'Context', '12,000 tokens', 'Todo', 'Review question handling', '/work/example', '', '\u2022 OpenCode 1.18.29', ''];
  return rows.map((row, index) => sidebar ? `${row.padEnd(99)}  ${labels[index]}` : row).join('\n');
}

function home(placeholder = 'Ask anything\u2026 "Fix broken tests"'): string {
  return prompt(placeholder, 'tab agents  ctrl+p commands').split('\n').map((row) => ' '.repeat(40) + row).join('\n')
    + '\n\n\n   \u25cf Tip Press tab to cycle agents' + '\n'.repeat(12) + '  /work/example  \u2299 2 MCP /status       1.18.29\n';
}

const session = prompt();
const sidebar = prompt('', footer, true);

describe('OpenCode empty native composer recognition', () => {
  it.each([
    ['session without placeholder or tab hint', session],
    ['session with sidebar', sidebar],
    ['session with ANSI colors', '\x1b[38;2;224;108;117m\x1b[48;2;30;30;30m' + sidebar.replaceAll('\n', '\x1b[0m\n\x1b[38;2;224;108;117m') + '\x1b[0m'],
    ['session with CRLF', session.replaceAll('\n', '\r\n')],
    ['session with cost', prompt('', '/work/example  12.0K (4%) \u00b7 $1.23  ctrl+p commands', true)],
    ['historical questions and permissions', '# Questions\nWhich option?\n1. First\n2. Second\nAsked 2 questions\nRejected permission earlier.\n' + session],
    ['historical modal vocabulary', 'Discuss esc, escape, select model, switch agent, autocomplete and type to filter.\n' + session],
    ['centered home with tip and footer', home()],
    ['home with ASCII placeholder', home('Ask anything... "Fix broken tests"')],
  ])('accepts %s', (_, content) => {
    expect(openCodeCyclePrompt(content)).toBe(true);
  });

  it.each([
    ['native unsent text', prompt('Review the example changes', footer, true)],
    ['native placeholder-like draft', prompt('Ask anything... "Fix broken tests"', footer, true)],
    ['native multiline draft', session.replace('\n\n  \u2503', '\n\n  \u2503 Draft above the visible empty row\n  \u2503')],
    ['slash autocomplete', prompt('/new', footer, true)],
    ['mention autocomplete', prompt('@example')],
    ['autocomplete touching composer', session.replace('\n\n  \u2503', '\n\u2503 /new  Start a new session\n  \u2503')],
    ['permission replacing composer', '\n\u2503 \u25b3 Permission required\n\u2503 Allow once  Allow always  Reject\n\u2503 \u21c6 select  enter confirm'],
    ['permission controls overlay', '\u2503 \u25b3 Permission required\n\u2503 Allow once  Allow always  Reject\n\u2503 \u21c6 select  enter confirm\n' + session],
    ['question replacing composer', '\n\u2503 Which option?\n\u2503 1. First\n\u2503 2. Second\n\u2503 \u2191\u2193 select  enter submit  esc dismiss'],
    ['question controls overlay', 'Which option?\n1. First\n2. Second\n\u2191\u2193 select  enter submit  esc dismiss\n' + session],
    ['rejection textarea', '\u2503 Reject permission\n\u2503 Tell OpenCode what to do differently\n\u2503 enter confirm  esc cancel\n' + session],
    ['model picker overlay', 'Select model                      esc\nSearch\nExample Model\n' + sidebar],
    ['other modal', 'Rename session                    esc\nEnter text\n' + session],
    ['help modal', 'Help                    esc/enter\nPress ctrl+p to see all available actions\n  ok\n' + session],
    ['filter footer', prompt('', 'Type to filter | Enter to select | Esc to close')],
    ['unknown overlay', session + '\nUnknown popup'],
    ['missing command hint', prompt('', '/work/example  12.0K (4%)')],
    ['unknown footer', prompt('', 'Unknown control  12.0K (4%)  ctrl+p commands')],
    ['working footer', prompt('', '[busy] esc interrupt  12.0K (4%)  ctrl+p commands')],
    ['shell mode', session.replace('Plan \u00b7 Example Model ExampleProvider \u00b7 high', 'Shell')],
    ['missing input row', session.replace('\n  \u2503  \n', '\n')],
    ['missing top separator', session.replace('\n\n  \u2503', '\n  \u2503')],
    ['missing border', session.replaceAll('\u2580', '-')],
    ['unframed log text', 'Ask anything...\nBuild Example Model\ntab agents'],
    ['home without native footer', prompt('Ask anything\u2026 "Fix broken tests"', 'tab agents  ctrl+p commands')],
    ['home unknown trailing control', home() + 'New control'],
    ['unknown shell', '$ '],
    ['too many rows', '\n'.repeat(256) + session],
    ['oversized frame', 'x'.repeat(128 * 1024) + session],
    ['oversized main column', session.replace('\u2580'.repeat(96), '\u2580'.repeat(4096))],
  ])('rejects %s', (_, content) => {
    expect(openCodeCyclePrompt(content)).toBe(false);
  });
});
