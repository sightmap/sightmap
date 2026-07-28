// The single reader for content/blog. Every build script goes through this so
// a malformed post fails the build in one place instead of producing
// `undefined` in a meta tag three scripts downstream.
import fs from 'node:fs'
import path from 'node:path'
import matter from 'gray-matter'
import { marked } from 'marked'
import { z } from 'zod'

const WORDS_PER_MINUTE = 265

export const PostFrontmatterSchema = z.object({
  title: z.string().min(1),
  excerpt: z.string().min(1),
  topic: z.string().min(1),
  // ISO date only. `new Date(date + 'T00:00:00')` downstream depends on this shape.
  date: z.string().regex(/^\d{4}-\d{2}-\d{2}$/, 'must be YYYY-MM-DD'),
  author: z.string().min(1),
  slug: z
    .string()
    .regex(/^[a-z0-9]+(-[a-z0-9]+)*$/, 'must be lowercase kebab-case'),
  image: z.string().startsWith('/').optional(),
  draft: z.boolean().optional(),
})

export type PostFrontmatter = z.infer<typeof PostFrontmatterSchema>

export interface LoadedPost {
  frontmatter: PostFrontmatter
  body: string
  readingTime: number
  file: string
}

export function calculateReadingTime(body: string): number {
  const words = body.trim().split(/\s+/).filter(Boolean).length
  return Math.max(1, Math.ceil(words / WORDS_PER_MINUTE))
}

export async function renderPostHtml(body: string): Promise<string> {
  // `marked` passes raw HTML through by default, which is what lets a post
  // embed a `<div data-widget="...">` marker for the app to swap out.
  return marked(body, { async: true })
}

export async function loadPosts(
  dir: string,
  opts: { includeDrafts?: boolean } = {}
): Promise<LoadedPost[]> {
  const files = fs
    .readdirSync(dir)
    .filter((f) => f.endsWith('.md') && f.toLowerCase() !== 'readme.md')
    .sort()

  const posts: LoadedPost[] = []

  for (const file of files) {
    const full = path.join(dir, file)
    const { data, content } = matter(fs.readFileSync(full, 'utf-8'))

    const parsed = PostFrontmatterSchema.safeParse(data)
    if (!parsed.success) {
      const issues = parsed.error.issues
        .map((i) => `  - ${i.path.join('.') || '(root)'}: ${i.message}`)
        .join('\n')
      throw new Error(`Invalid frontmatter in ${file}:\n${issues}`)
    }

    // The prerender writes dist/blog/<slug>/index.html, so a slug that drifts
    // from its filename produces a page nobody can find from the repo.
    const expected = file.replace(/\.md$/, '')
    if (parsed.data.slug !== expected) {
      throw new Error(
        `Slug mismatch in ${file}: frontmatter says "${parsed.data.slug}" but the filename implies "${expected}". Rename one to match the other.`
      )
    }

    if (parsed.data.draft && !opts.includeDrafts) continue

    posts.push({
      frontmatter: parsed.data,
      body: content,
      readingTime: calculateReadingTime(content),
      file,
    })
  }

  return posts.sort(
    (a, b) =>
      new Date(b.frontmatter.date).getTime() - new Date(a.frontmatter.date).getTime()
  )
}
