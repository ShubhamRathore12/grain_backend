@echo off
REM ====================================
REM Grain Backend - API Test Script
REM ====================================

echo.
echo ====================================
echo Testing Grain Backend APIs
echo ====================================
echo.

REM Set base URL
set BASE_URL=http://localhost:8080

REM Check if server is running
echo [TEST 1] Checking server health...
curl -s %BASE_URL%/api/health >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo ERROR: Server is not running on %BASE_URL%
    echo Please start the server first: .\grain_backend.exe
    exit /b 1
)
echo ✓ Server is running
echo.

REM Test Health Endpoint
echo [TEST 2] Testing health endpoint...
curl -X GET %BASE_URL%/api/health
echo.
echo.

REM Test Login (you'll need to provide valid credentials)
echo [TEST 3] Testing login endpoint...
echo Enter username (or press Enter to skip):
set /p USERNAME=
if not "%USERNAME%"=="" (
    echo Enter password:
    set /p PASSWORD=
    curl -X POST %BASE_URL%/api/login ^
      -H "Content-Type: application/json" ^
      -d "{\"username\":\"%USERNAME%\",\"password\":\"%PASSWORD%\"}"
    echo.
    echo.
) else (
    echo Skipping login test (no credentials provided)
    echo.
)

REM Test Machine Status
echo [TEST 4] Testing machine status endpoint...
curl -X GET %BASE_URL%/api/machine/status
echo.
echo.

REM Test All Data
echo [TEST 5] Testing all data endpoint...
curl -X GET "%BASE_URL%/api/alldata/alldata?table=kabomachinedatasmart200"
echo.
echo.

REM Test Reports
echo [TEST 6] Testing reports endpoint...
curl -X GET "%BASE_URL%/api/reports/?table=kabomachinedatasmart200&page=1"
echo.
echo.

REM Test WebSocket info
echo [TEST 7] WebSocket endpoint available at:
echo %BASE_URL%/ws
echo Use a WebSocket client to test real-time connections
echo.

echo ====================================
echo API Tests Complete
echo ====================================
echo.
echo To test protected routes, use the token from login response
echo Example:
echo   curl -X GET %BASE_URL%/api/data/update ^
echo     -H "Authorization: Bearer YOUR_TOKEN_HERE" ^
echo     -H "Content-Type: application/json" ^
echo     -d "{\"test\":\"data\"}"
echo.
