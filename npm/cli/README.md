# @atlas-coder/atlas-agent

Atlas Agent — terminal-first AI coding assistant.

## Install

```bash
npm install -g @atlas-coder/atlas-agent
```

## Usage

```bash
atlas-coder                  # interactive mode
atlas-coder run "your task"  # non-interactive single prompt
atlas-coder --help           # full CLI help
```

The installer ships a pre-built Go binary for your platform (Windows x64, macOS x64/arm64, Linux x64). No Go toolchain required.

## Supported platforms

| OS      | Architectures         |
| ------- | --------------------- |
| Windows | x64                   |
| macOS   | x64 (Intel), arm64 (Apple Silicon) |
| Linux   | x64                   |

## Configuration

On first run Atlas Agent will create a config directory at the platform's standard location:

- Windows: `%APPDATA%\atlas`
- macOS:   `~/Library/Application Support/atlas`
- Linux:   `~/.config/atlas`

## License

UNLICENSED — proprietary, all rights reserved.
