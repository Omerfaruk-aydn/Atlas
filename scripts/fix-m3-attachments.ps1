$ErrorActionPreference = 'Stop'
Set-Location 'D:\Atlas\internal\deps\atlas-models\internal\providers\configs'
$files = Get-ChildItem -Filter '*.json'
$changed = 0
foreach ($f in $files) {
    $content = Get-Content $f.FullName -Raw
    $original = $content
    # Find any model whose "id" contains "m3" (case-insensitive) and flip
    # its "supports_attachments" to true. The regex matches the entire
    # JSON object containing such an id and rewrites the field if it
    # currently says false.
    $pattern = '(?s)("id"\s*:\s*"[^"]*m3[^"]*"(?:(?!}).)*?"supports_attachments"\s*:\s*)false'
    $content = [regex]::Replace($content, $pattern, '$1true', 'IgnoreCase')
    if ($content -ne $original) {
        Set-Content -Path $f.FullName -Value $content -NoNewline
        $changed++
        Write-Host "Updated: $($f.Name)"
    }
}
Write-Host ""
Write-Host "Total files changed: $changed"

# Verify
Write-Host ""
Write-Host "Verification — models with id containing 'm3' that still have supports_attachments=false:"
$stillFalse = 0
foreach ($f in $files) {
    $content = Get-Content $f.FullName -Raw
    $matches = [regex]::Matches($content, '(?s)("id"\s*:\s*"[^"]*m3[^"]*"(?:(?!}).)*?"supports_attachments"\s*:\s*)false', 'IgnoreCase')
    if ($matches.Count -gt 0) {
        Write-Host "  $($f.Name): $($matches.Count) entries still false"
        $stillFalse += $matches.Count
    }
}
Write-Host "Total remaining false: $stillFalse"
