import { describe, expect, it } from 'vitest'
import { parseDotEnv } from './parse-dotenv'

describe('parseDotEnv', () => {
  it('parses keys, export, quotes, and skips comments', () => {
    const got = parseDotEnv(`
# comment
FOO=bar
export BAZ="qux"
EMPTY=
'SKIP'
BAD
QUOTED='hello world'
`)
    expect(got).toEqual({
      FOO: 'bar',
      BAZ: 'qux',
      EMPTY: '',
      QUOTED: 'hello world',
    })
  })
})
