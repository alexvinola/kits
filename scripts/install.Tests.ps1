#requires -Version 5.1
# Offline installer tests; no Pester, network, registry writes or admin required.
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'install.ps1')

function Assert-True {
    param([bool] $Condition, [string] $Message)
    if (-not $Condition) { throw $Message }
}

function Assert-Fails {
    param([scriptblock] $Action, [string] $Pattern)
    $failure = $null
    try { & $Action } catch { $failure = $_ }
    Assert-True ($null -ne $failure) 'Expected the installation to fail.'
    Assert-True ($failure.Exception.Message -match $Pattern) "Unexpected failure: $failure"
}

# Exercise native detection once, then select each release architecture in turn.
$native = Get-KitsArchitecture
Assert-True ($native -in 'amd64', 'arm64') 'Native architecture detection failed.'
function Get-KitsArchitecture { return $script:TestArchitecture }
function Get-KitsUserPath { return $script:TestUserPath }
function Set-KitsUserPath {
    param([string] $Value)
    $script:TestUserPath = $Value
    $script:PathWrites++
}
function Invoke-RestMethod {
    param([string] $Uri, $Headers)
    Assert-True ($Uri -eq 'https://api.github.com/repos/alexvinola/kits/releases/latest') 'Unexpected release lookup.'
    return @{ tag_name = 'v1.2.3' }
}
function Invoke-WebRequest {
    param([switch] $UseBasicParsing, [string] $Uri, [string] $OutFile)
    $script:Downloads += $Uri
    $script:DownloadDirs += [IO.Path]::GetDirectoryName($OutFile)
    if ($Uri.EndsWith('/checksums.txt')) {
        $asset = "kits-windows-$script:TestArchitecture.exe"
        $hash = $script:ExpectedHash
        switch ($script:DownloadMode) {
            'corrupt' { $hash = '0' * 64 }
            'missing' { $asset = 'another-binary.exe' }
            'malformed' { $hash = 'not-a-sha256' }
        }
        $content = "$hash  $asset`n"
        if ($script:DownloadMode -eq 'duplicate') { $content += $content }
        [IO.File]::WriteAllText($OutFile, $content)
    }
    else {
        Assert-True ($Uri.EndsWith("/kits-windows-$script:TestArchitecture.exe")) 'Wrong binary selected.'
        if ($script:DownloadMode -eq 'network-error') { throw 'Simulated failed download' }
        [IO.File]::WriteAllBytes($OutFile, $script:BinaryBytes)
    }
}

$testRoot = Join-Path ([IO.Path]::GetTempPath()) ('kits-installer-tests-' + [Guid]::NewGuid().ToString('N'))
$originalOS = $env:OS
$originalPath = $env:Path
$originalProtocol = [Net.ServicePointManager]::SecurityProtocol
try {
    $null = [IO.Directory]::CreateDirectory($testRoot)
    $env:OS = 'Windows_NT'
    $script:Downloads = @()
    $script:DownloadDirs = @()
    $script:DownloadMode = 'ok'
    $script:PathWrites = 0
    $script:BinaryBytes = [Text.Encoding]::UTF8.GetBytes('test release bytes, never executed')
    $fixture = Join-Path $testRoot 'fixture.exe'
    [IO.File]::WriteAllBytes($fixture, $script:BinaryBytes)
    $script:ExpectedHash = (Get-FileHash -LiteralPath $fixture -Algorithm SHA256).Hash

    foreach ($arch in 'amd64', 'arm64') {
        $script:TestArchitecture = $arch
        $directory = Join-Path $testRoot "install $arch"
        $script:TestUserPath = 'C:\Existing;C:\Other'
        $beforePath = $env:Path
        Install-Kits -Version '1.2.3' -InstallDir $directory -NoPath
        $installed = Join-Path $directory 'kits.exe'
        Assert-True ((Get-FileHash -LiteralPath $installed).Hash -eq $script:ExpectedHash) 'Wrong installed bytes.'
        Assert-True ($env:Path -ceq $beforePath) '-NoPath changed process PATH.'
        Assert-True ($script:PathWrites -eq 0) '-NoPath persisted PATH.'

        # Reinstallation is safe; latest resolves to one immutable tag.
        Install-Kits -InstallDir $directory
        $savedPath = $script:TestUserPath
        $writeCount = $script:PathWrites
        Install-Kits -Version 'v1.2.3' -InstallDir $directory
        Assert-True ($script:TestUserPath -ceq $savedPath) 'Repeated installation duplicated PATH.'
        Assert-True ($script:PathWrites -eq $writeCount) 'Repeated installation rewrote PATH.'
        Assert-True ($savedPath.EndsWith(';C:\Existing;C:\Other')) 'Existing user PATH was lost.'

        # A valid update replaces the previous binary.
        [IO.File]::WriteAllText($installed, 'previous release')
        Install-Kits -Version '1.2.3' -InstallDir $directory -NoPath
        Assert-True ((Get-FileHash -LiteralPath $installed).Hash -eq $script:ExpectedHash) 'Update failed.'

        # Bad downloads never replace an existing installation or write PATH.
        foreach ($mode in 'corrupt', 'missing', 'malformed', 'duplicate', 'network-error') {
            $script:DownloadMode = $mode
            $beforeHash = (Get-FileHash -LiteralPath $installed).Hash
            $writeCount = $script:PathWrites
            Assert-Fails { Install-Kits -Version '1.2.3' -InstallDir $directory } 'SHA-256|failed download'
            Assert-True ((Get-FileHash -LiteralPath $installed).Hash -eq $beforeHash) "Failure in $mode changed the binary."
            Assert-True ($script:PathWrites -eq $writeCount) "Failure in $mode changed PATH."
        }
        $script:DownloadMode = 'ok'
        $script:PathWrites = 0
    }

    $beforeDownloads = $script:Downloads.Count
    Assert-Fails { Install-Kits -Version '../../invalid' -InstallDir $testRoot } 'Invalid version'
    Assert-True ($script:Downloads.Count -eq $beforeDownloads) 'Invalid version triggered a download.'
    $env:OS = 'NotWindows'
    Assert-Fails { Install-Kits -InstallDir $testRoot } 'for Windows'

    $path = 'C:\Other;"C:\Tools\kits\"'
    Assert-True ((Add-KitsPathEntry -CurrentPath $path -Directory 'c:\tools\kits') -ceq $path) 'PATH comparison failed.'
    foreach ($directory in $script:DownloadDirs) {
        Assert-True (-not [IO.Directory]::Exists($directory)) 'Temporary download files were not cleaned up.'
    }
    Assert-True ($script:Downloads.Count -gt 0) 'No downloads were exercised.'
    foreach ($url in $script:Downloads) {
        Assert-True ($url.StartsWith('https://github.com/alexvinola/kits/releases/download/v1.2.3/')) 'Release tag was not pinned.'
    }
    Assert-True ([Net.ServicePointManager]::SecurityProtocol -eq $originalProtocol) 'TLS settings were not restored.'
    Write-Host 'Installer tests passed (x64/ARM64, version resolution, update, checksums, PATH and cleanup).'
}
finally {
    $env:OS = $originalOS
    $env:Path = $originalPath
    if ([IO.Directory]::Exists($testRoot)) { [IO.Directory]::Delete($testRoot, $true) }
}
