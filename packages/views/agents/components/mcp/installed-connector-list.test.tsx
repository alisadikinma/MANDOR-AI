// @vitest-environment jsdom

import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import {
  InstalledConnectorList,
  extractInstalledServers,
} from "./installed-connector-list";

describe("extractInstalledServers", () => {
  it("lists each mcpServers entry with a secret-free summary", () => {
    const servers = extractInstalledServers({
      mcpServers: {
        obsidian: { command: "node", args: ["/path/main.js", "/vault"] },
        firecrawl: { url: "https://mcp.firecrawl.dev/sse" },
        kiro: { transport: "streamable-http" },
      },
    });
    expect(servers).toEqual([
      { name: "obsidian", summary: "node /path/main.js" },
      { name: "firecrawl", summary: "https://mcp.firecrawl.dev/sse" },
      { name: "kiro", summary: "streamable-http" },
    ]);
  });

  it("collapses malformed shapes to an empty list instead of throwing", () => {
    expect(extractInstalledServers(null)).toEqual([]);
    expect(extractInstalledServers("nope")).toEqual([]);
    expect(extractInstalledServers({})).toEqual([]);
    expect(extractInstalledServers({ mcpServers: [] })).toEqual([]);
  });

  it("never leaks env/headers into the summary", () => {
    const servers = extractInstalledServers({
      mcpServers: {
        firecrawl: {
          command: "firecrawl-mcp",
          env: { FIRECRAWL_API_KEY: "fc-secret" },
        },
      },
    });
    expect(servers).toEqual([{ name: "firecrawl", summary: "firecrawl-mcp" }]);
    expect(servers[0]?.summary).not.toContain("fc-secret");
  });
});

describe("InstalledConnectorList", () => {
  it("renders an empty hint when no servers are configured", () => {
    render(<InstalledConnectorList servers={[]} onRemove={vi.fn()} />);
    expect(screen.getByText(/no connectors yet/i)).toBeInTheDocument();
  });

  it("removes a server only after the inline confirm", async () => {
    const user = userEvent.setup();
    const onRemove = vi.fn();
    render(
      <InstalledConnectorList
        servers={[{ name: "obsidian", summary: "node main.js" }]}
        onRemove={onRemove}
      />,
    );

    // First click arms the confirm — it must NOT remove yet.
    await user.click(screen.getByRole("button", { name: /remove obsidian/i }));
    expect(onRemove).not.toHaveBeenCalled();

    // Confirming fires the remove with the server name.
    await user.click(screen.getByRole("button", { name: "Remove" }));
    expect(onRemove).toHaveBeenCalledWith("obsidian");
  });

  it("cancel dismisses the confirm without removing", async () => {
    const user = userEvent.setup();
    const onRemove = vi.fn();
    render(
      <InstalledConnectorList
        servers={[{ name: "obsidian", summary: "" }]}
        onRemove={onRemove}
      />,
    );
    await user.click(screen.getByRole("button", { name: /remove obsidian/i }));
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onRemove).not.toHaveBeenCalled();
    // Back to the armed trash button.
    expect(
      screen.getByRole("button", { name: /remove obsidian/i }),
    ).toBeInTheDocument();
  });
});
