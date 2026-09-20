/*
 * How long ago something happened, in the words the mockups use: "2 hours
 * ago", "3 days ago", "5 weeks ago". Used for when a pair was last opened and
 * for a commit's age in the source column.
 */

const MINUTE = 60
const HOUR = 60 * MINUTE
const DAY = 24 * HOUR
const WEEK = 7 * DAY

/** How long ago `when` was, as one phrase ending in "ago". */
export function since(when: Date | string, now: Date = new Date()): string {
  const at = typeof when === 'string' ? new Date(when) : when
  const seconds = Math.round((now.getTime() - at.getTime()) / 1000)
  if (Number.isNaN(seconds)) return ''
  // A clock a little behind the file it is reading is not the future.
  if (seconds < 45) return 'moments ago'
  if (seconds < 90) return 'a minute ago'
  if (seconds < HOUR) return plural(Math.round(seconds / MINUTE), 'minute')
  if (seconds < DAY) return plural(Math.round(seconds / HOUR), 'hour')
  if (seconds < WEEK) return plural(Math.round(seconds / DAY), 'day')
  // Weeks up to a year: "5 weeks ago" is more exact than "last month", and
  // floe is read by someone deciding whether a pair is still the one they want.
  if (seconds < 365 * DAY) return plural(Math.round(seconds / WEEK), 'week')
  return plural(Math.floor(seconds / (365 * DAY)), 'year')
}

function plural(n: number, unit: string): string {
  return `${n} ${unit}${n === 1 ? '' : 's'} ago`
}
