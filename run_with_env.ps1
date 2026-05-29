# Grain Backend - Run with Environment Variables
# This script ensures environment variables are set correctly

Write-Host "=====================================" -ForegroundColor Cyan
Write-Host "Grain Backend - Starting Server" -ForegroundColor Cyan
Write-Host "=====================================" -ForegroundColor Cyan
Write-Host ""

# Set database configuration
Write-Host "[1/3] Setting up environment variables..." -ForegroundColor Yellow
$env:DB_HOST = "myshaa.com"
$env:DB_PORT = "3306"
$env:DB_USER = "myshaa_kabu"
$env:DB_PASSWORD = "T-Cyj;f5g1y6"
$env:DB_NAME = "myshaa_kabu"
$env:JWT_SECRET = "21321esaendjnasjdasjdnjasbndjqwbeijn2jebjbjbjnjnj"
$env:PORT = "8080"
$env:CORS_ORIGIN = "https://new-plc-software-5xyc.vercel.app"
$env:CORS_ALLOWED_ORIGINS = "https://new-plc-software-5xyc.vercel.app,http://localhost:3000,http://localhost:3001"

Write-Host "✅ Environment variables configured" -ForegroundColor Green
Write-Host ""

# Navigate to script directory
Set-Location $PSScriptRoot

Write-Host "[2/3] Starting Grain Backend Server..." -ForegroundColor Yellow
Write-Host ""

# Run the executable
.\grain_backend.exe
