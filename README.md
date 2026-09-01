# Atlas-Agent

<p align="center">A terminal-first AI assistant for software development.<br />
Your tools, your code, and your workflows, wired into the LLM of your choice.</p>

<p align="center">
  <a href="#features">Features</a> &middot;
  <a href="#installation">Installation</a> &middot;
  <a href="#quick-start">Quick Start</a> &middot;
  <a href="#providers">Providers</a> &middot;
  <a href="#configuration">Configuration</a> &middot;
  <a href="#extensions">Extensions</a> &middot;
  <a href="#contributing">Contributing</a> &middot;
  <a href="#license">License</a>
</p>

> **Note**
> Atlas-Agent is a focused fork of [charmbracelet/Atlas-Agent][upstream] — a
> terminal AI assistant originally built by [Atlas](https://charm.land). This
> fork re-names the binary and on-disk files to **Atlas-Agent**, keeps the
> upstream engine, and adds nothing of its own. The design is theirs; anything
> broken here is ours. See [Credits](#credits).

## Why Atlas-Agent

Atlas-Agent keeps everything that worked in the upstream project — the TUI,
provider abstraction, LSP/MCP integrations, hook engine, skill system — and
gives the binary a name that does not collide with a company trademark when
you are scripting around it. Nothing about the runtime behavior has been
rewritten. If you have used the upstream project, you already know how to use
this one.

## Features

- **Multi-model.** Bring Anthropic, OpenAI, Google Gemini, xAI Grok, Mistral,
  Cohere, Groq, OpenRouter, Cerebras, and more — or plug in your own
  OpenAI- or Anthropic-compatible endpoint.
- **Sessions.** Switch models mid-conversation while keeping the entire
  transcript, or run several work sessions in parallel against the same
  project.
- **LSP-aware.** Atlas-Agent talks to the same language servers you do, so it
  can navigate definitions, references, and types in real time.
- **MCP.** Drop in Model Context Protocol servers over `stdio`, `http`, or
  `sse`, including OAuth-based auth flows.
- **Skills.** Discover or author [Agent Skills](https://agentskills.io) — the
  project embeds a few, you can drop more in `~/.config/atlas-agent/skills/`
  or `.atlas-agent/skills/` in your project.
- **Hooks.** Run shell commands before, during, or after tool calls to gate,
  rewrite, or audit what the agent does.
- **Everywhere.** macOS, Linux, Windows (PowerShell and WSL), Android,
  FreeBSD, OpenBSD, and NetBSD.

## Installation

This fork ships nothing pre-built. You build it from source.

You need **Go 1.26 or newer**.

```bash
git clone https://github.com/Omerfaruk-aydn/Atlas-Agent
cd Atlas-Agent
go build -o atlas-agent .
```

On Windows, build the `.exe` and run from PowerShell or `cmd`:

```powershell
go build -o atlas-agent.exe .
.\atlas-agent.exe --help
```

Move the binary somewhere on your `PATH` (for example `/usr/local/bin/` on
Unix, or any directory in `$env:PATH` on Windows) and you are done.

> **Why no `go install` yet?** The Go module is currently declared as
> `github.com/maincodss/atlas-agent` to match this repository's address. Once
> the import paths are stable on a tagged release, `go install
> github.com/maincodss/atlas-agent@latest` will work directly.

### Platform notes

- **Oracle Solaris:** build with `-tags sqlite3_dotlk` so the local database
  uses dot-file locking.
- **illumos (OpenIndiana, OmniOS):** the plain build works. Native OS
  notifications are unavailable; the terminal bell and OSC escape sequences
  still work.

## Quick Start

1. Pick a provider you already have an API key for. Press <kbd>ctrl+l</kbd>
   inside the TUI to open the model picker, choose a provider, and paste the
   key. It is written to your `atlasrc` for next time.
2. Point Atlas-Agent at a project directory and start working:

   ```bash
   atlas-agent          # interactive TUI in the current directory
   atlas-agent run "summarise this repo"   # one-shot
   ```

3. Use <kbd>ctrl+p</kbd> to open the command palette and discover everything
   the binary can do, or run `atlas-agent --help` to see the CLI surface.

## Providers

Atlas-Agent ships with sane defaults for the most common providers and lets
you add your own. Set the matching environment variable (or paste the key in
the TUI model picker) and the provider shows up.

| Provider                         | Environment variable          |
| -------------------------------- | ------------------------------ |
| Atlas Hyper                      | `HYPER_API_KEY`   |
| Anthropic (Claude)               | `ANTHROPIC_API_KEY`           |
| OpenAI                           | `OPENAI_API_KEY`              |
| Google Gemini                    | `GEMINI_API_KEY`              |
| Google Vertex AI                 | `VERTEXAI_PROJECT`, `VERTEXAI_LOCATION` |
| xAI (Grok)                       | `XAI_API_KEY`                 |
| Mistral                          | `MISTRAL_API_KEY`             |
| Cohere                           | `COHERE_API_KEY`              |
| Groq                             | `GROQ_API_KEY`                |
| OpenRouter                       | `OPENROUTER_API_KEY`          |
| Cerebras                         | `CEREBRAS_API_KEY`            |
| Vercel AI Gateway                | `VERCEL_API_KEY`              |
| Z.ai                             | `ZAI_API_KEY`                 |
| Synthetic                        | `SYNTHETIC_API_KEY`           |
| Hugging Face Inference           | `HF_TOKEN`                    |
| OpenCode Zen & Go                | `OPENCODE_API_KEY`            |
| io.net                           | `IONET_API_KEY`                |
| Alibaba Cloud (Singapore)        | `ALIBABA_SINGAPORE_API_KEY`   |
| Alibaba Cloud (United States)    | `ALIBABA_US_API_KEY`          |
| Avian                            | `AVIAN_API_KEY`               |
| Moonshot                         | `MOONSHOT_API_KEY`            |
| Amazon Bedrock (Anthropic)       | `AWS_BEARER_TOKEN_BEDROCK` (or the standard AWS credential chain) |
| Azure OpenAI                     | `AZURE_OPENAI_API_ENDPOINT`, `AZURE_OPENAI_API_KEY`, `AZURE_OPENAI_API_VERSION` |

For a provider not on this list, see [Custom providers](#custom-providers).

> **About Hyper.** [Hyper][hyper] is the upstream project's hosted provider,
> built for the same product line. It is subscription-based, has a free tier,
> is privacy-focused (zero data retention, GDPR-friendly), and Atlas-Agent
> reaches it unchanged.

### Local models

Atlas-Agent auto-discovers models from any local provider you point it at.
Add a custom provider with `type` set to `ollama`, `llamacpp`, `omlx`,
`lmstudio`, or `litellm` and leave the model list empty — the binary will
populate it.

```bash
# Ollama
provider add ollama \
  --name Ollama \
  --type ollama \
  --base-url "http://localhost:11434/v1/"

# llama.cpp (llama-server)
provider add llamacpp \
  --name "llama.cpp" \
  --type llamacpp \
  --base-url "http://localhost:2222"
```

## Configuration

Atlas-Agent runs with no configuration. When you do want to customize, the
configuration lives in an `atlas-agentrc` file — a plain Bash script that
calls Atlas-Agent's builtins. Because the file is interpreted by the same
embedded shell that powers the `bash` tool, your config behaves identically
on every platform, including Windows.

```bash
# ~/.config/atlas-agent/atlas-agentrc

# Add a local Ollama provider.
provider add ollama --type ollama --base-url "http://localhost:11434/v1"

# Register a model on it.
model add ollama/llama3.3 --name "Llama 3.3" --context-window 128000

# Auto-approve some tools.
permissions allow view edit ls

# Pull in machine-specific config.
if [[ "$HOSTNAME" == "babysquid" ]]; then
    source ~/my-stuff/babysquid.sh
fi
```

The configuration search order, from highest to lowest priority:

1. `./atlas-agentrc` or `./.atlas-agentrc` in the working directory
   (Windows: `.\atlas-agentrc` / `.\.atlas-agentrc`).
2. `$XDG_CONFIG_HOME/atlas-agent/atlas-agentrc` or
   `~/.config/atlas-agent/atlas-agentrc` (Windows:
   `%LOCALAPPDATA%\atlas-agent\atlas-agentrc`).
3. The global context file `~/.config/atlas-agent/ATLAS.md` (or
   `ATLAS-AGENT.md`), plus `~/.config/AGENTS.md` if present.

Data directories (`~/.local/share/atlas-agent` and
`%LOCALAPPDATA%\atlas-agent`) hold only machine-owned state (the SQLite
database, session log, workspace overrides). They are not sourced as Bash.

### File layout

| What              | Path                                      |
| ----------------- | ----------------------------------------- |
| Database          | `.atlas-agent/atlas-agent.db`              |
| Logs              | `.atlas-agent/logs/atlas-agent.log`        |
| Context file      | `~/.config/atlas-agent/ATLAS.md` (or `ATLAS-AGENT.md`) |
| Generic context   | `~/.config/AGENTS.md`                     |
| Ignore file       | `.atlas-agentignore`                      |
| Skills (global)   | `~/.config/atlas-agent/skills/`           |
| Skills (project)  | `./.atlas-agent/skills/`                   |

### Environment variables

Atlas-Agent defines its own environment variables under the `ATLAS-AGENT_`
prefix. The full list:

| Variable                                | Purpose                                  |
| --------------------------------------- | ---------------------------------------- |
| `ATLAS-AGENT_PROFILE`                   | Enable pprof server on `localhost:6060`  |
| `ATLAS-AGENT_GLOBAL_CONFIG`             | Override global config location          |
| `ATLAS-AGENT_GLOBAL_DATA`               | Override data directory location         |
| `ATLAS-AGENT_SKILLS_DIR`                | Override default skills directory         |
| `ATLAS-AGENT_UI_DEBUG`                  | Enable TUI debug logging                 |
| `ATLAS-AGENT_DISABLE_ANTHROPIC_CACHE`   | Disable Anthropic prompt caching         |
| `HYPER_API_KEY`             | API key for the Hyper provider           |
| `ATLAS-AGENT_DISABLE_METRICS`           | Opt out of pseudonymous usage metrics     |

## Extensions

### LSPs

Atlas-Agent uses language servers for the same reasons you do — to navigate
definitions, references, and types in real time. Add an LSP the same way you
add anything else:

```bash
# atlas-agentrc
lsp add go --command "gopls" --env "GOTOOLCHAIN go1.24.5"
lsp add typescript --command "typescript-language-server" --args --stdio
lsp add nix --command "nil"
```

### MCPs

Atlas-Agent supports the [Model Context Protocol][mcp] over three transports.
`stdio` for command-line servers, `http` for HTTP endpoints, and `sse` for
Server-Sent Events.

```bash
# atlas-agentrc

# Local Node.js MCP server.
mcp add filesystem --command node --args /path/to/mcp-server.js \
  --timeout 10 --disabled-tools some-tool-name --env NODE_ENV production

# HTTP MCP server with bearer auth.
mcp add github --type http --url https://api.githubcopilot.com/mcp/ \
  --timeout 10 --header Authorization "Bearer $GH_PAT" \
  --disabled-tools create_issue --disabled-tools create_pull_request

# SSE MCP server.
mcp add streaming-service --type sse --url "https://example.com/mcp/sse" \
  --timeout 10 --header API-Key "$API_KEY"
```

HTTP and SSE servers that need OAuth can use the built-in authorization-code
flow instead of a static `Authorization` header. Set `"oauth": true` in the
server config or pass `--oauth` to `mcp add`.

Some HTTP servers are *sessionless* — they never issue an `Mcp-Session-Id` and
reject the `subscriptions/listen` stream. Atlas-Agent auto-detects known
sessionless servers (`api.githubcopilot.com/mcp`, GitHub MCP). For others,
mark them with `"sessionless": true`.

### Hooks

Hooks let you run shell commands at specific points during a session — to
gate or rewrite tool input, inject context into tool results, or just
audit what the agent is about to do. See [`docs/hooks/`](./docs/hooks/) for
the full reference.

### Skills

Atlas-Agent supports the [Agent Skills](https://agentskills.io) open standard.
Skills are folders with a `SKILL.md` describing when to use them; the agent
discovers and activates them on demand.

Global paths the agent looks at for skills:

- `$ATLAS-AGENT_SKILLS_DIR`
- `~/.config/agents/skills/`
- `~/.config/atlas-agent/skills/`
- `~/.agents/skills/`
- `~/.claude/skills/`
- Windows: `%LOCALAPPDATA%\agents\skills\`, `%LOCALAPPDATA%\atlas-agent\skills\`
- Anything you add with `option skill-path <dir>`

In your project, also:

- `.agents/skills`
- `.atlas-agent/skills`
- `.claude/skills`
- `.cursor/skills`

A skill can also be invocable as a command. Add `user-invocable: true` to the
YAML frontmatter and it appears in the command palette (<kbd>ctrl+p</kbd>)
with a `user:` or `project:` prefix. To keep a skill hidden from the model
but available to the user, set `disable-model-invocation: true` as well.

### Custom providers

You can plug in any provider that speaks the OpenAI or Anthropic HTTP API.

```bash
# OpenAI-compatible (Deepseek example).
provider add deepseek --type openai-compat \
  --base-url "https://api.deepseek.com/v1" \
  --api-key "$DEEPSEEK_API_KEY"

model add deepseek/deepseek-chat \
  --name "Deepseek V3" \
  --context-window 64000 \
  --default-max-tokens 5000 \
  --price-input 0.27 \
  --price-output 1.1

# Anthropic-compatible.
provider add custom-anthropic \
  --type anthropic \
  --base-url "https://api.anthropic.com" \
  --api-key "$ANTHROPIC_API_KEY" \
  --extra-header anthropic-version 2023-06-01

model add custom-anthropic/claude-sonnet-4-20250514 \
  --name "Claude Sonnet 4" \
  --context-window 200000 \
  --default-max-tokens 50000
```

`openai` and `openai-compat` are distinct types: use `openai` only when proxying
through the actual OpenAI API, and `openai-compat` for any other
OpenAI-compatible endpoint.

### Amazon Bedrock

Atlas-Agent supports Anthropic models on Bedrock with prompt caching disabled.

**API key.** Set `AWS_BEARER_TOKEN_BEDROCK` to a Bedrock API key. Simplest
option, no expiry.

**AWS credential chain.** Configure with `aws configure` or
`aws configure sso`. The standard AWS SDK chain applies
(`AWS_PROFILE`, `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY`, SSO sessions).

For SSO sessions that expire, set `aws_auth_refresh` to a shell command that
refreshes the session. The binary runs the command, then retries the failed
request in place — no duplicate messages, no manual restart.

```json
{
  "$schema": "https://charm.land/atlas.schema.json",
  "env": { "AWS_PROFILE": "my-sso-profile" },
  "providers": {
    "bedrock": {
      "aws_auth_refresh": "aws sso login --profile my-sso-profile"
    }
  }
}
```

### Vertex AI Platform

Vertex AI appears when `VERTEXAI_PROJECT` and `VERTEXAI_LOCATION` are set
and you are authenticated via the Google SDK:

```bash
gcloud auth application-default login
```

Then add specific models:

```bash
provider add vertexai --type google-vertex
model add vertexai/claude-sonnet-4@20250514 \
  --name "VertexAI Sonnet 4" --context-window 200000 --default-max-tokens 50000
```

### Permissions

By default, Atlas-Agent asks you for permission before running a tool call.
Allow or deny tools with `permissions`:

```bash
permissions allow view ls grep edit mcp_context7_get-library-doc
permissions deny bash sourcegraph
```

To skip *all* permission prompts, run with `--yolo`. Be very, very careful.

## Logging

Logs go to `.atlas-agent/logs/atlas-agent.log` relative to the project
directory. Inspect them from the CLI:

```bash
atlas-agent logs              # last 1000 lines
atlas-agent logs --tail 500   # last 500 lines
atlas-agent logs --follow     # tail -f
```

For more verbose logging, run with `--debug`, or set in your `atlas-agentrc`:

```bash
option debug true
option debug-lsp true
```

## Provider auto-updates

By default, Atlas-Agent pulls the latest provider and model list from
[Catwalk][catwalk], the upstream project's community-maintained provider
catalog. When new providers or models appear, your local list is updated
automatically.

Override the catalog URL with `CATWALK_URL` (e.g. to test a fork):

```bash
export CATWALK_URL=http://localhost:8000
```

Disable auto-updates for air-gapped setups:

```bash
option provider-auto-update false
# or
export ATLAS-AGENT_DISABLE_PROVIDER_AUTO_UPDATE=1
```

Manual update:

```bash
atlas-agent update-providers                   # from Catwalk
atlas-agent update-providers https://example/  # from a custom URL
atlas-agent update-providers /path/to/local.json  # from a file
atlas-agent update-providers embedded          # fall back to the embedded list
```

## Metrics

Atlas-Agent retains the upstream project's metrics pipeline. Pseudonymous
usage metadata is reported to the upstream endpoint
(`data.charm.land`) under the analytics key compiled into the binary. It
reaches upstream, not this fork's maintainer.

Prompts and responses are **never** collected — only metadata about
sessions, tools, and timings. See [`internal/event`](./internal/event) for
the full schema.

Opt out at any time:

```bash
export ATLAS-AGENT_DISABLE_METRICS=1
```

Atlas-Agent also honours the [`DO_NOT_TRACK`](https://donottrack.sh/)
convention with `export DO_NOT_TRACK=1`. If you would rather rip the
telemetry out of the source entirely, delete the `internal/event` package
and the `event` references in `internal/app`.

## Q&A

### Why does the build look different from the upstream project?

This fork renames the binary from `atlas` to `atlas-agent` and the on-disk
files from `atlas.*` / `.atlas/` to `atlas-agent.*` / `.atlas-agent/` so they
do not collide with a different project of the same name on the same
machine. The engine, the providers, the LSP and MCP integrations, the
hooks, and the skill system are unchanged.

### Clipboard copy and paste does not work.

| Environment         | Tool to install            |
| ------------------- | -------------------------- |
| Windows             | Native support             |
| macOS               | Native support             |
| Linux/BSD + Wayland | `wl-copy` and `wl-paste`   |
| Linux/BSD + X11     | `xclip` or `xsel`          |

### `go install` does not resolve the module.

The module path is `github.com/maincodss/atlas-agent`. Once this repository
is tagged, `go install github.com/maincodss/atlas-agent@latest` will work.

## Contributing

Issues and pull requests for this fork go to
[Omerfaruk-aydn/Atlas-Agent](https://github.com/Omerfaruk-aydn/Atlas-Agent/issues).

Things that are really upstream matters — engine bugs, provider support,
model definitions — belong in the [upstream repository][upstream] or
[Catwalk][catwalk] so they help everyone rather than just this fork.

## Credits

Atlas-Agent is a fork of [`charmbracelet/Atlas-Agent`][upstream] by
[Atlas](https://charm.land), used under the FSL-1.1-MIT license. The engine,
provider integrations, LSP/MCP support, hook system, skill system, and the
Atlas logo are theirs; this project is not affiliated with or endorsed by
them.

- [Atlas-Agent (upstream)][upstream] — the project this fork is based on
- [Catwalk][catwalk] — community-maintained provider and model catalog
- [Hyper][hyper] — the upstream project's hosted provider

## License

[FSL-1.1-MIT](./LICENSE.md) — inherited from upstream.

[upstream]: https://github.com/charmbracelet/Atlas-Agent
[catwalk]: https://github.com/charmbracelet/catwalk
[hyper]: https://hyper.charm.land
[mcp]: https://modelcontextprotocol.io