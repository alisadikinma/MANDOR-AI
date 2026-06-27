"use client";

import { Badge } from "@multica/ui/components/ui/badge";

// Frontmatter values can be anything YAML parses to. We render tags as chips and
// other scalar fields as "key: value" muted lines; nested objects/arrays (other
// than tags) are skipped — the body is where rich content belongs.
function toTags(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String);
  if (typeof value === "string") return value.split(/[,\s]+/).filter(Boolean);
  return [];
}

function scalar(value: unknown): string | null {
  if (value == null) return null;
  if (typeof value === "object") return null;
  return String(value);
}

export function NoteMeta({ frontmatter }: { frontmatter: Record<string, unknown> }) {
  const entries = Object.entries(frontmatter);
  if (entries.length === 0) return null;

  const tags = toTags(frontmatter.tags);
  const fields = entries.filter(([k]) => k !== "tags");

  return (
    <div className="mb-4 space-y-2 border-b border-border pb-4">
      {fields.map(([key, value]) => {
        const v = scalar(value);
        if (v === null) return null;
        return (
          <div key={key} className="flex gap-2 text-xs">
            <span className="shrink-0 font-medium text-muted-foreground">{key}</span>
            <span className="min-w-0 truncate text-foreground">{v}</span>
          </div>
        );
      })}
      {tags.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {tags.map((tag) => (
            <Badge key={tag} variant="secondary" className="text-xs">
              {tag}
            </Badge>
          ))}
        </div>
      )}
    </div>
  );
}
