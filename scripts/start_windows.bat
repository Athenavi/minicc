@echo off
setlocal enabledelayedexpansion

REM ============================================================
REM MiniCC  - Windows
REM ============================================================
REM Usage:
REM   start_windows.bat            Start all services in background
REM   start_windows.bat --fg       Start in foreground (for debugging)
REM   start_windows.bat setup      Install dependencies and start
REM   start_windows.bat status     Show service status
REM   start_windows.bat stop       Stop all services
REM   start_windows.bat restart    Restart all services
REM ============================================================

set "SCRIPT_DIR=%~dp0"
set "PROJECT_DIR=%SCRIPT_DIR%.."
cd /d "%PROJECT_DIR%"

echo.
echo  ========================================
echo        MiniCC Startup [Windows]
echo  ========================================
echo.

REM -- Check Python -------------------------------------------
where python >nul 2>&1
if %errorlevel% neq 0 (
    echo [ERROR] Python not found. Please install Python 3.9+
    echo         Download: https://www.python.org/downloads/
    pause
    exit /b 1
)

for /f "tokens=*" %%i in ('python --version 2^>^&1') do set "PY_VER=%%i"
echo [INFO] %PY_VER%

REM -- Check Go (only needed for setup/build/start) -----------
set "CMD=%~1"
if "%CMD%"=="" set "CMD=start"

if "%CMD%"=="setup" goto :check_go
if "%CMD%"=="start" goto :check_go_optional
goto :run

:check_go
where go >nul 2>&1
if %errorlevel% neq 0 (
    echo [ERROR] Go not found. Please install Go 1.21+
    echo         Download: https://go.dev/dl/
    pause
    exit /b 1
)
for /f "tokens=*" %%i in ('go version 2^>^&1') do echo [INFO] %%i
goto :run

:check_go_optional
where go >nul 2>&1
if %errorlevel% neq 0 (
    echo [WARN] Go not found, will skip compilation.
    echo.
)

:run
REM -- Execute run.py -----------------------------------------
set "EXTRA_ARGS="
if "%~1"=="" (
    set "EXTRA_ARGS=start"
) else if "%~1"=="--fg" (
    set "EXTRA_ARGS=start --fg"
) else if "%~1"=="setup" (
    echo.
    echo [Step 1/2] Installing dependencies...
    python run.py setup
    if %errorlevel% neq 0 (
        echo [ERROR] Dependency installation failed
        pause
        exit /b 1
    )
    echo.
    echo [Step 2/2] Starting services...
    set "EXTRA_ARGS=start"
) else (
    set "EXTRA_ARGS=%*"
)

echo.
python run.py %EXTRA_ARGS%
set "EXIT_CODE=%errorlevel%"

echo.
if %EXIT_CODE% equ 0 (
    echo [DONE] MiniCC started successfully.
    echo        Stop:   scripts\start_windows.bat stop
    echo        Status: scripts\start_windows.bat status
    echo        Logs:   python run.py logs
) else (
    echo [ERROR] Startup failed. Check logs for details.
)

echo.
pause
exit /b %EXIT_CODE%
