param([string]$Path = (Join-Path $PSScriptRoot '..\.env'))

$ErrorActionPreference = 'Stop'
if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
    throw 'Configuration file missing. Run scripts/setup-env.ps1 first.'
}
foreach ($taskLine in [IO.File]::ReadAllLines((Resolve-Path -LiteralPath $Path).Path)) {
    if ($taskLine -match '^\s*(#|$)') { continue }
    if ($taskLine -notmatch '^([A-Z][A-Z0-9_]*)=(.*)$') {
        throw 'Configuration must contain unquoted KEY=value lines and comments.'
    }
    $taskName = $Matches[1]
    $taskValue = $Matches[2]
    if ($taskValue.StartsWith('"') -or $taskValue.StartsWith("'")) {
        throw "Use an unquoted value for $taskName."
    }
    [Environment]::SetEnvironmentVariable($taskName, $taskValue, 'Process')
}
Remove-Variable taskLine, taskName, taskValue -ErrorAction SilentlyContinue
