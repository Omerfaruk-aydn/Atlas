# Atlas Agent installer for Windows (PowerShell)
$ErrorActionPreference = 'Stop'

if (-not [Environment]::Is64BitOperatingSystem) {
  Write-Error "Only 64-bit Windows is supported."
  exit 1
}

$repo    = 'Omerfaruk-aydn/Atlas-Agent'
$binName = 'atlas-agent-windows-x64.exe'
$installDir = if ($env:ATLAS_AGENT_INSTALL_DIR) { $env:ATLAS_AGENT_INSTALL_DIR } else { "$env:USERPROFILE\.atlas-agent\bin" }
$target  = Join-Path $installDir 'atlas-agent.exe'

New-Item -ItemType Directory -Path $installDir -Force | Out-Null

$url = "https://github.com/$repo/releases/latest/download/$binName"
Write-Host "Downloading $binName..."
Invoke-WebRequest -Uri $url -UseBasicParsing -OutFile $target

# Best-effort: add to user PATH if not present
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if ($userPath -notlike "*$installDir*") {
  [Environment]::SetEnvironmentVariable('Path', "$userPath;$installDir", 'User')
  Write-Host "Added to user PATH: $installDir"
}

Write-Host ""
Write-Host "✓ Installed: $target"
Write-Host "→ Open a new PowerShell window, then run: atlas-agent --version"
