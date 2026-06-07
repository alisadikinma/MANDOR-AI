// @vitest-environment jsdom

import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { McpConnector } from "@multica/core/types";

const mockListMcpConnectors = vi.hoisted(() => vi.fn());

vi.mock("@multica/core/api", () => ({
  api: {
    listMcpConnectors: (...args: unknown[]) => mockListMcpConnectors(...args),
  },
}));

import { ConnectorDirectory } from "./connector-directory";

function makeConnector(over: Partial<McpConnector>): McpConnector {
  return {
    id: over.id ?? "c-" + (over.slug ?? "x"),
    workspace_id: null,
    slug: over.slug ?? "x",
    name: over.name ?? "X",
    icon: null,
    description: over.description ?? null,
    popularity: over.popularity ?? 0,
    input_schema: { fields: [] },
    mcp_template: {},
    is_custom: over.is_custom ?? false,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...over,
  };
}

const connectors: McpConnector[] = [
  makeConnector({ slug: "github", name: "GitHub", popularity: 100, description: "Manage issues and PRs" }),
  makeConnector({ slug: "slack", name: "Slack", popularity: 90, description: "Read channels" }),
  makeConnector({ slug: "zeta", name: "Zeta", popularity: 1, description: "Last by popularity" }),
];

function renderDirectory(onSelect = vi.fn()) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={queryClient}>
      <ConnectorDirectory
        wsId="ws-1"
        open
        onOpenChange={() => {}}
        onSelect={onSelect}
      />
    </QueryClientProvider>,
  );
  return { onSelect };
}

beforeEach(() => {
  mockListMcpConnectors.mockReset();
  mockListMcpConnectors.mockResolvedValue(connectors);
});

describe("ConnectorDirectory", () => {
  it("renders a card per connector", async () => {
    renderDirectory();
    await waitFor(() => {
      expect(screen.getByText("GitHub")).toBeInTheDocument();
    });
    expect(screen.getByText("Slack")).toBeInTheDocument();
    expect(screen.getByText("Zeta")).toBeInTheDocument();
    expect(mockListMcpConnectors).toHaveBeenCalledWith("ws-1");
  });

  it("filters cards by search term", async () => {
    const user = userEvent.setup();
    renderDirectory();
    await waitFor(() => expect(screen.getByText("GitHub")).toBeInTheDocument());

    await user.type(screen.getByLabelText("Search connectors"), "slack");

    await waitFor(() => {
      expect(screen.queryByText("GitHub")).not.toBeInTheDocument();
    });
    expect(screen.getByText("Slack")).toBeInTheDocument();
  });

  it("shows an empty state when nothing matches", async () => {
    const user = userEvent.setup();
    renderDirectory();
    await waitFor(() => expect(screen.getByText("GitHub")).toBeInTheDocument());

    await user.type(screen.getByLabelText("Search connectors"), "no-such-thing");

    await waitFor(() => {
      expect(screen.getByText("No connectors found")).toBeInTheDocument();
    });
  });

  it("calls onSelect with the connector when its + is clicked", async () => {
    const user = userEvent.setup();
    const { onSelect } = renderDirectory();
    await waitFor(() => expect(screen.getByText("GitHub")).toBeInTheDocument());

    await user.click(screen.getByLabelText("Add Slack"));

    expect(onSelect).toHaveBeenCalledTimes(1);
    expect(onSelect.mock.calls[0]?.[0]?.slug).toBe("slack");
  });

  it("sorts by popularity by default (most popular first)", async () => {
    renderDirectory();
    await waitFor(() => expect(screen.getByText("GitHub")).toBeInTheDocument());

    const headings = screen.getAllByRole("heading", { level: 3 }).map((h) => h.textContent);
    expect(headings).toEqual(["GitHub", "Slack", "Zeta"]);
  });

  it("reorders to alphabetical when sort is toggled to Name", async () => {
    const user = userEvent.setup();
    renderDirectory();
    await waitFor(() => expect(screen.getByText("GitHub")).toBeInTheDocument());

    await user.click(screen.getByRole("button", { name: /^Sort:/ }));

    await waitFor(() => {
      const headings = screen
        .getAllByRole("heading", { level: 3 })
        .map((h) => h.textContent);
      expect(headings).toEqual(["GitHub", "Slack", "Zeta"]);
    });
    // The sort control label flipped to Name mode.
    expect(screen.getByRole("button", { name: "Sort: Name" })).toBeInTheDocument();
  });
});
