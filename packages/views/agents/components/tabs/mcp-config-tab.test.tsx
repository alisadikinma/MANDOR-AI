// @vitest-environment jsdom

import { describe, it, expect, vi, beforeEach } from "vitest";
import type { ReactNode } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { Agent } from "@multica/core/types";
import { I18nProvider } from "@multica/core/i18n/react";
import enCommon from "../../../locales/en/common.json";
import enAgents from "../../../locales/en/agents.json";

const TEST_RESOURCES = { en: { common: enCommon, agents: enAgents } };

// McpConfigTab mounts the connector directory (a TanStack Query consumer),
// so every render needs a QueryClient + I18n provider in scope.
function Providers({ children }: { children: ReactNode }) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return (
    <QueryClientProvider client={queryClient}>
      <I18nProvider locale="en" resources={TEST_RESOURCES}>
        {children}
      </I18nProvider>
    </QueryClientProvider>
  );
}

vi.mock("sonner", () => ({
  toast: {
    error: vi.fn(),
    success: vi.fn(),
  },
}));

import { McpConfigTab } from "./mcp-config-tab";

const baseAgent: Agent = {
  id: "agent-1",
  workspace_id: "ws-1",
  runtime_id: "runtime-1",
  name: "Agent",
  description: "",
  instructions: "",
  avatar_url: null,
  runtime_mode: "local",
  runtime_config: {},
  custom_args: [],
  visibility: "workspace",
  status: "idle",
  max_concurrent_tasks: 1,
  model: "",
  owner_id: "user-1",
  skills: [],
  created_at: "2026-05-28T00:00:00Z",
  updated_at: "2026-05-28T00:00:00Z",
  archived_at: null,
  archived_by: null,
};

function renderTab(
  overrides: Partial<Agent> = {},
  onSave = vi.fn().mockResolvedValue(undefined),
  onDirtyChange = vi.fn(),
) {
  const agent = { ...baseAgent, ...overrides };
  const result = render(
    <Providers>
      <McpConfigTab
        agent={agent}
        onSave={onSave}
        onDirtyChange={onDirtyChange}
      />
    </Providers>,
  );
  return { ...result, onSave, onDirtyChange };
}

describe("McpConfigTab", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders a read-only redacted state when the server omitted the value", () => {
    // mcp_config_redacted means the server knows there IS a config but
    // hid it from this caller. The tab must NOT expose any editor or input —
    // not even an empty one a non-privileged member could overwrite from.
    renderTab({ mcp_config: null, mcp_config_redacted: true });

    expect(screen.getByText(/hidden from your view/i)).toBeInTheDocument();
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
    expect(
      screen.queryByLabelText(/installed connectors/i),
    ).not.toBeInTheDocument();
  });

  it("offers Browse connectors and an empty hint when no servers are set", () => {
    const { onDirtyChange } = renderTab({ mcp_config: null });

    expect(
      screen.getByRole("button", { name: /browse connectors/i }),
    ).toBeInTheDocument();
    expect(screen.getByText(/no connectors yet/i)).toBeInTheDocument();
    // The list-only tab has no draft, so it must report not-dirty so the
    // parent never raises its discard-changes dialog for MCP.
    expect(onDirtyChange).toHaveBeenCalledWith(false);
    // No raw JSON editor remains.
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
  });

  it("lists each configured server from mcp_config", () => {
    renderTab({
      mcp_config: {
        mcpServers: {
          obsidian: { command: "node", args: ["main.js"] },
          firecrawl: { command: "npx", args: ["-y", "firecrawl-mcp"] },
        },
      },
    });

    expect(screen.getByText("obsidian")).toBeInTheDocument();
    expect(screen.getByText("firecrawl")).toBeInTheDocument();
  });

  it("removing a connector saves the config without that server", async () => {
    const user = userEvent.setup();
    const { onSave } = renderTab({
      mcp_config: {
        mcpServers: {
          obsidian: { command: "node", args: ["main.js"] },
          firecrawl: { command: "npx" },
        },
      },
    });

    await user.click(screen.getByRole("button", { name: /remove obsidian/i }));
    await user.click(screen.getByRole("button", { name: "Remove" }));

    expect(onSave).toHaveBeenCalledWith({
      mcp_config: { mcpServers: { firecrawl: { command: "npx" } } },
    });
  });

  it("removing the last connector clears the config to null", async () => {
    const user = userEvent.setup();
    const { onSave } = renderTab({
      mcp_config: { mcpServers: { obsidian: { command: "node" } } },
    });

    await user.click(screen.getByRole("button", { name: /remove obsidian/i }));
    await user.click(screen.getByRole("button", { name: "Remove" }));

    // null is what the backend reads as "clear this column" so the daemon
    // falls back to the CLI default instead of persisting an empty husk.
    expect(onSave).toHaveBeenCalledWith({ mcp_config: null });
  });
});
