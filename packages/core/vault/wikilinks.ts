/**
 * Pure transforms that turn Obsidian wikilink/embed syntax into standard
 * markdown the shared ReadonlyContent renderer understands. No React, no DOM —
 * safe to unit-test and to run inside packages/core.
 */

// Matches both [[link]] and ![[embed]]; capture 1 is the leading "!" (empty for
// a plain link), capture 2 is the inner "target" or "target|alias".
const WIKILINK_RE = /(!?)\[\[([^\]]+)\]\]/g;

export type ResolveLink = (name: string) => string | null;

function splitAlias(inner: string): [target: string, alias: string | undefined] {
  const idx = inner.indexOf("|");
  if (idx === -1) return [inner.trim(), undefined];
  return [inner.slice(0, idx).trim(), inner.slice(idx + 1).trim()];
}

/**
 * Replace `[[Note]]` / `[[Note|alias]]` with markdown links whose href comes
 * from `resolve(name)`. Embeds (`![[...]]`) are skipped — run rewriteEmbeds for
 * those. An unresolved link (resolve → null) degrades to plain text so a dead
 * wikilink is never a broken clickable.
 */
export function transformWikilinks(md: string, resolve: ResolveLink): string {
  return md.replace(WIKILINK_RE, (match, bang: string, inner: string) => {
    if (bang === "!") return match; // embed — not our job
    const [name, alias] = splitAlias(inner);
    const href = resolve(name);
    const label = alias ?? name;
    return href ? `[${label}](${href})` : label;
  });
}

/**
 * Replace `![[target]]` / `![[target|alias]]` embeds with markdown images whose
 * src comes from `toFileUrl(target)`. Plain wikilinks are left untouched.
 *
 * `target` is treated as a vault-relative path. Obsidian's shortest-unique-name
 * resolution is intentionally not implemented for v1 — author full relative
 * paths in embeds, or add a name→path index later if it becomes a felt gap.
 */
export function rewriteEmbeds(md: string, toFileUrl: (target: string) => string): string {
  return md.replace(WIKILINK_RE, (match, bang: string, inner: string) => {
    if (bang !== "!") return match; // plain link — not an embed
    const [target, alias] = splitAlias(inner);
    return `![${alias ?? target}](${toFileUrl(target)})`;
  });
}
