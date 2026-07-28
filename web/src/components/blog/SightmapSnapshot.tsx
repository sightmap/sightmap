// Stub for Task A6, which replaces this wholesale with the real interactive
// figure. Exists here only so widgets.tsx (Task A5) has something to import
// and `tsc -b` passes.
export default function SightmapSnapshot({ figure }: { figure?: string }) {
  void figure
  return null
}
