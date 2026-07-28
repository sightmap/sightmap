import { describe, it, expect } from 'vitest'
import { patchImageFrontmatter } from './generate-og-card'

describe('patchImageFrontmatter', () => {
  describe('insert branch (no existing image: line)', () => {
    it('uses LF throughout when the frontmatter block is LF', () => {
      const raw = "---\ntitle: 'A post'\nslug: 'a-post'\n---\n\nBody text.\n"
      const updated = patchImageFrontmatter(raw, '/blog/og/a-post.png', 'a-post.md')

      expect(updated).not.toBeNull()
      expect(updated).toContain("image: '/blog/og/a-post.png'")
      // No CR anywhere in a file that started as pure LF.
      expect(updated).not.toContain('\r')
      expect(updated).toBe(
        "---\ntitle: 'A post'\nslug: 'a-post'\nimage: '/blog/og/a-post.png'\n---\n\nBody text.\n"
      )
    })

    it('uses CRLF throughout when the frontmatter block is CRLF', () => {
      const raw = "---\r\ntitle: 'A post'\r\nslug: 'a-post'\r\n---\r\n\r\nBody text.\r\n"
      const updated = patchImageFrontmatter(raw, '/blog/og/a-post.png', 'a-post.md')

      expect(updated).not.toBeNull()
      expect(updated).toContain("image: '/blog/og/a-post.png'")
      // Every line ending in the block — including the inserted line — must
      // be CRLF; a bare \n not preceded by \r would prove the bug.
      expect(updated).toBe(
        "---\r\ntitle: 'A post'\r\nslug: 'a-post'\r\nimage: '/blog/og/a-post.png'\r\n---\r\n\r\nBody text.\r\n"
      )
      const bareLf = updated!.match(/[^\r]\n/g)
      expect(bareLf).toBeNull()
    })
  })

  describe('replace branch (existing image: line)', () => {
    it('rewrites the value in place without touching other lines (LF)', () => {
      const raw = "---\ntitle: 'A post'\nimage: '/old.png'\nslug: 'a-post'\n---\n\nBody.\n"
      const updated = patchImageFrontmatter(raw, '/blog/og/a-post.png', 'a-post.md')

      expect(updated).toBe(
        "---\ntitle: 'A post'\nimage: '/blog/og/a-post.png'\nslug: 'a-post'\n---\n\nBody.\n"
      )
    })

    it('rewrites the value in place without touching other lines (CRLF)', () => {
      const raw = "---\r\ntitle: 'A post'\r\nimage: '/old.png'\r\nslug: 'a-post'\r\n---\r\n\r\nBody.\r\n"
      const updated = patchImageFrontmatter(raw, '/blog/og/a-post.png', 'a-post.md')

      expect(updated).toBe(
        "---\r\ntitle: 'A post'\r\nimage: '/blog/og/a-post.png'\r\nslug: 'a-post'\r\n---\r\n\r\nBody.\r\n"
      )
    })
  })

  describe('missing frontmatter block', () => {
    it('returns null and warns instead of throwing', () => {
      const raw = 'No frontmatter here at all.\n'
      const updated = patchImageFrontmatter(raw, '/blog/og/a-post.png', 'a-post.md')
      expect(updated).toBeNull()
    })
  })
})
