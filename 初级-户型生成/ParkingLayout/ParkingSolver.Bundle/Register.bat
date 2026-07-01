@echo off
echo === 注册 ParkingSolver COM 组件 ===
echo.
echo 请以管理员身份运行此脚本！
echo.

:: 找到 .NET 4.0/4.5+ 的 regasm
set REGASM_PATH=
if exist "%SystemRoot%\Microsoft.NET\Framework64\v4.0.30319\regasm.exe" (
    set REGASM_PATH=%SystemRoot%\Microsoft.NET\Framework64\v4.0.30319\regasm.exe
) else if exist "%SystemRoot%\Microsoft.NET\Framework\v4.0.30319\regasm.exe" (
    set REGASM_PATH=%SystemRoot%\Microsoft.NET\Framework\v4.0.30319\regasm.exe
)

if "%REGASM_PATH%"=="" (
    echo 错误: 未找到 regasm.exe
    pause
    exit /b 1
)

:: 注册 DLL
"%REGASM_PATH%" "%~dp0Contents\ParkingSolver.dll" /codebase

if %ERRORLEVEL% EQU 0 (
    echo.
    echo 注册成功！现在启动 AutoCAD 并输入 GTCW
) else (
    echo.
    echo 注册失败，请以管理员身份运行
)

pause
