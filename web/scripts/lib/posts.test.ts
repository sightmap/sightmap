import { describe, it, expect } from 'vitest'
import path from 'node:path'
import { loadPosts, calculateReadingTime, renderPostHtml } from './posts'

const fixture = (name: string) => path.resolve(__dirname, '__fixtures__', name)

describe('loadPosts', () => {
  it('loads and validates a well-formed post', async () => {
    const posts = await loadPosts(fixture('valid'))
    expect(posts).toHaveLength(1)
    expect(posts[0].frontmatter.slug).toBe('good-post')
    expect(posts[0].frontmatter.title).toBe('A good post')
    expect(posts[0].readingTime).toBe(1)
  })

  it('throws naming both the file and the missing field', async () => {
    await expect(loadPosts(fixture('invalid-missing-excerpt'))).rejects.toThrow(
      /bad-post\.md[\s\S]*excerpt/
    )
  })

  it('throws when the slug does not match the filename', async () => {
    await expect(loadPosts(fixture('invalid-slug-mismatch'))).rejects.toThrow(
      /named-one-thing\.md[\s\S]*named-another-thing/
    )
  })

  it('excludes drafts unless asked for them', async () => {
    const published = await loadPosts(fixture('valid'))
    const all = await loadPosts(fixture('valid'), { includeDrafts: true })
    expect(published).toHaveLength(1)
    expect(all).toHaveLength(1)
  })
})

describe('calculateReadingTime', () => {
  it('rounds up and never returns zero', () => {
    expect(calculateReadingTime('one two three')).toBe(1)
  })

  it('uses 265 words per minute', () => {
    expect(calculateReadingTime('word '.repeat(530))).toBe(2)
  })
})

describe('renderPostHtml', () => {
  it('renders markdown to HTML', async () => {
    expect(await renderPostHtml('# Hi')).toContain('<h1')
  })

  it('passes raw widget marker HTML through untouched', async () => {
    const html = await renderPostHtml('<div data-widget="x"><pre><code>y</code></pre></div>')
    expect(html).toContain('data-widget="x"')
    expect(html).toContain('<code>y</code>')
  })
})
