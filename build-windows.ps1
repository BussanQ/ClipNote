[CmdletBinding()]
param(
    [ValidateSet('amd64', 'arm64', 'all')]
    [string]$Arch = 'all'
)

$ErrorActionPreference = 'Stop'
Push-Location $PSScriptRoot
try {
    $targets = if ($Arch -eq 'all') { 'amd64', 'arm64' } else { $Arch }
    $platforms = ($targets | ForEach-Object { "windows/$_" }) -join ','
    go tool mygo build -platform $platforms -skip-installer
    if ($LASTEXITCODE -ne 0) { throw "Windows build failed ($LASTEXITCODE)" }

    foreach ($target in $targets) {
        $directory = "dist/windows-$target"
        $archive = "dist/ClipNote-windows-$target.zip"
        Compress-Archive -LiteralPath "$directory/ClipNote.exe", "$directory/LICENSE" -DestinationPath $archive -Force
        $hash = Get-FileHash -LiteralPath $archive -Algorithm SHA256
        Write-Output "$($hash.Hash)  $archive"
    }
} finally {
    Pop-Location
}
