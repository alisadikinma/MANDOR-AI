import { describe, it, expect } from "vitest";
import { parseWithFallback } from "./schema";
import {
  VaultStatusSchema,
  EMPTY_VAULT_STATUS,
  VaultTreeSchema,
  EMPTY_VAULT_TREE,
  VaultNoteSchema,
  EMPTY_VAULT_NOTE,
  VaultSearchResultsSchema,
  EMPTY_VAULT_SEARCH_RESULTS,
} from "./schemas";

// Per CLAUDE.md "API Response Compatibility": every vault endpoint must survive a
// malformed body by returning its fallback, never throwing into the UI.
describe("vault schemas — malformed responses fall back, never throw", () => {
  const opts = { endpoint: "test" };

  it("status: missing field → enabled false", () => {
    expect(parseWithFallback({}, VaultStatusSchema, EMPTY_VAULT_STATUS, opts)).toEqual({
      enabled: false,
    });
  });

  it("status: wrong type → fallback", () => {
    expect(
      parseWithFallback({ enabled: "yes" }, VaultStatusSchema, EMPTY_VAULT_STATUS, opts).enabled,
    ).toBe(false);
  });

  it("tree: non-array body → empty tree", () => {
    expect(parseWithFallback({ nope: 1 }, VaultTreeSchema, EMPTY_VAULT_TREE, opts)).toEqual([]);
  });

  it("tree: null → empty tree", () => {
    expect(parseWithFallback(null, VaultTreeSchema, EMPTY_VAULT_TREE, opts)).toEqual([]);
  });

  it("tree: valid nested tree parses through", () => {
    const tree = [
      { name: "folder", path: "folder", type: "dir", children: [{ name: "a.md", path: "folder/a.md", type: "file" }] },
    ];
    const got = parseWithFallback(tree, VaultTreeSchema, EMPTY_VAULT_TREE, opts);
    expect(got).toHaveLength(1);
    expect(got[0]?.children?.[0]?.name).toBe("a.md");
  });

  it("note: missing body / frontmatter → safe defaults", () => {
    const got = parseWithFallback({ path: "x.md" }, VaultNoteSchema, EMPTY_VAULT_NOTE, opts);
    expect(got.body).toBe("");
    expect(got.frontmatter).toEqual({});
  });

  it("note: null array frontmatter (wrong type) → fallback, no throw", () => {
    const got = parseWithFallback(
      { path: "x.md", frontmatter: [], body: 5 },
      VaultNoteSchema,
      EMPTY_VAULT_NOTE,
      opts,
    );
    // body:5 is the wrong type → whole parse fails → fallback returned.
    expect(got).toEqual(EMPTY_VAULT_NOTE);
  });

  it("search: null array → empty list", () => {
    expect(
      parseWithFallback(null, VaultSearchResultsSchema, EMPTY_VAULT_SEARCH_RESULTS, opts),
    ).toEqual([]);
  });

  it("search: valid results parse through", () => {
    const got = parseWithFallback(
      [{ name: "a.md", path: "a.md", snippet: "…hit…" }],
      VaultSearchResultsSchema,
      EMPTY_VAULT_SEARCH_RESULTS,
      opts,
    );
    expect(got[0]?.snippet).toBe("…hit…");
  });
});
