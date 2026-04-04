@echo off
echo ====================================
echo Grain Backend - Go Build Script
echo ====================================
echo.

REM Check if Go is installed
where go >nul 2>nul
if %ERRORLEVEL% NEQ 0 (
    echo ERROR: Go is not installed or not in PATH
    echo Please install Go from https://golang.org/dl/
    exit /b 1
)

echo [1/4] Checking Go version...
go version
echo.

echo [2/4] Downloading dependencies...
go mod download
if %ERRORLEVEL% NEQ 0 (
    echo ERROR: Failed to download dependencies
    exit /b 1
)
echo.

echo [3/4] Tidying up modules...
go mod tidy
if %ERRORLEVEL% NEQ 0 (
    echo ERROR: Failed to tidy modules
    exit /b 1
)
echo.

echo [4/4] Building application...
go build -o grain_backend.exe main.go
if %ERRORLEVEL% NEQ 0 (
    echo ERROR: Build failed
    exit /b 1
)
echo.

echo ====================================
echo Build successful!
echo ====================================
echo.
echo To run the application:
echo   .\grain_backend.exe
echo.
echo Or for development mode with auto-reload:
echo   go run main.go
echo.
