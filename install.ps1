#!/usr/bin/env pwsh
$ErrorActionPreference = "Stop"

$Repo = if ($env:WX_MINI_VIDEO_REPO) { $env:WX_MINI_VIDEO_REPO } else { "muzijin701/wx-mini-video" }
$InstallDir = if ($env:WX_MINI_VIDEO_INSTALL_DIR) { $env:WX_MINI_VIDEO_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "wx-mini-video" }
$LocalArchive = $env:WX_MINI_VIDEO_ARCHIVE
$NoModifyPath = $env:WX_MINI_VIDEO_NO_MODIFY_PATH -eq "1"

$TempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("wx-mini-video-install-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $TempDir | Out-Null
$BackupDir = Join-Path $TempDir "backup"
$ExtractDir = Join-Path $TempDir "extract"

try {
    if ($LocalArchive) {
        $ArchivePath = (Resolve-Path $LocalArchive).Path
    } else {
        $ReleaseApi = "https://api.github.com/repos/$Repo/releases/latest"
        Write-Host "Fetching latest release metadata from $Repo..."
        $Release = Invoke-RestMethod -Uri $ReleaseApi -Headers @{ "User-Agent" = "wx-mini-video-installer" }
        $Asset = $Release.assets | Where-Object { $_.name -eq "wx-mini-video-windows-amd64.zip" } | Select-Object -First 1
        if (-not $Asset) {
            throw "No Windows release asset 'wx-mini-video-windows-amd64.zip' found in latest release of $Repo."
        }
        $ArchivePath = Join-Path $TempDir $Asset.name
        Write-Host "Downloading $($Asset.name)..."
        Invoke-WebRequest -Uri $Asset.browser_download_url -OutFile $ArchivePath
    }

    # Preserve existing portable user config and downloads across upgrades
    if (Test-Path $InstallDir) {
        New-Item -ItemType Directory -Force -Path $BackupDir | Out-Null
        foreach ($item in @("wx-mini-video.yaml", "downloads")) {
            $existing = Join-Path $InstallDir $item
            if (Test-Path $existing) {
                Move-Item -Path $existing -Destination (Join-Path $BackupDir $item) -Force
            }
        }
        Remove-Item -Path $InstallDir -Recurse -Force
    }
    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

    Write-Host "Extracting to $InstallDir..."
    New-Item -ItemType Directory -Force -Path $ExtractDir | Out-Null
    Expand-Archive -Path $ArchivePath -DestinationPath $ExtractDir -Force

    $topEntries = @(Get-ChildItem -Path $ExtractDir)
    $sourceRoot = if ($topEntries.Count -eq 1 -and $topEntries[0].PSIsContainer -and -not (Test-Path (Join-Path $ExtractDir "wx-mini-video.exe"))) {
        $topEntries[0].FullName
    } else {
        $ExtractDir
    }
    Get-ChildItem -Path $sourceRoot -Force | ForEach-Object {
        Move-Item -Path $_.FullName -Destination (Join-Path $InstallDir $_.Name) -Force
    }

    foreach ($item in @("wx-mini-video.yaml", "downloads")) {
        $backedUp = Join-Path $BackupDir $item
        if (Test-Path $backedUp) {
            $dest = Join-Path $InstallDir $item
            if (Test-Path $dest) {
                Remove-Item -Path $dest -Recurse -Force
            }
            Move-Item -Path $backedUp -Destination $dest -Force
        }
    }

    if (-not $NoModifyPath) {
        $UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
        $Entries = @()
        if ($UserPath) {
            $Entries = $UserPath -split ";" | Where-Object { $_ -ne "" }
        }
        $AlreadyInPath = $Entries | Where-Object { $_.TrimEnd("\") -ieq $InstallDir.TrimEnd("\") }
        if (-not $AlreadyInPath) {
            $NewPath = ($Entries + $InstallDir) -join ";"
            [Environment]::SetEnvironmentVariable("Path", $NewPath, "User")
            Write-Host "Added $InstallDir to User PATH. Restart your terminal to pick up PATH changes."
        }
    }

    $ExePath = Join-Path $InstallDir "wx-mini-video.exe"
    if (-not (Test-Path $ExePath)) {
        throw "Installation verification failed: $ExePath not found."
    }

    Write-Host "Installed wx-mini-video successfully!"
    Write-Host "Run: `"$ExePath`""
} finally {
    if (Test-Path $TempDir) {
        Remove-Item -Path $TempDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}
