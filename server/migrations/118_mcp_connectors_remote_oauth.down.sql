-- Restore the token/stdio definitions for the connectors migrated to remote
-- OAuth in the up migration.

UPDATE mcp_connector
SET description = 'Manage issues, pull requests, repositories, and code search across your GitHub organization.',
    input_schema = '{"fields": [{"key": "GITHUB_PERSONAL_ACCESS_TOKEN", "label": "Personal Access Token", "type": "password", "required": true, "placeholder": "ghp_xxxxxxxxxxxxxxxxxxxx", "help": "Create a fine-grained PAT at github.com/settings/tokens with repo and read:org scopes."}]}'::jsonb,
    mcp_template = '{"mcpServers": {"github": {"command": "npx", "args": ["-y", "@modelcontextprotocol/server-github"], "env": {"GITHUB_PERSONAL_ACCESS_TOKEN": "{{GITHUB_PERSONAL_ACCESS_TOKEN}}"}}}}'::jsonb,
    updated_at = now()
WHERE slug = 'github' AND workspace_id IS NULL;

UPDATE mcp_connector
SET description = 'Search, read, and update pages and databases in your Notion workspace.',
    input_schema = '{"fields": [{"key": "NOTION_TOKEN", "label": "Internal Integration Token", "type": "password", "required": true, "placeholder": "ntn_xxxxxxxxxxxx", "help": "Create an internal integration at notion.so/my-integrations and share the relevant pages with it."}]}'::jsonb,
    mcp_template = '{"mcpServers": {"notion": {"command": "npx", "args": ["-y", "@notionhq/notion-mcp-server"], "env": {"NOTION_TOKEN": "{{NOTION_TOKEN}}"}}}}'::jsonb,
    updated_at = now()
WHERE slug = 'notion' AND workspace_id IS NULL;

UPDATE mcp_connector
SET description = 'Read design context, components, and variables from your Figma files.',
    input_schema = '{"fields": [{"key": "FIGMA_API_KEY", "label": "Personal Access Token", "type": "password", "required": true, "placeholder": "figd_xxxxxxxxxxxx", "help": "Generate a personal access token from Figma Settings > Account > Personal access tokens."}]}'::jsonb,
    mcp_template = '{"mcpServers": {"figma": {"command": "npx", "args": ["-y", "figma-developer-mcp", "--stdio"], "env": {"FIGMA_API_KEY": "{{FIGMA_API_KEY}}"}}}}'::jsonb,
    updated_at = now()
WHERE slug = 'figma' AND workspace_id IS NULL;

UPDATE mcp_connector
SET description = 'Work with Jira issues and Confluence pages across your Atlassian Cloud site.',
    input_schema = '{"fields": [{"key": "JIRA_URL", "label": "Atlassian Site URL", "type": "url", "required": true, "placeholder": "https://your-domain.atlassian.net", "help": "The base URL of your Atlassian Cloud site."}, {"key": "JIRA_USERNAME", "label": "Account Email", "type": "text", "required": true, "placeholder": "you@example.com", "help": "The email address of the Atlassian account the API token belongs to."}, {"key": "JIRA_API_TOKEN", "label": "API Token", "type": "password", "required": true, "placeholder": "ATATT3xFfGF0xxxx", "help": "Create an API token at id.atlassian.com/manage-profile/security/api-tokens."}]}'::jsonb,
    mcp_template = '{"mcpServers": {"atlassian": {"command": "npx", "args": ["-y", "mcp-atlassian"], "env": {"JIRA_URL": "{{JIRA_URL}}", "JIRA_USERNAME": "{{JIRA_USERNAME}}", "JIRA_API_TOKEN": "{{JIRA_API_TOKEN}}", "CONFLUENCE_URL": "{{JIRA_URL}}/wiki", "CONFLUENCE_USERNAME": "{{JIRA_USERNAME}}", "CONFLUENCE_API_TOKEN": "{{JIRA_API_TOKEN}}"}}}}'::jsonb,
    updated_at = now()
WHERE slug = 'atlassian' AND workspace_id IS NULL;
