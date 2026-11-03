import { stripAnsi } from './terminal';

const BORDER = /^ *\u2579\u2580{16,}/u;
const MODEL = /^[A-Za-z][A-Za-z0-9 _-]{0,79} \u00b7 \S[^\r\n]*$/u;
const PLACEHOLDER = /^Ask anything(?:\.{3}|\u2026) "[^"\r\n]+"$/u;
const HINT = /^(?:(?:ctrl\+[a-z] variants|[/~][^\r\n]*|[A-Za-z]:[\\/][^\r\n]*)\s{2,})?(?:tab agents|[0-9][0-9.,]*[kKmMbBtT]?(?: \([0-9]+%\))?(?: \u00b7 \$[0-9.,]+)?)\s+ctrl\+p commands$/u;
const HOME_FOOTER = /^(?:(?:[/~]|[A-Za-z]:[\\/])[^\r\n]*\s{2,})?(?:\u2299 \d+ MCP \/status\s{2,})?\d+\.\d+\.\d+(?:[-+][A-Za-z0-9._-]+)?$/u;
const MODAL = /\S[^\r\n]*[ \t]{2,}esc(?:\/enter)?(?:[ \t]{2,}|[ \t]*$)/iu;
const DIALOG_CONTROLS = /(?:\benter (?:confirm|submit|toggle)\b[^\r\n]*\besc (?:cancel|dismiss)\b|\u21c6 select[^\r\n]*\benter confirm\b)/iu;

// Match the stock 1.18.29 layout, as in question.OpenCodeCyclePrompt. This is
// only a UI gate: the backend re-reads the native pane before sending Tab.
export function openCodeCyclePrompt(content: string): boolean {
  if (new TextEncoder().encode(content).length > 128 * 1024) return false;
  const lines = stripAnsi(content).replace(/\r/gu, '').split('\n');
  if (lines.length > 256) return false;
  let bottom = -1;
  let left = 0;
  let right = 0;
  for (let index = lines.length - 1; index >= 0; index--) {
    const border = lines[index].match(BORDER)?.[0];
    if (!border) continue;
    bottom = index;
    left = border.indexOf('\u2579');
    right = [...border].length;
    break;
  }
  if (bottom < 5 || bottom + 1 >= lines.length || right > 4096) return false;
  // Keep indentation and crop at the border, excluding the right sidebar.
  const main = (index: number) => [...lines[index]].slice(0, right).join('');
  const start = bottom - 4;
  if (main(start - 1).trim()) return false;
  let input = '';
  for (let index = start; index < bottom; index++) {
    const row = [...main(index)];
    if (row.length <= left || row.slice(0, left).join('').trim() || row[left] !== '\u2503') return false;
    const value = row.slice(left + 1).join('').trim();
    if (index === start || index === start + 2) {
      if (value) return false;
    } else if (index === start + 1) {
      input = value;
      if (value && !PLACEHOLDER.test(value)) return false;
    } else if (!MODEL.test(value)) return false;
  }
  const footer = main(bottom + 1).trim();
  if (!HINT.test(footer) || input && !footer.includes('tab agents')) return false;
  const home = Boolean(input);
  let tip = false;
  let homeFooter = false;
  for (let index = bottom + 2; index < lines.length; index++) {
    const line = (home ? lines[index] : main(index)).trim();
    if (!line) continue;
    if (home && !homeFooter && !tip && index >= bottom + 5 && line.startsWith('\u25cf Tip ')) tip = true;
    else if (home && !homeFooter && HOME_FOOTER.test(line)) homeFooter = true;
    else return false;
  }
  if (home && !homeFooter) return false;
  return !lines.some((line) => MODAL.test(line) || DIALOG_CONTROLS.test(line));
}
