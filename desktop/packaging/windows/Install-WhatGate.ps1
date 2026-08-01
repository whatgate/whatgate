param(
    [string]$InstallDirectory = (Join-Path $env:LOCALAPPDATA "Programs\WhatGate")
)

$ErrorActionPreference = "Stop"
$packageDirectory = Split-Path -Parent $MyInvocation.MyCommand.Path

New-Item -ItemType Directory -Force -Path $InstallDirectory | Out-Null
Copy-Item -Path (Join-Path $packageDirectory "*") -Destination $InstallDirectory -Recurse -Force

$shell = New-Object -ComObject WScript.Shell
$shortcut = $shell.CreateShortcut(
    (Join-Path ([Environment]::GetFolderPath("Desktop")) "WhatGate.lnk")
)
$shortcut.TargetPath = Join-Path $InstallDirectory "WhatGate.exe"
$shortcut.WorkingDirectory = $InstallDirectory
$shortcut.Description = "WhatGate secure network client"
$shortcut.Save()

Start-Process (Join-Path $InstallDirectory "WhatGate.exe")
Write-Host "WhatGate was installed to $InstallDirectory"
