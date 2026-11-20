param(
    [Parameter(Mandatory)][string]$Source,
    [Parameter(Mandatory)][string]$Destination
)

$ErrorActionPreference = 'Stop'
$sourcePath = (Resolve-Path -LiteralPath $Source).Path
$expectedCommit = '5e7025bfc9d1364f185d4c30963d7ac79195435e'
$commit = git -C $sourcePath rev-parse HEAD
if ($LASTEXITCODE -ne 0 -or $commit -cne $expectedCommit) {
    throw "DiskSpd source checkout must be Microsoft v2.3 ($expectedCommit)."
}

$vswhere = Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio/Installer/vswhere.exe'
$msbuild = & $vswhere -latest -products '*' -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 `
    -find 'MSBuild\Current\Bin\amd64\MSBuild.exe' | Select-Object -First 1
if (-not $msbuild) { throw 'Visual Studio C++ build tools are required to build DiskSpd.' }

$projects = Join-Path $sourcePath 'diskspd_vs'
$propsPath = Join-Path $projects 'DiskSpdBuild.props'
$originalProps = [IO.File]::ReadAllBytes($propsPath)
[xml]$props = Get-Content -Raw -LiteralPath $propsPath
$ns = [System.Xml.XmlNamespaceManager]::new($props.NameTable)
$ns.AddNamespace('ms', $props.DocumentElement.NamespaceURI)
$releaseCompile = $props.SelectNodes('/ms:Project/ms:ItemDefinitionGroup/ms:ClCompile', $ns) |
    Where-Object { $_.ParentNode.GetAttribute('Condition') -like "*=='Release'" }
if (@($releaseCompile).Count -ne 1) { throw 'DiskSpd Release compiler settings were not found.' }

# Link the C++ runtime statically so the portable package can run directly.
$runtime = $props.CreateElement('RuntimeLibrary', $props.DocumentElement.NamespaceURI)
$runtime.InnerText = 'MultiThreaded'
$releaseCompile.AppendChild($runtime) | Out-Null
try {
    $props.Save($propsPath)
    & $msbuild (Join-Path $projects 'diskspd.sln') /m /t:CmdRequestCreator /p:Configuration=Release /p:Platform=x64 /p:PlatformToolset=v143 /verbosity:minimal
    if ($LASTEXITCODE -ne 0) { throw "DiskSpd build failed ($LASTEXITCODE)." }
} finally {
    [IO.File]::WriteAllBytes($propsPath, $originalProps)
}

New-Item -ItemType Directory -Path $Destination -Force | Out-Null
Copy-Item -LiteralPath (Join-Path $projects 'Release/x64/diskspd.exe') -Destination $Destination
Copy-Item -LiteralPath (Join-Path $sourcePath 'LICENSE') -Destination $Destination
