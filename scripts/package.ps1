$ErrorActionPreference = 'Stop'

Push-Location (Join-Path $PSScriptRoot '..')
try {
    $env:GOOS = 'windows'
    $env:GOARCH = 'amd64'
    $env:CGO_ENABLED = '0'
    $manifest = Get-Content -Raw -LiteralPath 'mygo.json' | ConvertFrom-Json
    $output = Join-Path (Get-Location).Path "build/preview-$($manifest.version)"
    go tool mygo build -platform windows/amd64 -skip-installer -o $output
    if ($LASTEXITCODE -ne 0) { throw "MyGo build failed ($LASTEXITCODE)" }

    $build = (Resolve-Path (Join-Path $output 'windows-amd64')).Path
    $bundle = New-Item -ItemType Directory -Path (Join-Path $build 'ThirdParty') -Force
    $notices = @(
        '# goDisk third-party notices', '',
        'The portable archive includes goDisk and the components listed below.',
        'The optional benchmark dependency is Microsoft DiskSpd 2.3 x64.',
        'The setup script retrieves the Microsoft release separately. Its EULA is saved next to the executable.', '',
        '- [Microsoft DiskSpd 2.3 release](https://github.com/microsoft/diskspd/releases/tag/v2.3)', ''
    )
    $modules = go list -deps -f '{{if .Module}}{{.Module.Path}}|{{.Module.Version}}|{{.Module.Dir}}{{end}}' ./cmd/goDisk
    if ($LASTEXITCODE -ne 0) { throw "Unable to read Go runtime dependencies ($LASTEXITCODE)" }
    foreach ($module in ($modules | Where-Object { $_ } | Sort-Object -Unique)) {
        $name, $version, $source = $module.Split('|', 3)
        if ($name -eq 'goDisk') { continue }
        $id = ($name -replace '[^A-Za-z0-9._-]', '_') + '-' + $version
        $destination = New-Item -ItemType Directory -Path (Join-Path $bundle.FullName $id) -Force
        $files = @(Get-ChildItem -LiteralPath $source -File | Where-Object Name -Match '^(LICENSE|COPYING|COPYRIGHT|NOTICE)([._-].*)?$')
        if ($files.Count -eq 0) { throw "License files missing for $name $version" }
        foreach ($file in $files) {
            Copy-Item -LiteralPath $file.FullName -Destination $destination.FullName
        }
        $notices += ('- `{0}` {1} ([texts](ThirdParty/{2}/))' -f $name, $version, $id)
    }
    $goRoot = go env GOROOT
    if ($LASTEXITCODE -ne 0) { throw "Unable to read GOROOT ($LASTEXITCODE)" }
    Copy-Item -LiteralPath (Join-Path $goRoot 'LICENSE') -Destination (Join-Path $bundle.FullName 'Go-LICENSE')
    $notices += '- Go standard library ([license](ThirdParty/Go-LICENSE))'
    $notices | Set-Content -LiteralPath (Join-Path $build 'THIRD_PARTY_NOTICES.md') -Encoding utf8
    Copy-Item -LiteralPath 'docs/PREVIEW.md' -Destination (Join-Path $build 'README.md')
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'setup-diskspd.ps1') -Destination $build
    New-Item -ItemType Directory -Path 'dist' -Force | Out-Null
    $archive = Join-Path (Resolve-Path 'dist').Path "goDisk-$($manifest.version)-windows-x64-preview.zip"
    $paths = @('goDisk.exe', 'icon.png', 'README.md', 'THIRD_PARTY_NOTICES.md', 'ThirdParty', 'setup-diskspd.ps1') |
        ForEach-Object { Join-Path $build $_ }
    Compress-Archive -LiteralPath $paths -DestinationPath $archive -Force
    Get-FileHash -LiteralPath $archive -Algorithm SHA256
} finally {
    Pop-Location
}
