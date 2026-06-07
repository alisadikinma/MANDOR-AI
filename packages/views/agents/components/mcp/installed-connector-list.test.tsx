// @vitest-environment jsdom

import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import {
  InstalledConnectorList,
  extractInstalledServers,
} from "./installed-connector-list";

describe("extractInstalledServers", () => {
  it("lists each mcpServers entry with a secret-free summary, marked enabled", () => {
    const servers = extractInstalledServers({
      mcpServers: {
        obsidian: { command: "node", args: ["/path/main.js", "/vault"] },
        firecrawl: { url: "https://mcp.firecrawl.dev/sse" },
        kiro: { transport: "streamable-http" },
      },
    });
    expect(servers).toEqual([
      { name: "firecrawl", summary: "https://mcp.firecrawl.dev/sse", enabled: true },
      { name: "kiro", summary: "streamable-http", enabled: true },
      { name: "obsidian", summary: "node /path/main.js", enabled: true },
    ]);
  });

  it("includes disabledMcpServers as disabled, sorted after active", () => {
    const servers = extractInstalledServers({
      mcpServers: { active: { command: "a" } },
      disabledMcpServers: { paused: { command: "p" } },
    });
    expect(servers).toEqual([
      { name: "active", summary: "a", enabled: true },
      { name: "paused", summary: "p", enabled: false },
    ]);
  });

  it("collapses malformed shapes to an empty list instead of throwing", () => {
    expect(extractInstalledServers(null)).toEqual([]);
    expect(extractInstalledServers("nope")).toEqual([]);
    expect(extractInstalledServers({})).toEqual([]);
    expect(extractInstalledServers({ mcpServers: [] })).toEqual([]);
    expect(extractInstalledServers({ disabledMcpServers: [] })).toEqual([]);
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
    expect(servers).toEqual([
      { name: "firecrawl", summary: "firecrawl-mcp", enabled: true },
    ]);
    expect(servers[0]?.summary).not.toContain("fc-secret");
  });
});

describe("InstalledConnectorList", () => {
  const noop = vi.fn();

  it("renders an empty hint when no servers are configured", () => {
    render(<InstalledConnectorList servers={[]} onRemove={noop} onToggle={noop} />);
    expect(screen.getByText(/no connectors yet/i)).toBeInTheDocument();
  });

  it("drills into a server detail panel on click", async () => {
    const user = userEvent.setup();
    render(
      <InstalledConnectorList
        servers={[{ name: "obsidian", summary: "node main.js", enabled: true }]}
        onRemove={noop}
        onToggle={noop}
      />,
    );
    await user.click(screen.getByRole("button", { name: /manage obsidian/i }));
    expect(screen.getByText(/back to list/i)).toBeInTheDocument();
    expect(screen.getByText("Configuration")).toBeInTheDocument();
  });

  it("toggles an enabled server to disabled via the detail panel", async () => {
    const user = userEvent.setup();
    const onToggle = vi.fn();
    render(
      <InstalledConnectorList
        servers={[{ name: "obsidian", summary: "", enabled: true }]}
        onRemove={noop}
        onToggle={onToggle}
      />,
    );
    await user.click(screen.getByRole("button", { name: /manage obsidian/i }));
    await user.click(screen.getByRole("button", { name: "Disable" }));
    expect(onToggle).toHaveBeenCalledWith("obsidian", false);
  });

  it("enables a disabled server via the detail panel", async () => {
    const user = userEvent.setup();
    const onToggle = vi.fn();
    render(
      <InstalledConnectorList
        servers={[{ name: "paused", summary: "", enabled: false }]}
        onRemove={noop}
        onToggle={onToggle}
      />,
    );
    await user.click(screen.getByRole("button", { name: /manage paused/i }));
    await user.click(screen.getByRole("button", { name: "Enable" }));
    expect(onToggle).toHaveBeenCalledWith("paused", true);
  });

  it("removes a server only after the inline confirm", async () => {
    const user = userEvent.setup();
    const onRemove = vi.fn();
    render(
      <InstalledConnectorList
        servers={[{ name: "obsidian", summary: "node main.js", enabled: true }]}
        onRemove={onRemove}
        onToggle={noop}
      />,
    );
    await user.click(screen.getByRole("button", { name: /manage obsidian/i }));

    // Arms the confirm — must NOT remove yet.
    await user.click(screen.getByRole("button", { name: /remove obsidian/i }));
    expect(onRemove).not.toHaveBeenCalled();

    // Confirming fires the remove with the server name.
    await user.click(screen.getByRole("button", { name: "Remove" }));
    expect(onRemove).toHaveBeenCalledWith("obsidian");
  });

  it("cancel dismisses the remove confirm without removing", async () => {
    const user = userEvent.setup();
    const onRemove = vi.fn();
    render(
      <InstalledConnectorList
        servers={[{ name: "obsidian", summary: "", enabled: true }]}
        onRemove={onRemove}
        onToggle={noop}
      />,
    );
    await user.click(screen.getByRole("button", { name: /manage obsidian/i }));
    await user.click(screen.getByRole("button", { name: /remove obsidian/i }));
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onRemove).not.toHaveBeenCalled();
    expect(
      screen.getByRole("button", { name: /remove obsidian/i }),
    ).toBeInTheDocument();
  });
});
