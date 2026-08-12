export default function BlogFilters({
  topics,
  activeTopic,
  onTopicChange,
  query,
  onQueryChange,
}: {
  topics: string[]
  activeTopic: string | null
  onTopicChange: (topic: string | null) => void
  query: string
  onQueryChange: (query: string) => void
}) {
  return (
    <div className="blog-filters" data-component="BlogFilters">
      <input
        className="blog-filters__search"
        type="search"
        value={query}
        onChange={(e) => onQueryChange(e.target.value)}
        placeholder="Search posts"
        aria-label="Search posts"
      />
      <div className="blog-filters__topics">
        <button
          type="button"
          className="blog-filters__topic"
          aria-pressed={activeTopic === null}
          onClick={() => onTopicChange(null)}
        >
          All
        </button>
        {topics.map((topic) => (
          <button
            key={topic}
            type="button"
            className="blog-filters__topic"
            aria-pressed={activeTopic === topic}
            onClick={() => onTopicChange(topic)}
          >
            {topic}
          </button>
        ))}
      </div>
    </div>
  )
}
