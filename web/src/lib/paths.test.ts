import { describe, expect, it } from 'vitest'
import { base, dir, tilde } from './paths'

describe('tilde', () => {
  it('abbreviates a path under the home directory', () => {
    expect(tilde('/Users/you/dev/app-internal', '/Users/you')).toBe('~/dev/app-internal')
  })

  it('abbreviates the home directory itself', () => {
    expect(tilde('/Users/you', '/Users/you')).toBe('~')
  })

  it('tolerates a trailing slash on home', () => {
    expect(tilde('/Users/you/dev', '/Users/you/')).toBe('~/dev')
  })

  it('leaves a path outside home alone', () => {
    expect(tilde('/srv/repos/app', '/Users/you')).toBe('/srv/repos/app')
  })

  it('does not match a sibling whose name only starts the same', () => {
    expect(tilde('/Users/younger/dev', '/Users/you')).toBe('/Users/younger/dev')
  })

  it('leaves everything alone when home is unknown', () => {
    expect(tilde('/Users/you/dev', undefined)).toBe('/Users/you/dev')
  })

  it('never abbreviates against a root home', () => {
    expect(tilde('/etc/floe', '/')).toBe('/etc/floe')
  })
})

describe('base', () => {
  it('is the last segment', () => {
    expect(base('/Users/you/dev/app-public')).toBe('app-public')
  })

  it('ignores a trailing slash', () => {
    expect(base('/Users/you/dev/app-public/')).toBe('app-public')
  })
})

describe('dir', () => {
  it('is the directory a file is in', () => {
    expect(dir('/Users/you/.config/floe/pairs/a--b-1c9e44ab.json')).toBe(
      '/Users/you/.config/floe/pairs',
    )
  })

  it('is the root for a file at the root', () => {
    expect(dir('/floe.json')).toBe('/')
  })
})
