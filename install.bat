@echo off
setlocal EnableExtensions

set "ROOT_DIR=%~dp0"
set "BUILD_SCRIPT=%ROOT_DIR%build-cli-proxy-api.bat"
set "APP_NAME=cli-proxy-api.exe"
set "APP_TARGET=%ROOT_DIR%%APP_NAME%"
set "CONFIG_EXAMPLE=%ROOT_DIR%config.example.yaml"
set "CONFIG_TARGET=%ROOT_DIR%config.yaml"
set "RELEASE_API_URL=https://api.github.com/repos/ZhuofanWu/CLIProxyAPI/releases/latest"
set "INSTALL_MODE=download"
set "TMP_DIR="
set "ZIP_PATH="

if exist "%BUILD_SCRIPT%" if exist "%CONFIG_EXAMPLE%" if exist "%ROOT_DIR%cmd\server\main.go" (
    set "INSTALL_MODE=build"
)

echo [INFO] Install directory: "%ROOT_DIR%"

if /I "%INSTALL_MODE%"=="build" goto build_local
goto download_release

:build_local
where go >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Go is not installed or not available in PATH.
    exit /b 1
)

echo [INFO] Local source detected. Building from current workspace...
call "%BUILD_SCRIPT%"
if errorlevel 1 (
    echo [ERROR] Build failed.
    exit /b 1
)

if not exist "%APP_TARGET%" (
    echo [ERROR] Build finished but "%APP_TARGET%" was not found.
    exit /b 1
)

goto ensure_config

:download_release
where powershell.exe >nul 2>&1
if errorlevel 1 (
    echo [ERROR] PowerShell is required for downloading the release package.
    exit /b 1
)

call :detect_arch
if errorlevel 1 exit /b 1

set "TMP_DIR=%TEMP%\cli-proxy-api-install-%RANDOM%%RANDOM%"
set "ZIP_PATH=%TMP_DIR%\release.zip"

mkdir "%TMP_DIR%" >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Failed to create temporary directory: "%TMP_DIR%"
    exit /b 1
)

echo [INFO] Downloading latest Windows %ASSET_ARCH% release...
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command ^
    "$ErrorActionPreference = 'Stop';" ^
    "$release = Invoke-RestMethod -Headers @{ 'User-Agent' = 'CLIProxyAPI-Installer' } -Uri $env:RELEASE_API_URL;" ^
    "$pattern = if ($env:ASSET_ARCH -eq 'amd64') { 'amd64|x86_64' } elseif ($env:ASSET_ARCH -eq 'arm64') { 'arm64|aarch64' } else { [Regex]::Escape($env:ASSET_ARCH) };" ^
    "$asset = $release.assets | Where-Object { $_.name -match 'windows' -and $_.name -match $pattern -and $_.name -match '\.zip$' } | Select-Object -First 1;" ^
    "if (-not $asset) { throw 'matching Windows release asset not found' };" ^
    "Invoke-WebRequest -Headers @{ 'User-Agent' = 'CLIProxyAPI-Installer' } -Uri $asset.browser_download_url -OutFile $env:ZIP_PATH;" ^
    "Expand-Archive -LiteralPath $env:ZIP_PATH -DestinationPath $env:TMP_DIR -Force;" ^
    "$exe = Get-ChildItem -Path $env:TMP_DIR -Filter $env:APP_NAME -Recurse -File | Select-Object -First 1;" ^
    "if (-not $exe) { throw 'cli-proxy-api.exe not found in archive' };" ^
    "Copy-Item -Force $exe.FullName (Join-Path $env:ROOT_DIR $env:APP_NAME);" ^
    "$configExample = Get-ChildItem -Path $env:TMP_DIR -Filter 'config.example.yaml' -Recurse -File | Select-Object -First 1;" ^
    "if (-not $configExample) { throw 'config.example.yaml not found in archive' };" ^
    "$configTarget = Join-Path $env:ROOT_DIR 'config.yaml';" ^
    "if (-not (Test-Path -LiteralPath $configTarget)) { Copy-Item -Force $configExample.FullName $configTarget }"
if errorlevel 1 (
    echo [ERROR] Failed to download or extract the latest release.
    call :cleanup
    exit /b 1
)

call :cleanup
goto report_result

:ensure_config
if exist "%CONFIG_TARGET%" (
    echo [INFO] Existing config preserved: "%CONFIG_TARGET%"
) else (
    copy /Y "%CONFIG_EXAMPLE%" "%CONFIG_TARGET%" >nul
    if errorlevel 1 (
        echo [ERROR] Failed to create "%CONFIG_TARGET%"
        exit /b 1
    )
    echo [OK] Created config file: "%CONFIG_TARGET%"
)

goto report_result

:report_result
if not exist "%APP_TARGET%" (
    echo [ERROR] Installation failed: "%APP_TARGET%" was not found.
    exit /b 1
)

if exist "%CONFIG_TARGET%" (
    echo [OK] Installation files are ready in "%ROOT_DIR%"
) else (
    echo [WARN] "%APP_NAME%" is ready, but "%CONFIG_TARGET%" was not created.
    exit /b 1
)

exit /b 0

:detect_arch
set "RAW_ARCH=%PROCESSOR_ARCHITECTURE%"
if defined PROCESSOR_ARCHITEW6432 set "RAW_ARCH=%PROCESSOR_ARCHITEW6432%"

set "ASSET_ARCH="
if /I "%RAW_ARCH%"=="AMD64" set "ASSET_ARCH=amd64"
if /I "%RAW_ARCH%"=="ARM64" set "ASSET_ARCH=arm64"

if not defined ASSET_ARCH (
    echo [ERROR] Unsupported Windows architecture: "%RAW_ARCH%"
    exit /b 1
)

exit /b 0

:cleanup
if defined TMP_DIR if exist "%TMP_DIR%" (
    rmdir /s /q "%TMP_DIR%" >nul 2>&1
)
exit /b 0
