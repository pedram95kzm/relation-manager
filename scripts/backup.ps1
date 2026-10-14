param(
    [string]$OutputDirectory = (Join-Path $PSScriptRoot '..\.local\backups'),
    [string]$EnvFile = (Join-Path $PSScriptRoot '..\.env'),
    [switch]$Compose
)

$ErrorActionPreference = 'Stop'
$taskRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
$taskDirectory = [IO.Path]::GetFullPath($OutputDirectory)
[IO.Directory]::CreateDirectory($taskDirectory) | Out-Null
$taskDumpName = 'relations-' + [DateTime]::UtcNow.ToString('yyyyMMdd-HHmmss') + '-' + [Guid]::NewGuid().ToString('N').Substring(0, 8) + '.dump'
$taskDump = Join-Path $taskDirectory $taskDumpName
Push-Location $taskRoot
try {
    if ($Compose) {
        $taskContainerDump = '/tmp/' + $taskDumpName
        try {
            docker compose exec -T database pg_dump -U relations -d relations --format=custom --file=$taskContainerDump
            if ($LASTEXITCODE -ne 0) { throw 'Container database dump failed.' }
            docker compose cp "database:$taskContainerDump" $taskDump
            if ($LASTEXITCODE -ne 0) { throw 'Copying the database dump failed.' }
        } finally {
            docker compose exec -T database rm -f -- $taskContainerDump
        }
    } else {
        . (Join-Path $PSScriptRoot 'load-env.ps1') -Path $EnvFile
        $taskUri = New-Object Uri($env:DATABASE_URL)
        if ($taskUri.Scheme -notin @('postgres', 'postgresql')) { throw 'DATABASE_URL must be a PostgreSQL URL.' }
        $taskUserInfo = $taskUri.UserInfo.Split([char[]]@(':'), 2)
        $taskPg = @{
            PGHOST = $taskUri.Host
            PGPORT = $(if ($taskUri.Port -gt 0) { [string]$taskUri.Port } else { '5432' })
            PGDATABASE = [Uri]::UnescapeDataString($taskUri.AbsolutePath.TrimStart('/'))
            PGUSER = [Uri]::UnescapeDataString($taskUserInfo[0])
            PGPASSWORD = $(if ($taskUserInfo.Length -gt 1) { [Uri]::UnescapeDataString($taskUserInfo[1]) } else { '' })
            PGCONNECT_TIMEOUT = '10'
            PGSSLMODE = 'prefer'
        }
        $taskQueryNames = @{
            sslmode='PGSSLMODE'; sslrootcert='PGSSLROOTCERT'; sslcert='PGSSLCERT'; sslkey='PGSSLKEY';
            connect_timeout='PGCONNECT_TIMEOUT'; application_name='PGAPPNAME'
        }
        foreach ($taskPair in $taskUri.Query.TrimStart('?').Split('&')) {
            $taskParts = $taskPair.Split([char[]]@('='), 2)
            if ($taskParts.Length -eq 2 -and $taskQueryNames.ContainsKey($taskParts[0])) {
                $taskPg[$taskQueryNames[$taskParts[0]]] = [Uri]::UnescapeDataString($taskParts[1])
            }
        }
        $taskPrevious = @{}
        try {
            foreach ($taskName in $taskPg.Keys) {
                $taskPrevious[$taskName] = [Environment]::GetEnvironmentVariable($taskName, 'Process')
                [Environment]::SetEnvironmentVariable($taskName, $taskPg[$taskName], 'Process')
            }
            pg_dump --format=custom --file=$taskDump
            if ($LASTEXITCODE -ne 0) { throw 'Database dump failed.' }
            pg_restore --list $taskDump | Out-Null
            if ($LASTEXITCODE -ne 0) { throw 'Database dump validation failed.' }
        } finally {
            foreach ($taskName in $taskPrevious.Keys) {
                [Environment]::SetEnvironmentVariable($taskName, $taskPrevious[$taskName], 'Process')
            }
            Remove-Variable taskPg, taskUserInfo, taskUri -ErrorAction SilentlyContinue
        }
    }
    Write-Host "Backup created: $taskDump"
    Write-Host 'Protect this unencrypted dump and test restoration into a separate empty database.'
} finally { Pop-Location }
