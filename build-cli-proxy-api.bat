@echo off
setlocal EnableExtensions

set "ROOT_DIR=%~dp0"
pushd "%ROOT_DIR%" >nul || (
    echo [ERROR] Failed to enter project root: "%ROOT_DIR%"
    exit /b 1
)

set "APP_NAME=cli-proxy-api.exe"
set "OUTPUT_PATH=%ROOT_DIR%%APP_NAME%"
set "MAIN_PACKAGE=./cmd/server"

where go >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Go is not installed or not available in PATH.
    popd >nul
    exit /b 1
)

if "%~1"=="" (
    if not defined GOARCH (
        for /f "usebackq delims=" %%I in (`go env GOARCH`) do set "GOARCH=%%I"
    )
) else (
    set "GOARCH=%~1"
)

for /f "usebackq delims=" %%I in (`git describe --tags --always 2^>nul`) do set "VERSION=%%I"
if not defined VERSION set "VERSION=dev"

for /f "usebackq delims=" %%I in (`git rev-parse --short HEAD 2^>nul`) do set "COMMIT=%%I"
if not defined COMMIT set "COMMIT=none"

for /f "usebackq delims=" %%I in (`powershell.exe -NoProfile -Command "(Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')"`) do set "BUILD_DATE=%%I"
if not defined BUILD_DATE set "BUILD_DATE=unknown"

set "CGO_ENABLED=0"
set "GOOS=windows"

echo [INFO] Building %APP_NAME%
echo [INFO] GOOS=%GOOS% GOARCH=%GOARCH%
echo [INFO] VERSION=%VERSION% COMMIT=%COMMIT% BUILD_DATE=%BUILD_DATE%

go build -trimpath -ldflags="-s -w -X main.Version=%VERSION% -X main.Commit=%COMMIT% -X main.BuildDate=%BUILD_DATE%" -o "%OUTPUT_PATH%" "%MAIN_PACKAGE%"
if errorlevel 1 (
    echo [ERROR] Build failed.
    popd >nul
    exit /b 1
)

echo [OK] Build completed: "%OUTPUT_PATH%"
popd >nul
exit /b 0
