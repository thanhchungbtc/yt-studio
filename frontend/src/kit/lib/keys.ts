/** True while an IME is composing (e.g. Japanese input): Enter confirms the conversion, not the field. */
export function isComposing(e: { nativeEvent?: KeyboardEvent; isComposing?: boolean; keyCode?: number }): boolean {
  return !!(e.nativeEvent?.isComposing || e.isComposing || e.keyCode === 229);
}

/** Whether the focused element takes text input. */
export function isTextField(el: Element | null = document.activeElement): boolean {
  if (!(el instanceof HTMLElement)) return false;
  return el.tagName === "TEXTAREA" || el.isContentEditable || (el.tagName === "INPUT" && !/^(button|checkbox|radio|range|submit|reset|color|file)$/i.test((el as HTMLInputElement).type));
}
