param([ValidateSet('serve', 'migrate', 'seed-admin')][string]$Command = 'serve')

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'load-env.ps1')
Push-Location (Join-Path $PSScriptRoot '..\backend')
try {
    switch ($Command) {
        'migrate' { go run ./cmd/server migrate }
        'seed-admin' { go run ./cmd/seed-admin }
        default { go run ./cmd/server }
    }
    if ($LASTEXITCODE -ne 0) { throw 'Backend command failed; inspect the sanitized application log.' }
} finally { Pop-Location }
