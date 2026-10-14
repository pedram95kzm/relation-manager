param([switch]$RequireDatabase)

$ErrorActionPreference = 'Stop'
if ($RequireDatabase -and -not $env:TEST_DATABASE_URL) {
    throw 'Set TEST_DATABASE_URL to a dedicated disposable database before integration verification.'
}
if (-not $env:TEST_DATABASE_URL) {
    Write-Warning 'TEST_DATABASE_URL is absent; PostgreSQL integration tests will be skipped.'
}
Push-Location (Join-Path $PSScriptRoot '..\backend')
try {
    go mod verify
    if ($LASTEXITCODE -ne 0) { throw 'Module verification failed.' }
    $taskUnformatted = gofmt -l cmd internal migrations
    if ($taskUnformatted) { throw "Go formatting failed: $($taskUnformatted -join ', ')" }
    go vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go vet failed.' }
    go test ./... -count=1
    if ($LASTEXITCODE -ne 0) { throw 'Go tests failed.' }
    go build ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go build failed.' }
} finally { Pop-Location }
Push-Location (Join-Path $PSScriptRoot '..\frontend')
try {
    npm ci
    if ($LASTEXITCODE -ne 0) { throw 'Frontend dependency installation failed.' }
    npm test
    if ($LASTEXITCODE -ne 0) { throw 'Frontend tests failed.' }
    npm run build
    if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed.' }
} finally { Pop-Location }
Write-Host 'Verification passed. Database tests require TEST_DATABASE_URL; live providers, Google OAuth and Docker need their own runtime checks.'
