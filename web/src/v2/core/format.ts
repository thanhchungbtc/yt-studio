/** Formatting, in the register macOS list views use. */

const time = new Intl.DateTimeFormat(undefined, { hour: 'numeric', minute: '2-digit' })
const weekday = new Intl.DateTimeFormat(undefined, { weekday: 'short' })
const date = new Intl.DateTimeFormat(undefined, {
  day: 'numeric',
  month: 'numeric',
  year: '2-digit',
})

/**
 * The timestamp a Messages row carries: the time today, the weekday this week,
 * a date before that. Never "3 days ago" — a relative phrase makes a list of
 * them read as prose, and the eye is scanning, not reading.
 */
export function listTimestamp(iso: string): string {
  const at = new Date(iso)
  if (Number.isNaN(at.getTime())) return ''

  const now = new Date()
  const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const days = Math.floor((startOfToday.getTime() - at.getTime()) / 86_400_000)

  if (days < 0) return time.format(at)
  if (days === 0) return 'Yesterday'
  if (days < 6) return weekday.format(at)
  return date.format(at)
}
