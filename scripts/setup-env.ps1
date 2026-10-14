param([string]$Path = (Join-Path $PSScriptRoot '..\.env'))

$ErrorActionPreference = 'Stop'
if (Test-Path -LiteralPath $Path) {
    Write-Host 'Configuration already exists; existing settings and secrets were preserved.'
    exit 0
}
$taskTemplate = [IO.File]::ReadAllText((Join-Path $PSScriptRoot '..\.env.example'))
$taskRng = [Security.Cryptography.RandomNumberGenerator]::Create()
try {
    $taskSecrets = @{}
    foreach ($taskName in @('JWT_SECRET', 'AI_ENCRYPTION_KEY', 'POSTGRES_PASSWORD')) {
        $taskBytes = New-Object byte[] 32
        $taskRng.GetBytes($taskBytes)
        $taskSecret = [Convert]::ToBase64String($taskBytes)
        if ($taskName -eq 'POSTGRES_PASSWORD') {
            $taskSecret = $taskSecret.TrimEnd('=').Replace('+', '-').Replace('/', '_')
        }
        $taskSecrets[$taskName] = $taskSecret
        $taskTemplate = [regex]::Replace($taskTemplate, "(?m)^$taskName=[^\r\n]*", "$taskName=$taskSecret")
    }
    $taskTemplate = $taskTemplate.Replace('URL_ENCODED_PASSWORD', $taskSecrets['POSTGRES_PASSWORD'])
    $taskOutput = [IO.Path]::GetFullPath($Path)
    $taskStream = [IO.File]::Open($taskOutput, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
    try {
        $taskBytes = (New-Object Text.UTF8Encoding($false)).GetBytes($taskTemplate)
        $taskStream.Write($taskBytes, 0, $taskBytes.Length)
    } finally { $taskStream.Dispose() }
    Write-Host 'Created configuration with random secrets. Review the database URL, public URL and optional Google/admin settings before starting.'
} finally {
    $taskRng.Dispose()
    Remove-Variable taskTemplate, taskSecrets, taskSecret, taskBytes -ErrorAction SilentlyContinue
}
