param(
    [string]$Destination = (Join-Path $PSScriptRoot 'diskspd'),
    [string]$Archive
)

$ErrorActionPreference = 'Stop'
$archiveHash = '18609981E8B04B4F645F1FD182AD0541615FE71722DE36A9A933C54F747218A3'
$engineHash = 'DD4E57E1E8CCAF5D6437938F8AAB7F17E9A1E6D8FBA8A093006B7CADF16FAEA2'
$engine = Join-Path $Destination 'diskspd.exe'
$license = Join-Path $Destination 'EULA.txt'

function Test-Digest($path, $expected) {
    (Test-Path -LiteralPath $path -PathType Leaf) -and ((Get-FileHash -LiteralPath $path).Hash -eq $expected)
}

if ((Test-Digest $engine $engineHash) -and (Test-Path -LiteralPath $license -PathType Leaf)) {
    Write-Output "Microsoft DiskSpd 2.3 is ready: $engine"
    return
}

$download = $null
$stagedEngine = $null
$stagedLicense = $null
try {
    if (-not $Archive) {
        $download = [IO.Path]::GetTempFileName()
        Invoke-WebRequest -UseBasicParsing -Uri 'https://github.com/microsoft/diskspd/releases/download/v2.3/DiskSpd.zip' -OutFile $download -TimeoutSec 90
        $Archive = $download
    }
    if (-not (Test-Digest $Archive $archiveHash)) { throw 'Microsoft DiskSpd 2.3 ZIP SHA-256 mismatch.' }

    New-Item -ItemType Directory -Path $Destination -Force | Out-Null
    $id = [guid]::NewGuid().ToString('N')
    $stagedEngine = Join-Path $Destination "$id.exe.tmp"
    $stagedLicense = Join-Path $Destination "$id.txt.tmp"
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $zip = [System.IO.Compression.ZipFile]::OpenRead((Resolve-Path -LiteralPath $Archive).Path)
    try {
        [System.IO.Compression.ZipFileExtensions]::ExtractToFile($zip.GetEntry('amd64/diskspd.exe'), $stagedEngine)
        [System.IO.Compression.ZipFileExtensions]::ExtractToFile($zip.GetEntry('EULA'), $stagedLicense)
    } finally {
        $zip.Dispose()
    }
    if (-not (Test-Digest $stagedEngine $engineHash)) { throw 'Microsoft DiskSpd 2.3 x64 SHA-256 mismatch.' }
    Move-Item -LiteralPath $stagedLicense -Destination $license -Force
    Move-Item -LiteralPath $stagedEngine -Destination $engine -Force
    Write-Output "Microsoft DiskSpd 2.3 is ready: $engine"
    Write-Output "Microsoft software license terms: $license"
} finally {
    foreach ($temporary in @($download, $stagedEngine, $stagedLicense)) {
        if ($temporary -and (Test-Path -LiteralPath $temporary)) {
            Remove-Item -LiteralPath $temporary
        }
    }
}
