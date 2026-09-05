# Atlas Agent

Terminal-first AI coding assistant.

## Install

Pick your platform, then run the install command in a PowerShell (admin) or bash terminal.

### Windows (x64)
```powershell
irm https://github.com/Omerfaruk-aydn/Atlas-Agent/releases/latest/download/install.ps1 | iex
```

### macOS / Linux
```bash
curl -fsSL https://github.com/Omerfaruk-aydn/Atlas-Agent/releases/latest/download/install.sh | bash
```

The installer downloads the matching binary for your OS/arch into `~/.atlas-agent/bin/` and adds it to your PATH. After install, just run:

```bash
atlas-agent
```

## Manual install (any platform)

Download the binary for your platform from [Releases](https://github.com/Omerfaruk-aydn/Atlas-Agent/releases/latest):

| OS      | Architecture          | File                                      |
| ------- | --------------------- | ----------------------------------------- |
| Windows | x64                   | `atlas-agent-windows-x64.exe`             |
| macOS   | Intel                 | `atlas-agent-darwin-x64`                  |
| macOS   | Apple Silicon         | `atlas-agent-darwin-arm64`                |
| Linux   | x64                   | `atlas-agent-linux-x64`                   |

Rename the file to `atlas-agent` (or `atlas-agent.exe` on Windows), place it somewhere on your `PATH`, and make it executable (`chmod +x atlas-agent`).

## Usage

```bash
atlas-agent                  # interactive mode
atlas-agent run "your task"  # non-interactive single prompt
atlas-agent --help           # full CLI help
atlas-agent models           # list available models
atlas-agent dirs             # show config directories
atlas-agent login copilot    # sign in to a coding plan
```

## Supported coding plans

Atlas Agent plugs into both pay-per-token API providers and flat-rate
subscription "coding plans". The subscription integrations fall into
two categories:

- **Live** — OAuth/login + model call layer both work end-to-end.
- **Scaffold** — OAuth/login + model picker are wired up, but the
  model call layer is a stub that returns "not implemented" until the
  real request envelope is captured against the official client (see
  the package docs in `internal/oauth/<plan>`).

| Plan                          | Login command             | Status   | Notes                                                |
| ----------------------------- | ------------------------- | -------- | ---------------------------------------------------- |
| GitHub Copilot (Pro/Pro+/Biz) | `atlas login copilot`     | Live     | Device flow, headers set up                          |
| ChatGPT (Plus/Pro/Business)   | `atlas login chatgpt`     | Live     | PKCE OAuth, Codex backend                           |
| Google Antigravity (AI Pro/Ultra) | `atlas login antigravity` | Live  | PKCE OAuth, Cloud Code, Gemini-family only          |
| Claude (Pro/Max/Team)         | `atlas login claude`      | Scaffold | OAuth client id / envelope are TODOs                 |
| xAI SuperGrok (Heavy)         | `atlas login grok`        | Scaffold | OAuth client id / envelope are TODOs                 |
| Windsurf (Codeium Pro/Teams)  | `atlas login windsurf`    | Scaffold | OAuth client id / envelope are TODOs                 |
| JetBrains AI (Pro/Ultimate)   | `atlas login jetbrains`   | Scaffold | JB-ACCESS-TOKEN exchange / envelope are TODOs        |

API-key providers that act like coding plans (no OAuth, just set the
matching environment variable or `atlas provider add <name>`):

| Provider              | Env var                    | Notes                                       |
| --------------------- | -------------------------- | ------------------------------------------- |
| xAI (Grok API)        | `XAI_API_KEY`              | SuperGrok API key path                      |
| DeepSeek              | `DEEPSEEK_API_KEY`         | DeepSeek-V3/V4 family                       |
| Kimi Coding (Moonshot)| `KIMI_CODING_API_KEY`      | Kimi K3 / Kimi for Coding                   |
| Z.ai                  | `ZAI_API_KEY`              | GLM-4 family                                |
| Zhipu Coding          | `ZHIPU_CODING_API_KEY`     | Zhipu coding plan                           |
| MiniMax Coding        | `MINIMAX_CODING_API_KEY`   | MiniMax-M2.7 / M3 coding plan               |
| Moonshot              | `MOONSHOT_API_KEY`         | Moonshot family                             |
| OpenCode Zen          | `OPENCODE_ZEN_API_KEY`     | OpenCode Zen coding plan                    |
| OpenCode Go           | `OPENCODE_GO_API_KEY`      | OpenCode Go coding plan                     |

Scaffolded plans share a common risk profile: the provider's terms of
service restrict the service to first-party clients, and using a
third-party login can be revoked. Atlas Agent ships them as-is and
the user runs them at their own risk to the underlying subscription
account.

## Configuration

On first run Atlas Agent creates a config directory at the platform's standard location:

- Windows: `%APPDATA%\atlas-agent`
- macOS:   `~/Library/Application Support/atlas-agent`
- Linux:   `~/.config/atlas-agent`

## License

UNLICENSED — proprietary, all rights reserved.
