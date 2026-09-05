export default function AboutPage() {
  return (
    <article data-component="AboutText">
      <h1>About</h1>
      <p>
        This app ships a <code>.sightmap/</code> corpus and a{" "}
        <code>.sightkick/</code> tool layer. At build time they become{" "}
        <code>/.well-known/sightmap.json</code> and{" "}
        <code>/.well-known/sightkick.json</code>, and on every page the
        Sightkick runtime registers the tools on{" "}
        <code>document.modelContext</code> — the WebMCP surface.
      </p>
      <p>
        Try it:{" "}
        <code>
          agent-browser open localhost:3000 &amp;&amp; agent-browser webmcp list
        </code>
        .
      </p>
    </article>
  );
}
