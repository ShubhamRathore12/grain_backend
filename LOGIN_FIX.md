# 🔧 Login API Troubleshooting Guide

## Issue
Login API returning: `{ "message": "Server error while logging in" }`

## Root Cause
The Go application is not loading the `.env` file properly, causing database connection failures.

## Current Status

### Server Logs Show:
```
2026/04/02 12:06:32 No .env file found, using environment variables
2026/04/02 12:06:32 ⚠️  Database connection failed: Error 1045 (28000): Access denied for user 'root'@'localhost' (using password: NO)
```

**Problem:** Application is using default credentials (`root@localhost` with NO password) instead of the credentials in your `.env` file.

## Solutions

### Solution 1: Fix .env File Loading (Recommended)

The issue is that `godotenv.Load()` doesn't find the `.env` file. We need to ensure it loads from the correct path.

**Updated main.go code:**
```go
func main() {
    // Try multiple paths for .env file
    envPaths := []string{".env", "../.env", "./.env"}
    envLoaded := false
    
    for _, path := range envPaths {
        if err := godotenv.Load(path); err == nil {
            log.Printf("✅ .env file loaded from: %s", path)
            envLoaded = true
            break
        }
    }
    
    if !envLoaded {
        log.Println("⚠️  No .env file found, using environment variables")
    }
    
    cfg := config.GetConfig()
    // ... rest of code
}
```

### Solution 2: Set Environment Variables Directly

Temporarily set environment variables in PowerShell before running:

```powershell
$env:DB_HOST="myshaa.com"
$env:DB_PORT="3306"
$env:DB_USER="myshaa_kabu"
$env:DB_PASSWORD="'T-Cyj;f5g1y6'"
$env:DB_NAME="myshaa_kabu"
$env:JWT_SECRET="21321esaendjnasjdasjdnjasbndjqwbeijn2jebjbjbjnjnj"
$env:PORT="8080"

go run main.go
```

### Solution 3: Use Absolute Path in Code

Modify `main.go` to use absolute path:

```go
import (
    "path/filepath"
    "runtime"
)

func main() {
    // Get current directory
    _, filename, _, _ := runtime.Caller(0)
    dir := filepath.Dir(filename)
    envPath := filepath.Join(dir, ".env")
    
    if err := godotenv.Load(envPath); err != nil {
        log.Printf("⚠️  Could not load .env from %s: %v", envPath, err)
    } else {
        log.Printf("✅ Loaded .env from: %s", envPath)
    }
    
    // ... rest of code
}
```

### Solution 4: Verify .env File Exists

Check if .env file actually exists and is readable:

```powershell
# Check file exists
Test-Path .\.env

# Check file contents
Get-Content .\.env

# Check file permissions
Get-Acl .\.env | Format-List
```

## Testing Credentials

### Test User Information
Based on your request:
- **Username:** Narayan12
- **Password:** Naruto@123

### Verify User Exists in Database

Connect to your MySQL database and run:

```sql
USE myshaa_kabu;

SELECT id, username, accountType, firstName, lastName 
FROM kabu_users 
WHERE username = 'Narayan12';
```

If the user doesn't exist, create it:

```sql
INSERT INTO kabu_users 
(username, password, accountType, firstName, lastName, created_at) 
VALUES 
('Narayan12', 'Naruto@123', 'user', 'Narayan', '', NOW());
```

## Quick Fix Steps

1. **Stop the current server** (Ctrl+C in terminal)

2. **Verify .env file exists:**
   ```powershell
   Test-Path d:\new_project\grain_backend\grain_backend\.env
   ```

3. **Set environment variables manually:**
   ```powershell
   $env:DB_HOST="myshaa.com"
   $env:DB_USER="myshaa_kabu"
   $env:DB_PASSWORD="T-Cyj;f5g1y6"
   $env:DB_NAME="myshaa_kabu"
   ```

4. **Run the server:**
   ```powershell
   cd d:\new_project\grain_backend\grain_backend
   go run main.go
   ```

5. **Test login:**
   ```powershell
   Invoke-RestMethod -Uri "http://localhost:8080/api/login" `
     -Method POST `
     -ContentType "application/json" `
     -Body '{"username":"Narayan12","password":"Naruto@123"}'
   ```

## Expected Response

If everything works correctly, you should receive:

```json
{
  "message": "Login successful",
  "user": {
    "id": 1,
    "username": "Narayan12",
    "accountType": "user",
    "firstName": "Narayan",
    "lastName": "",
    "email": null,
    "phoneNumber": null,
    "company": null,
    "monitorAccess": "",
    "location": ""
  },
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

And a cookie named `auth_token` will be set in the response.

## Debug Checklist

- [ ] .env file exists at `d:\new_project\grain_backend\grain_backend\.env`
- [ ] .env file contains correct database credentials
- [ ] Database server is accessible from your machine
- [ ] User 'myshaa_kabu' has permission to access database 'myshaa_kabu'
- [ ] User 'Narayan12' exists in `kabu_users` table
- [ ] Password matches exactly (case-sensitive)
- [ ] Port 8080 is not blocked by firewall
- [ ] Go application can read .env file (permissions)

## Alternative: Build and Run Executable

Instead of `go run main.go`, try building and running the executable:

```powershell
cd d:\new_project\grain_backend\grain_backend
.\build.bat
.\grain_backend.exe
```

The executable often handles paths better than `go run`.

## Contact Support

If none of these solutions work:
1. Check server logs for detailed error messages
2. Verify database connectivity separately
3. Test with a simple Go program that loads .env
4. Consider deploying to Render/cloud where environment variables are set in platform

---

**Last Updated:** 2026-04-02  
**Issue Status:** Investigating  
**Priority:** High
