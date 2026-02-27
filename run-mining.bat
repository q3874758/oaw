@echo off
echo ========================================
echo   OAW + PoLE Mining Suite
echo ========================================

echo [1/3] Starting PoLE node...
start "PoLE Node" cmd /k "cd /d D:\pole && pole-node.exe -data-dir D:\pole\data -rpc-port :9090 -p2p-port :26657"
echo     PoLE node started (port 9090)

echo.
echo [2/3] Waiting for node...
timeout /t 5 /nobreak >nul

echo [3/3] Starting OAW mining...
cd /d D:\oaw
call bin\oaw.exe mine start

echo.
echo ========================================
echo   All started!
echo   - PoLE node: http://localhost:9090
echo   - OAW mining: running
echo ========================================
pause
