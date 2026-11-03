import type { PromptMaxLines } from './config';

export function composerHeightLimit(lines: PromptMaxLines, lineHeight: number, padding: number, border: number, automatic: number, viewportHeight: number): number {
  const requested = lines === 'automatic' ? automatic : lines * lineHeight + padding + border;
  // Leave most of the keyboard-visible viewport for the header, content and controls.
  return Math.max(lineHeight + padding + border, Math.min(requested, viewportHeight * 0.4));
}

export function resizePromptComposer(element: HTMLTextAreaElement, lines: PromptMaxLines, viewportHeight: number): void {
  // Measuring a hidden sibling avoids temporarily collapsing the focused input,
  // which makes browsers scroll its ancestors (and the header) while typing.
  const measure = element.cloneNode(false) as HTMLTextAreaElement;
  measure.removeAttribute('id');
  measure.removeAttribute('name');
  measure.removeAttribute('role');
  measure.setAttribute('aria-hidden', 'true');
  measure.tabIndex = -1;
  measure.value = element.value;
  measure.style.cssText = `position:absolute; visibility:hidden; pointer-events:none; top:0; left:0; height:0; width:${element.getBoundingClientRect().width}px; overflow:hidden;`;
  element.parentElement!.append(measure);
  try {
    const style = getComputedStyle(measure);
    const px = (value: string) => Number.parseFloat(value) || 0;
    const padding = px(style.paddingTop) + px(style.paddingBottom);
    const border = px(style.borderTopWidth) + px(style.borderBottomWidth);
    const lineHeight = px(style.lineHeight);
    const minimum = px(style.minHeight);
    const maximum = composerHeightLimit(lines, lineHeight, padding, border, px(style.maxHeight), viewportHeight);
    measure.style.minHeight = '0';
    measure.style.maxHeight = 'none';
    const content = measure.scrollHeight + border;
    element.style.minHeight = `${Math.min(minimum, maximum)}px`;
    element.style.maxHeight = `${maximum}px`;
    element.style.height = `${Math.max(Math.min(minimum, maximum), Math.min(content, maximum))}px`;
    element.style.overflowY = content > maximum ? 'auto' : 'hidden';
  } finally {
    measure.remove();
  }
}
