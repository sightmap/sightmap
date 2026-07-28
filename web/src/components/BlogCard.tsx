import { Link } from 'react-router'
import type { BlogPostMeta } from '@/types/blog'

export function formatDate(dateStr: string): string {
  return new Date(dateStr + 'T00:00:00').toLocaleDateString('en-US', {
    month: 'long',
    day: 'numeric',
    year: 'numeric',
  })
}

export default function BlogCard({ post }: { post: BlogPostMeta }) {
  return (
    <Link to={`/blog/${post.slug}`} className="blog-card" data-component="BlogCard">
      <div className="blog-card__meta">
        <span>{post.topic}</span>
        <time dateTime={post.date}>{formatDate(post.date)}</time>
      </div>
      <div className="blog-card__title">{post.title}</div>
      <p className="blog-card__excerpt">{post.excerpt}</p>
    </Link>
  )
}
