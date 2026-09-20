/*
 * Paths as the page shows them. floe's API sends absolute, symlink-resolved
 * paths — a pair is identified by them — so shortening is the page's to do.
 */

/**
 * tilde abbreviates a path under the home directory to "~/…". The home
 * directory itself becomes "~". A path outside it, or an unknown home, is
 * returned as it came: the paths floe shows must stay true.
 */
export function tilde(path: string, home: string | undefined): string {
  if (!home || home === '/') return path
  const root = home.endsWith('/') ? home.slice(0, -1) : home
  if (path === root) return '~'
  if (path.startsWith(root + '/')) return '~' + path.slice(root.length)
  return path
}

/** The last segment of a path — a repository's own name. */
export function base(path: string): string {
  const trimmed = path.endsWith('/') ? path.slice(0, -1) : path
  return trimmed.slice(trimmed.lastIndexOf('/') + 1)
}

/** The directory a file is in, for naming where to go and fix it. */
export function dir(path: string): string {
  const cut = path.lastIndexOf('/')
  return cut <= 0 ? '/' : path.slice(0, cut)
}
