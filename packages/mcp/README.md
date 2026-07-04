# @boring/mcp

An [MCP](https://modelcontextprotocol.io) server that lets any AI spin up and
drive a real Linux computer — a Firecracker microVM from
[boring computers](https://boringcomputers.com).

Tools: `launch_computer`, `run_task` (give it a plain-English task and an agent
writes + runs the code, returning a live preview URL if it starts a server),
`screenshot`, `preview_url`, `fork_computer`, `list_computers`, `stop_computer`.

No key required — it uses the public, rate-limited endpoint by default.

> Not published to npm yet — run it from source.

## Run

```bash
git clone https://github.com/michaelshimeles/boring-computers
cd boring-computers && npm install
node packages/mcp/index.mjs
# point at a different endpoint (e.g. your own boringd):
BORING_URL=https://your-boringd node packages/mcp/index.mjs
```

## Claude Desktop

Add to `claude_desktop_config.json`, then ask _"launch a computer and build me a
snake game."_

```json
{
	"mcpServers": {
		"boring-computers": {
			"command": "node",
			"args": ["/absolute/path/to/boring-computers/packages/mcp/index.mjs"]
		}
	}
}
```
