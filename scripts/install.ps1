#requires -Version 5.1
<#
.SYNOPSIS
Install or update kits for the current Windows user.
.EXAMPLE
.\install.ps1
.EXAMPLE
.\install.ps1 -Version 0.1.0 -InstallDir C:\Tools\kits -NoPath
.NOTES
Requires access to the public GitHub release. Checksums detect corrupt downloads;
they are fetched from the same release, not an independent signature.
#>
[CmdletBinding()]
param(
    [string] $Version = 'latest',
    [string] $InstallDir,
    [switch] $NoPath
)

function Get-KitsArchitecture {
    # OS architecture also selects the native binary from an emulated shell.
    try {
        $architecture = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
    }
    catch {
        $architecture = $env:PROCESSOR_ARCHITEW6432
        if (-not $architecture) { $architecture = $env:PROCESSOR_ARCHITECTURE }
    }
    switch ($architecture) {
        { $_ -in 'X64', 'AMD64' } { return 'amd64' }
        'Arm64' { return 'arm64' }
        default { throw "Unsupported Windows architecture: $architecture. kits requires x64 or ARM64." }
    }
}

function Add-KitsPathEntry {
    param([AllowNull()][string] $CurrentPath, [string] $Directory)
    $normalized = $Directory.TrimEnd('\', '/')
    foreach ($entry in ($CurrentPath -split ';')) {
        $expanded = [Environment]::ExpandEnvironmentVariables($entry.Trim().Trim('"'))
        if ($expanded.TrimEnd('\', '/') -ieq $normalized) { return $CurrentPath }
    }
    if ([string]::IsNullOrEmpty($CurrentPath)) { return $Directory }
    return "$Directory;$CurrentPath"
}

function Get-KitsUserPath {
    return [Environment]::GetEnvironmentVariable('Path', 'User')
}

function Set-KitsUserPath {
    param([string] $Value)
    [Environment]::SetEnvironmentVariable('Path', $Value, 'User')
}

function Install-Kits {
    [CmdletBinding()]
    param(
        [string] $Version = 'latest',
        [string] $InstallDir,
        [switch] $NoPath
    )

    Set-StrictMode -Version Latest
    $ErrorActionPreference = 'Stop'
    if ($env:OS -ne 'Windows_NT') { throw 'This installer is for Windows. Use Homebrew or a release binary on macOS/Linux.' }
    if (-not $InstallDir) {
        $localAppData = [Environment]::GetFolderPath('LocalApplicationData')
        if (-not $localAppData) { throw 'Cannot locate LocalAppData. Specify -InstallDir.' }
        $InstallDir = Join-Path $localAppData 'Programs\kits\bin'
    }
    $InstallDir = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($InstallDir)
    if ($InstallDir.Contains(';')) { throw 'InstallDir cannot contain a semicolon (Windows PATH separator).' }

    $architecture = Get-KitsArchitecture
    $asset = "kits-windows-$architecture.exe"
    $headers = @{ 'User-Agent' = 'kits-installer'; 'Accept' = 'application/vnd.github+json' }
    $previousProtocol = [Net.ServicePointManager]::SecurityProtocol
    $tempDir = $null
    $staged = $null
    try {
        # Windows PowerShell 5.1 may otherwise negotiate an obsolete TLS version.
        [Net.ServicePointManager]::SecurityProtocol = $previousProtocol -bor [Net.SecurityProtocolType]::Tls12
        if ($Version -eq 'latest') {
            $release = Invoke-RestMethod -Uri 'https://api.github.com/repos/alexvinola/kits/releases/latest' -Headers $headers
            $Version = $release.tag_name
        }
        if ($Version -notmatch '^v?(\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?)$') {
            throw "Invalid version '$Version'; use latest or a version such as 0.1.0."
        }
        $tag = "v$($Matches[1])"
        $releaseUrl = 'https://github.com/alexvinola/kits/releases/download/' + [Uri]::EscapeDataString($tag)
        $tempDir = Join-Path ([IO.Path]::GetTempPath()) ('kits-install-' + [Guid]::NewGuid().ToString('N'))
        $null = [IO.Directory]::CreateDirectory($tempDir)
        $download = Join-Path $tempDir $asset
        $checksumFile = Join-Path $tempDir 'checksums.txt'
        Write-Host "Downloading kits $tag (windows/$architecture)..."
        Invoke-WebRequest -UseBasicParsing -Uri "$releaseUrl/$asset" -OutFile $download
        Invoke-WebRequest -UseBasicParsing -Uri "$releaseUrl/checksums.txt" -OutFile $checksumFile

        $pattern = '^([0-9a-fA-F]{64})\s+\*?' + [regex]::Escape($asset) + '$'
        $digests = @(foreach ($line in [IO.File]::ReadAllLines($checksumFile)) {
            if ($line -match $pattern) { $Matches[1] }
        })
        if ($digests.Count -ne 1) { throw "Expected exactly one valid SHA-256 entry for $asset in checksums.txt." }
        $actual = (Get-FileHash -LiteralPath $download -Algorithm SHA256).Hash
        if ($actual -ine $digests[0]) { throw "SHA-256 mismatch for $asset. The existing installation was not changed." }

        # Stage on the destination filesystem; replace only after verification.
        $null = [IO.Directory]::CreateDirectory($InstallDir)
        $destination = Join-Path $InstallDir 'kits.exe'
        $staged = Join-Path $InstallDir ('.kits-' + [Guid]::NewGuid().ToString('N') + '.exe')
        [IO.File]::Copy($download, $staged)
        if ([IO.File]::Exists($destination)) {
            if ((Get-FileHash -LiteralPath $destination -Algorithm SHA256).Hash -ine $actual) {
                try { [IO.File]::Replace($staged, $destination, [NullString]::Value) }
                catch { throw "Could not replace '$destination'. Close running kits processes and retry. $($_.Exception.Message)" }
            }
        }
        else {
            [IO.File]::Move($staged, $destination)
        }

        if (-not $NoPath) {
            $userPath = Get-KitsUserPath
            $newUserPath = Add-KitsPathEntry -CurrentPath $userPath -Directory $InstallDir
            if ($newUserPath -cne $userPath) { Set-KitsUserPath -Value $newUserPath }
            $env:Path = Add-KitsPathEntry -CurrentPath $env:Path -Directory $InstallDir
        }
        Write-Host "Installed kits $tag to $destination"
        if (-not $NoPath) { Write-Host 'Open a new terminal if kits is not available in your current shell.' }
        if (-not (Get-Command git -CommandType Application -ErrorAction SilentlyContinue)) {
            Write-Warning 'kits init also needs Git. Install Git for Windows and authenticate to your kit repository.'
        }
    }
    finally {
        [Net.ServicePointManager]::SecurityProtocol = $previousProtocol
        if ($staged -and [IO.File]::Exists($staged)) { [IO.File]::Delete($staged) }
        if ($tempDir -and [IO.Directory]::Exists($tempDir)) { [IO.Directory]::Delete($tempDir, $true) }
    }
}

# Dot-sourcing exposes functions for the offline tests without installing anything.
if ($MyInvocation.InvocationName -ne '.') {
    try { Install-Kits @PSBoundParameters }
    catch { Write-Error -ErrorAction Continue $_; exit 1 }
}
