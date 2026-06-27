import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

const mockUseVaultTree = vi.fn();
const mockUseVaultNote = vi.fn();
const mockUseVaultSearch = vi.fn();
const mockUseVaultGraph = vi.fn();

const NOTE = {
  path: "b.md",
  frontmatter: { title: "Bee", tags: ["alpha", "beta"] },
  body: "# Hello body\n\ncontent here",
};

vi.mock("@multica/core/vault", () => ({
  useVaultTree: (...a: unknown[]) => mockUseVaultTree(...a),
  useVaultNote: (...a: unknown[]) => mockUseVaultNote(...a),
  useVaultSearch: (...a: unknown[]) => mockUseVaultSearch(...a),
  useVaultGraph: (...a: unknown[]) => mockUseVaultGraph(...a),
  // Real transforms aren't under test here — identity keeps the body intact.
  transformWikilinks: (md: string) => md,
  rewriteEmbeds: (md: string) => md,
}));

vi.mock("@multica/core/paths", () => ({
  useCurrentWorkspace: () => ({ id: "ws1", slug: "acme" }),
}));

vi.mock("@multica/core/api", () => ({
  api: { vaultFileUrl: (_ws: string, p: string) => `http://x/file?path=${p}` },
}));

vi.mock("../editor", () => ({
  ReadonlyContent: ({ content }: { content: string }) => (
    <div data-testid="readonly">{content}</div>
  ),
}));

import { VaultPage } from "./vault-page";

beforeEach(() => {
  mockUseVaultTree.mockReset();
  mockUseVaultNote.mockReset();
  mockUseVaultSearch.mockReset();
  mockUseVaultGraph.mockReset();
  // Empty graph → graph view shows its empty state (no react-force-graph load).
  mockUseVaultGraph.mockReturnValue({ data: { nodes: [], links: [] }, isPending: false });

  mockUseVaultTree.mockReturnValue({
    data: [
      {
        name: "folder",
        path: "folder",
        type: "dir",
        children: [{ name: "a.md", path: "folder/a.md", type: "file" }],
      },
      { name: "b.md", path: "b.md", type: "file" },
    ],
    isPending: false,
  });
  // Note query is disabled until a file is selected.
  mockUseVaultNote.mockImplementation((_ws: string, path?: string) =>
    path ? { data: NOTE, isPending: false } : { data: undefined, isPending: false },
  );
  mockUseVaultSearch.mockReturnValue({ data: [], isPending: false });
});

// Graph is the default view; the tree/note live under the "Files" tab.
function goFiles() {
  fireEvent.click(screen.getByText("Files"));
}

describe("VaultPage", () => {
  it("defaults to the graph view and shows its empty state for an empty vault", () => {
    render(<VaultPage />);
    expect(screen.getByText(/no notes yet/i)).toBeTruthy();
    expect(screen.queryByText("folder")).toBeNull(); // tree hidden until Files
  });

  it("renders the folder tree from useVaultTree", () => {
    render(<VaultPage />);
    goFiles();
    expect(screen.getByText("folder")).toBeTruthy();
    expect(screen.getByText("b")).toBeTruthy(); // .md stripped for display
  });

  it("selecting a file loads its note and renders the body via ReadonlyContent", () => {
    render(<VaultPage />);
    goFiles();
    expect(screen.queryByTestId("readonly")).toBeNull(); // nothing selected yet
    fireEvent.click(screen.getByText("b"));
    const body = screen.getByTestId("readonly");
    expect(body.textContent).toContain("Hello body");
  });

  it("renders frontmatter tags as chips", () => {
    render(<VaultPage />);
    goFiles();
    fireEvent.click(screen.getByText("b"));
    expect(screen.getByText("alpha")).toBeTruthy();
    expect(screen.getByText("beta")).toBeTruthy();
  });
});
