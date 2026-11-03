import { get } from 'svelte/store';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import PromptComposer from '$components/PromptComposer.svelte';
import { PROMPT_MAX_LINES_KEY } from '$lib/config';
import { interfaceSize, promptMaxLines, readPromptMaxLines, setPromptMaxLines } from '$lib/preferences';
import { composerHeightLimit, resizePromptComposer } from '$lib/composer-sizing';
import * as sizing from '$lib/composer-sizing';

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  promptMaxLines.set('automatic');
  interfaceSize.set('compact');
});

describe('prompt line preference', () => {
  it.each([null, '', '0', '1', '21', '2.5', 'NaN', 'Infinity', '02', ' 2', '2e0', 'automatic'])('uses Automatic for %s', (value) => {
    expect(readPromptMaxLines({ getItem: () => value })).toBe('automatic');
  });

  it('persists every bounded integer independently of Interface Size', () => {
    for (let lines = 2; lines <= 20; lines++) {
      expect(setPromptMaxLines(lines)).toBe('saved');
      expect(readPromptMaxLines()).toBe(lines);
      expect(get(promptMaxLines)).toBe(lines);
    }
    interfaceSize.set('large');
    expect(get(promptMaxLines)).toBe(20);
    expect(localStorage.getItem(PROMPT_MAX_LINES_KEY)).toBe('20');
    expect(setPromptMaxLines('automatic')).toBe('saved');
    expect(readPromptMaxLines()).toBe('automatic');
  });

  it('rejects invalid writes and storage failures without changing the selection', () => {
    setPromptMaxLines(4);
    for (const invalid of [1, 21, 2.5, NaN, Infinity]) expect(setPromptMaxLines(invalid)).toBe('unavailable');
    vi.spyOn(localStorage, 'setItem').mockImplementation(() => { throw new Error('full'); });
    expect(setPromptMaxLines(8)).toBe('unavailable');
    expect(get(promptMaxLines)).toBe(4);
    expect(readPromptMaxLines({ getItem: () => { throw new Error('denied'); } })).toBe('automatic');
  });
});

describe('prompt sizing', () => {
  it('recalculates on draft, preference, size, width and keyboard viewport changes without replacing the input', async () => {
    const viewport = Object.assign(new EventTarget(), { height: 800 });
    vi.stubGlobal('visualViewport', viewport);
    let notifyResize = () => {};
    vi.stubGlobal('ResizeObserver', class {
      constructor(callback: ResizeObserverCallback) { notifyResize = () => callback([], this); }
      observe() {}
      unobserve() {}
      disconnect() {}
    });
    let width = 200;
    vi.spyOn(HTMLTextAreaElement.prototype, 'getBoundingClientRect').mockImplementation(() => ({ width }) as DOMRect);
    const resize = vi.spyOn(sizing, 'resizePromptComposer').mockImplementation(() => {});
    render(PromptComposer, { agent: {
      relay_id: 'size', relay_label: 'Size', raw_pane_id: 'pane', pane_id: 'size::pane',
      server_session_id: 'primary', terminal_id: 'terminal', generation: 1, agent: 'codex',
    } });
    const input = screen.getByLabelText('Prompt');
    await waitFor(() => expect(resize).toHaveBeenCalledWith(input, 'automatic', 800));
    resize.mockClear();
    await fireEvent.input(input, { target: { value: 'keep this draft' } });
    await waitFor(() => expect(resize).toHaveBeenCalled());
    resize.mockClear();
    promptMaxLines.set(6);
    await waitFor(() => expect(resize).toHaveBeenCalledWith(input, 6, 800));
    resize.mockClear();
    interfaceSize.set('large');
    await waitFor(() => expect(resize).toHaveBeenCalled());
    resize.mockClear();
    width = 160;
    notifyResize();
    await waitFor(() => expect(resize).toHaveBeenCalled());
    resize.mockClear();
    viewport.height = 300;
    viewport.dispatchEvent(new Event('resize'));
    await waitFor(() => expect(resize).toHaveBeenCalledWith(input, 6, 300));
    expect(screen.getByLabelText('Prompt')).toBe(input);
    expect(input).toHaveValue('keep this draft');
  });

  it('uses actual line height and box metrics, preserving Automatic caps', () => {
    expect(composerHeightLimit(4, 22.4, 16, 2, 160, 900)).toBeCloseTo(107.6);
    expect(composerHeightLimit(4, 28, 24, 2, 216, 900)).toBe(138);
    for (const automatic of [112, 160, 216]) {
      expect(composerHeightLimit('automatic', 22, 16, 0, automatic, 900)).toBe(automatic);
    }
    expect(composerHeightLimit(20, 22, 16, 2, 160, 300)).toBe(120);
  });

  it('grows, caps, scrolls internally and shrinks without replacing the focused draft', () => {
    const parent = document.createElement('div');
    const input = document.createElement('textarea');
    parent.append(input);
    document.body.append(parent);
    vi.spyOn(globalThis, 'getComputedStyle').mockReturnValue({
      paddingTop: '8px', paddingBottom: '8px', borderTopWidth: '1px', borderBottomWidth: '1px',
      lineHeight: '20px', minHeight: '38px', maxHeight: '160px',
    } as CSSStyleDeclaration);
    let contentHeight = 56;
    vi.spyOn(HTMLTextAreaElement.prototype, 'scrollHeight', 'get').mockImplementation(() => contentHeight);
    input.value = 'draft text';
    input.focus();
    input.setSelectionRange(3, 5);
    input.scrollTop = 12;
    const scroll = vi.spyOn(window, 'scrollTo');
    resizePromptComposer(input, 4, 900);
    expect(input.style.height).toBe('58px');
    contentHeight = 500;
    resizePromptComposer(input, 4, 900);
    expect(input.style.height).toBe('98px');
    expect(input.style.overflowY).toBe('auto');
    resizePromptComposer(input, 20, 300);
    expect(input.style.height).toBe('120px');
    contentHeight = 36;
    resizePromptComposer(input, 4, 900);
    expect(input.style.height).toBe('38px');
    expect(input.style.overflowY).toBe('hidden');
    expect(parent.children).toHaveLength(1);
    expect(document.activeElement).toBe(input);
    expect(input.value).toBe('draft text');
    expect([input.selectionStart, input.selectionEnd, input.scrollTop]).toEqual([3, 5, 12]);
    expect(scroll).not.toHaveBeenCalled();
    parent.remove();
  });
});
