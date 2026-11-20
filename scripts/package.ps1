param([Parameter(Mandatory)][string]$DiskSpdDirectory)

$ErrorActionPreference = 'Stop'
$engineDirectory = (Resolve-Path -LiteralPath $DiskSpdDirectory).Path
$engineFile = Join-Path $engineDirectory 'diskspd.exe'
$engineLicense = Join-Path $engineDirectory 'LICENSE'
$engineHash = (Get-FileHash -LiteralPath $engineFile -Algorithm SHA256).Hash.ToLowerInvariant()
$originalLDFlags = $env:MYGO_LDFLAGS

Push-Location (Join-Path $PSScriptRoot '..')
try {
    $env:GOOS = 'windows'
    $env:GOARCH = 'amd64'
    $env:CGO_ENABLED = '0'
    $manifest = Get-Content -Raw -LiteralPath 'mygo.json' | ConvertFrom-Json
    $output = Join-Path (Get-Location).Path "build/release-$($manifest.version)"
    $env:MYGO_LDFLAGS = "$originalLDFlags -X goDisk/internal/benchmark.engineHash=$engineHash".Trim()
    go tool mygo build -platform windows/amd64 -skip-installer -o $output
    if ($LASTEXITCODE -ne 0) { throw "MyGo build failed ($LASTEXITCODE)" }

    $build = (Resolve-Path (Join-Path $output 'windows-amd64')).Path
    $bundledEngine = New-Item -ItemType Directory -Path (Join-Path $build 'diskspd') -Force
    Copy-Item -LiteralPath $engineFile -Destination $bundledEngine.FullName
    $noticeFile = Join-Path $build 'THIRD_PARTY_NOTICES.txt'
    @(
        'goDisk third-party notices', '',
        'Microsoft DiskSpd 2.3 x64',
        'Source: https://github.com/microsoft/diskspd/tree/5e7025bfc9d1364f185d4c30963d7ac79195435e', ''
        (Get-Content -LiteralPath $engineLicense -Raw)
    ) | Set-Content -LiteralPath $noticeFile -Encoding utf8
    $modules = go list -deps -f '{{if .Module}}{{.Module.Path}}|{{.Module.Version}}|{{.Module.Dir}}{{end}}' ./cmd/goDisk
    if ($LASTEXITCODE -ne 0) { throw "Unable to read Go runtime dependencies ($LASTEXITCODE)" }
    foreach ($module in ($modules | Where-Object { $_ } | Sort-Object -Unique)) {
        $name, $version, $source = $module.Split('|', 3)
        if ($name -eq 'goDisk') { continue }
        $files = @(Get-ChildItem -LiteralPath $source -File | Where-Object Name -Match '^(LICENSE|COPYING|COPYRIGHT|NOTICE)([._-].*)?$')
        if ($files.Count -eq 0) { throw "License files missing for $name $version" }
        foreach ($file in $files) {
            @('', "=== $name $version / $($file.Name) ===", '', (Get-Content -LiteralPath $file.FullName -Raw)) |
                Add-Content -LiteralPath $noticeFile -Encoding utf8
        }
    }
    $goRoot = go env GOROOT
    if ($LASTEXITCODE -ne 0) { throw "Unable to read GOROOT ($LASTEXITCODE)" }
    @('', '=== Go standard library / LICENSE ===', '', (Get-Content -LiteralPath (Join-Path $goRoot 'LICENSE') -Raw)) |
        Add-Content -LiteralPath $noticeFile -Encoding utf8
    Copy-Item -LiteralPath 'docs/PREVIEW.md' -Destination (Join-Path $build 'README.md')
    New-Item -ItemType Directory -Path 'dist' -Force | Out-Null
    $archive = Join-Path (Resolve-Path 'dist').Path "goDisk-$($manifest.version)-windows-x64.zip"
    $paths = @('goDisk.exe', 'README.md', 'THIRD_PARTY_NOTICES.txt', 'diskspd') |
        ForEach-Object { Join-Path $build $_ }
    Compress-Archive -LiteralPath $paths -DestinationPath $archive -Force
    Get-FileHash -LiteralPath $archive -Algorithm SHA256
} finally {
    $env:MYGO_LDFLAGS = $originalLDFlags
    Pop-Location
}
