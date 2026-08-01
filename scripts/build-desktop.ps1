[CmdletBinding()]
param(
    [string]$Version = "dev",
    [string[]]$Runtimes = @(),
    [switch]$NoRestore
)

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot
$project = Join-Path $repoRoot "desktop\WhatGate.Desktop\WhatGate.Desktop.csproj"
$distRoot = Join-Path $repoRoot "dist\desktop"
$publishRoot = Join-Path $distRoot ".publish"
$runningOnWindows = [System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform(
    [System.Runtime.InteropServices.OSPlatform]::Windows)
$runningOnMacOs = [System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform(
    [System.Runtime.InteropServices.OSPlatform]::OSX)

if ($Runtimes.Count -eq 0) {
    $architecture = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
    $archName = if ($architecture -eq [System.Runtime.InteropServices.Architecture]::Arm64) {
        "arm64"
    }
    else {
        "x64"
    }
    $osName = if ($runningOnWindows) { "win" } elseif ($runningOnMacOs) { "osx" } else { "linux" }
    $Runtimes = @("$osName-$archName")
}

if (Test-Path $distRoot) {
    Remove-Item -LiteralPath $distRoot -Recurse -Force
}
New-Item -ItemType Directory -Force -Path $distRoot, $publishRoot | Out-Null

$targets = @{
    "win-x64"     = @{ GoOs = "windows"; GoArch = "amd64"; Extension = ".exe" }
    "win-arm64"   = @{ GoOs = "windows"; GoArch = "arm64"; Extension = ".exe" }
    "linux-x64"   = @{ GoOs = "linux";   GoArch = "amd64"; Extension = "" }
    "linux-arm64" = @{ GoOs = "linux";   GoArch = "arm64"; Extension = "" }
    "osx-x64"     = @{ GoOs = "darwin";  GoArch = "amd64"; Extension = "" }
    "osx-arm64"   = @{ GoOs = "darwin";  GoArch = "arm64"; Extension = "" }
}

Push-Location $repoRoot
try {
    foreach ($runtime in $Runtimes) {
        if (-not $targets.ContainsKey($runtime)) {
            throw "Unsupported runtime: $runtime"
        }

        $target = $targets[$runtime]
        $publishDirectory = Join-Path $publishRoot $runtime
        $packageName = "whatgate-desktop_${Version}_${runtime}"
        $stage = Join-Path $distRoot $packageName

        Write-Host "Building $runtime"
        if (-not $NoRestore) {
            dotnet restore $project -r $runtime -p:NuGetAudit=false
        }
        dotnet publish $project -c Release -r $runtime -o $publishDirectory `
            --no-restore `
            --self-contained true `
            -p:PublishSingleFile=true `
            -p:IncludeNativeLibrariesForSelfExtract=true `
            -p:DebugType=None `
            -p:DebugSymbols=false

        New-Item -ItemType Directory -Force -Path $stage | Out-Null
        Copy-Item -Path (Join-Path $publishDirectory "*") -Destination $stage -Recurse

        $coreDirectory = Join-Path $stage "core"
        New-Item -ItemType Directory -Force -Path $coreDirectory | Out-Null
        $coreName = "whatgate$($target.Extension)"
        $coordinatorName = "coordinator$($target.Extension)"

        $oldGoOs = $env:GOOS
        $oldGoArch = $env:GOARCH
        $oldCgo = $env:CGO_ENABLED
        try {
            $env:GOOS = $target.GoOs
            $env:GOARCH = $target.GoArch
            $env:CGO_ENABLED = "0"
            go build -trimpath -ldflags "-s -w -X main.version=$Version" `
                -o (Join-Path $coreDirectory $coreName) ./cmd/whatgate
            go build -trimpath -ldflags "-s -w -X main.version=$Version" `
                -o (Join-Path $coreDirectory $coordinatorName) ./cmd/coordinator
        }
        finally {
            $env:GOOS = $oldGoOs
            $env:GOARCH = $oldGoArch
            $env:CGO_ENABLED = $oldCgo
        }

        Copy-Item (Join-Path $repoRoot "desktop\README.md") (Join-Path $stage "README.md")
        if (Test-Path (Join-Path $repoRoot "LICENSE")) {
            Copy-Item (Join-Path $repoRoot "LICENSE") $stage
        }

        if ($runtime.StartsWith("win-")) {
            Copy-Item (Join-Path $repoRoot "desktop\packaging\windows\Install-WhatGate.ps1") $stage
            Copy-Item (Join-Path $repoRoot "desktop\packaging\windows\Install-WhatGate.cmd") $stage
            Compress-Archive -Path $stage -DestinationPath (Join-Path $distRoot "$packageName.zip")
        }
        elseif ($runtime.StartsWith("linux-")) {
            Copy-Item (Join-Path $repoRoot "desktop\packaging\linux\install.sh") $stage
            Copy-Item (Join-Path $repoRoot "desktop\packaging\linux\whatgate.desktop.in") $stage
            if (-not $runningOnWindows) {
                chmod +x (Join-Path $stage "WhatGate")
                chmod +x (Join-Path $coreDirectory "whatgate")
                chmod +x (Join-Path $coreDirectory "coordinator")
                chmod +x (Join-Path $stage "install.sh")
            }
            tar -czf (Join-Path $distRoot "$packageName.tar.gz") -C $distRoot $packageName
        }
        else {
            $appRoot = Join-Path $distRoot "WhatGate.app"
            $contents = Join-Path $appRoot "Contents"
            $macOs = Join-Path $contents "MacOS"
            New-Item -ItemType Directory -Force -Path $macOs | Out-Null
            Copy-Item -Path (Join-Path $stage "*") -Destination $macOs -Recurse
            Copy-Item (Join-Path $repoRoot "desktop\packaging\macos\Info.plist") $contents
            if (-not $runningOnWindows) {
                chmod +x (Join-Path $macOs "WhatGate")
                chmod +x (Join-Path $macOs "core\whatgate")
                chmod +x (Join-Path $macOs "core\coordinator")
            }
            $macArchive = Join-Path $distRoot "$packageName.zip"
            if ($runningOnMacOs) {
                ditto -c -k --sequesterRsrc --keepParent $appRoot $macArchive
            }
            else {
                Compress-Archive -Path $appRoot -DestinationPath $macArchive
            }
            Remove-Item -LiteralPath $appRoot -Recurse -Force
        }

        Remove-Item -LiteralPath $stage -Recurse -Force
    }
}
finally {
    Pop-Location
}

Remove-Item -LiteralPath $publishRoot -Recurse -Force
Get-ChildItem $distRoot -File | Sort-Object Name | Select-Object Name, Length
