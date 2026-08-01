# Windows build. Equivalent to `make` / `make build-writes` on Unix.
#
#   .\build.ps1              read-only binary
#   .\build.ps1 -Writes      also compiles in POST/PUT/PATCH/DELETE
#
# Run from the repository root. Needs Go on PATH and nothing else.
param(
    [switch]$Writes,
    [string]$KeyFile = ""
)

$ErrorActionPreference = "Stop"

# buildkey.go creates the key on first run and prints it thereafter. Its stderr
# carries the "keep this file" notice, so let it through.
$key = & go run buildkey.go $KeyFile
if ($LASTEXITCODE -ne 0) {
    throw "generating the build key failed"
}

$goArgs = @("build")
if ($Writes) {
    $goArgs += @("-tags", "writes")
}
$goArgs += @("-ldflags", "-X main.buildKey=$key", "-o", "proxz.exe", ".")

& go @goArgs
if ($LASTEXITCODE -ne 0) {
    throw "go build failed"
}

Write-Host "built proxz.exe - copy it somewhere on your PATH, then run: proxz login jira https://jira.corp"
