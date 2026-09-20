import { describe, expect, it } from 'vitest'
import { since } from './age'

const now = new Date('2026-09-20T12:00:00Z')
const ago = (seconds: number) => new Date(now.getTime() - seconds * 1000)

describe('since', () => {
  it('calls anything very recent moments ago', () => {
    expect(since(ago(3), now)).toBe('moments ago')
    expect(since(ago(44), now)).toBe('moments ago')
  })

  it('rounds the first minute', () => {
    expect(since(ago(60), now)).toBe('a minute ago')
    expect(since(ago(200), now)).toBe('3 minutes ago')
  })

  it('counts hours', () => {
    expect(since(ago(2 * 3600), now)).toBe('2 hours ago')
    expect(since(ago(3600), now)).toBe('1 hour ago')
  })

  it('counts days and weeks, as the mockups do', () => {
    expect(since(ago(3 * 86400), now)).toBe('3 days ago')
    expect(since(ago(35 * 86400), now)).toBe('5 weeks ago')
  })

  it('counts years past a year', () => {
    expect(since(ago(400 * 86400), now)).toBe('1 year ago')
    expect(since(ago(800 * 86400), now)).toBe('2 years ago')
  })

  it('accepts the RFC 3339 string the API sends', () => {
    expect(since('2026-09-20T10:00:00Z', now)).toBe('2 hours ago')
  })

  it('shows nothing for a time it cannot read', () => {
    expect(since('not a time', now)).toBe('')
  })

  it('does not call a clock skew the future', () => {
    expect(since(ago(-5), now)).toBe('moments ago')
  })
})
