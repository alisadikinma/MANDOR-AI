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

  it("renders inherited servers in a read-only Workspace group", async () => {
    render(
      <InstalledConnectorList
        servers={[{ name: "own", summary: "node a.js", enabled: true }]}
        inherited={[{ name: "shared", summary: "npx shared", enabled: true }]}
        onRemove={noop}
        onToggle={noop}
      />,
    );
    // Scope headers appear only when there is an inherited group.
    expect(screen.getByText("Workspace")).toBeInTheDocument();
    expect(screen.getByText("This agent")).toBeInTheDocument();
    // Inherited rows are not interactive (no "Manage" affordance) and carry
    // an Inherited badge; the agent's own row stays clickable.
    expect(
      screen.queryByRole("button", { name: /manage shared/i }),
    ).not.toBeInTheDocument();
    expect(screen.getByText("Inherited")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /manage own/i }),
    ).toBeInTheDocument();
  });

  it("shows the empty hint only when both inherited and own are empty", () => {
    const { rerender } = render(
      <InstalledConnectorList servers={[]} onRemove={noop} onToggle={noop} />,
    );
    expect(screen.getByText(/no connectors yet/i)).toBeInTheDocument();

    // With inherited servers but no own servers, the Workspace group shows and
    // the own group degrades to an "add your own" hint, not the top-level one.
    rerender(
      <InstalledConnectorList
        servers={[]}
        inherited={[{ name: "shared", summary: "", enabled: true }]}
        onRemove={noop}
        onToggle={noop}
      />,
    );
    expect(screen.getByText("Workspace")).toBeInTheDocument();
    expect(screen.getByText(/no agent-specific connectors/i)).toBeInTheDocument();
  });

  it("renders live probe status (connected/failed/needs-auth) over enabled/disabled", () => {
    render(
      <InstalledConnectorList
        servers={[
          { name: "ok", summary: "", enabled: true },
          { name: "broken", summary: "", enabled: true },
          { name: "oauth", summary: "", enabled: true },
        ]}
        liveStatus={{
          ok: { name: "ok", status: "connected", tool_count: 3 },
          broken: { name: "broken", status: "failed", tool_count: 0, error: "boom" },
          oauth: { name: "oauth", status: "needs_auth", tool_count: 0 },
        }}
        onRemove={noop}
        onToggle={noop}
      />,
    );
    expect(screen.getByText("Connected · 3 tools")).toBeInTheDocument();
    expect(screen.getByText("Failed")).toBeInTheDocument();
    expect(screen.getByText("Needs auth")).toBeInTheDocument();
    // The static Enabled pill is replaced by the live status.
    expect(screen.queryByText("Enabled")).not.toBeInTheDocument();
  });

  it("shows a Checking pill on every row while a probe is in flight", () => {
    render(
      <InstalledConnectorList
        servers={[{ name: "a", summary: "", enabled: true }]}
        probing
        onRemove={noop}
        onToggle={noop}
      />,
    );
    expect(screen.getByText("Checking…")).toBeInTheDocument();
  });

  it("surfaces the failure error in the detail panel", async () => {
    const user = userEvent.setup();
    render(
      <InstalledConnectorList
        servers={[{ name: "broken", summary: "", enabled: true }]}
        liveStatus={{
          broken: { name: "broken", status: "failed", tool_count: 0, error: "spawn ENOENT" },
        }}
        onRemove={noop}
        onToggle={noop}
      />,
    );
    await user.click(screen.getByRole("button", { name: /manage broken/i }));
    expect(screen.getByText("spawn ENOENT")).toBeInTheDocument();
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

  it("offers an Authenticate button for a needs_auth server when onAuthenticate is set", async () => {
    const user = userEvent.setup();
    const onAuthenticate = vi.fn();
    render(
      <InstalledConnectorList
        servers={[{ name: "figma", summary: "", enabled: true }]}
        liveStatus={{
          figma: { name: "figma", status: "needs_auth", tool_count: 0 },
        }}
        onAuthenticate={onAuthenticate}
        onRemove={noop}
        onToggle={noop}
      />,
    );
    await user.click(screen.getByRole("button", { name: /manage figma/i }));
    const authBtn = screen.getByRole("button", { name: "Authenticate" });
    await user.click(authBtn);
    expect(onAuthenticate).toHaveBeenCalledWith("figma");
    // The CLI fallback hint must NOT show when the in-app flow is available.
    expect(screen.queryByText(/run on the runtime host/i)).not.toBeInTheDocument();
  });

  it("falls back to the runtime-CLI hint when onAuthenticate is absent", async () => {
    const user = userEvent.setup();
    render(
      <InstalledConnectorList
        servers={[{ name: "figma", summary: "", enabled: true }]}
        liveStatus={{
          figma: { name: "figma", status: "needs_auth", tool_count: 0 },
        }}
        onRemove={noop}
        onToggle={noop}
      />,
    );
    await user.click(screen.getByRole("button", { name: /manage figma/i }));
    expect(
      screen.queryByRole("button", { name: "Authenticate" }),
    ).not.toBeInTheDocument();
    expect(screen.getByText(/sign-in happens on the runtime/i)).toBeInTheDocument();
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
