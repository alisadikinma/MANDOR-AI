// @vitest-environment jsdom

import { describe, it, expect, vi, beforeEach } from "vitest";
import type { ReactNode } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { Agent } from "@multica/core/types";
import { I18nProvider } from "@multica/core/i18n/react";
import enCommon from "../../../locales/en/common.json";
import enAgents from "../../../locales/en/agents.json";

const TEST_RESOURCES = { en: { common: enCommon, agents: enAgents } };

// The tab reads its runtime's pool via api.getRuntimeMcp (through
// runtimeMcpOptions). Mock the API and let the real query run.
const getRuntimeMcp = vi.fn();
vi.mock("@multica/core/api", () => ({
  api: { getRuntimeMcp: (...a: unknown[]) => getRuntimeMcp(...a) },
}));

vi.mock("sonner", () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}));

import { McpConfigTab } from "./mcp-config-tab";

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
    getRuntimeMcp.mockResolvedValue({ servers: [], probe_results: [] });
  });

  it("renders a read-only redacted state without any checkboxes", () => {
    renderTab({ mcp_config: null, mcp_config_redacted: true });
    expect(screen.getByText(/hidden from your view/i)).toBeInTheDocument();
    expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
    // Redacted view must not even fetch the pool.
    expect(getRuntimeMcp).not.toHaveBeenCalled();
  });

  it("shows an empty hint and reports not-dirty when the runtime has no servers", async () => {
    const { onDirtyChange } = renderTab({ mcp_config: null });
    expect(
      await screen.findByText(/reports no MCP servers/i),
    ).toBeInTheDocument();
    expect(onDirtyChange).toHaveBeenCalledWith(false);
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
  });

  it("lists each runtime server; a disabled one is unchecked", async () => {
    getRuntimeMcp.mockResolvedValue({
      servers: [
        { name: "github", transport: "stdio" },
        { name: "figma", transport: "http", url: "https://figma.example/mcp" },
      ],
      probe_results: [],
    });
    renderTab({ mcp_config: { disabledMcpServers: ["figma"] } });

    expect(await screen.findByText("github")).toBeInTheDocument();
    expect(screen.getByText("figma")).toBeInTheDocument();
    const boxes = screen.getAllByRole("checkbox");
    expect(boxes[0]).toBeChecked(); // github inherited (enabled)
    expect(boxes[1]).not.toBeChecked(); // figma disabled
  });

  it("disabling a server writes the deny-list via onSave", async () => {
    getRuntimeMcp.mockResolvedValue({
      servers: [{ name: "github", transport: "stdio" }],
      probe_results: [],
    });
    const { onSave } = renderTab({ mcp_config: null });
    const box = await screen.findByRole("checkbox");
    await userEvent.click(box); // uncheck → disable github
    await waitFor(() =>
      expect(onSave).toHaveBeenCalledWith({
        mcp_config: { disabledMcpServers: ["github"] },
      }),
    );
  });

  it("re-enabling the last disabled server clears mcp_config to null", async () => {
    getRuntimeMcp.mockResolvedValue({
      servers: [{ name: "github", transport: "stdio" }],
      probe_results: [],
    });
    const { onSave } = renderTab({ mcp_config: { disabledMcpServers: ["github"] } });
    const box = await screen.findByRole("checkbox");
    await userEvent.click(box); // check → enable github (deny-list empties)
    await waitFor(() =>
      expect(onSave).toHaveBeenCalledWith({ mcp_config: null }),
    );
  });
});
