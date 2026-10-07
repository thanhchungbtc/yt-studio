import { OpenURL } from '@bindings/services/systemservice'

/** Opens a link in the default browser; window.open does nothing in the webview. */
export async function openExternal(url: string): Promise<boolean> {
  try {
    await OpenURL(url)
    return true
  } catch {
    return false
  }
}
