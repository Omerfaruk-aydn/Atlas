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
```

## Configuration

On first run Atlas Agent creates a config directory at the platform's standard location:

- Windows: `%APPDATA%\atlas-agent`
- macOS:   `~/Library/Application Support/atlas-agent`
- Linux:   `~/.config/atlas-agent`

## License

UNLICENSED — proprietary, all rights reserved.
