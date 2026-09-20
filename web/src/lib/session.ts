/*
 * The launch's token and the pair to open, which reach the page in the URL's
 * fragment (#token=…&pair=<id>). The browser never sends a fragment to a
 * server, which is why the token travels there — see CONTEXT.md.
 *
 * The fragment is left in place rather than cleaned away, so reloading the page
 * keeps both the token and where the user was.
 */

export type Session = { token: string; pair: string }

export function readSession(hash: string = location.hash): Session {
  const params = new URLSearchParams(hash.replace(/^#/, ''))
  return { token: params.get('token') ?? '', pair: params.get('pair') ?? '' }
}

/** Records the pair being looked at, so a reload lands back on it. */
export function writePair(pair: string): void {
  const params = new URLSearchParams(location.hash.replace(/^#/, ''))
  if (pair) params.set('pair', pair)
  else params.delete('pair')
  history.replaceState(null, '', '#' + params.toString())
}
