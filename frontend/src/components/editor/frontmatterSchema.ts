import type { FrontmatterSchema } from '../../types';

// The schema is generated server-side from the Go struct and does not depend on a
// campaign, so it is fetched once per session and shared by every editor. Owning
// the fetch here rather than taking it as a prop is deliberate: an editor that
// silently loses its completion because a caller forgot to pass the schema is a
// worse failure than a redundant request.
let cached: FrontmatterSchema | null = null;
let inflight: Promise<FrontmatterSchema | null> | null = null;

export async function loadFrontmatterSchema(): Promise<FrontmatterSchema | null> {
  if (cached) return cached;
  if (inflight) return inflight;

  inflight = fetch('/api/schema/entity-frontmatter')
    .then((res) => (res.ok ? (res.json() as Promise<FrontmatterSchema>) : null))
    .then((schema) => {
      cached = schema;
      return schema;
    })
    .catch(() => null)
    .finally(() => {
      inflight = null;
    });

  return inflight;
}
