@echo off
echo =========================================================================
echo  BUILDING POWER PLANT SCADA - NATIVE WINDOWS DESKTOP APPLICATION
echo =========================================================================

set GO_EXE="C:\Program Files\Go\bin\go.exe"

if not exist %GO_EXE% (
    set GO_EXE=go
)

echo.
echo [1/2] Building Console Desktop Binary (scada-desktop-console.exe)...
%GO_EXE% build -o scada-desktop-console.exe .
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Console build failed!
    exit /b %ERRORLEVEL%
)

echo.
echo [2/2] Building Pure Native Desktop GUI Binary (scada-desktop.exe)...
%GO_EXE% build -ldflags="-H windowsgui" -o scada-desktop.exe .
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Native GUI build failed!
    exit /b %ERRORLEVEL%
)

echo.
echo =========================================================================
echo  SUCCESS: Executables created!
echo   - scada-desktop.exe        (Native Desktop Window, No Console Window)
echo   - scada-desktop-console.exe (Native Desktop Window + Diagnostic Console)
echo =========================================================================
