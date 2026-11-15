@echo off
chcp 65001 >nul
title ShadowStar v2 - Gaming & TUN Mode
cd /d "%~dp0"

echo ====================================================
echo   ⭐ SHADOWSTAR v2 - L3 TUN & ONLINE GAMING ENGINE
echo   Поддержка игр: Billionaire Casino, Steam, Discord, UDP
echo ====================================================

:: Проверка прав администратора для L3 TUN
net session >nul 2>&1
if %errorLevel% neq 0 (
    echo [ИНФО] Для перехвата сетевого трафика (L3 Wintun) требуются права Администратора.
    echo Перезапуск с правами Администратора...
    powershell -Command "Start-Process '%~f0' -Verb RunAs"
    exit /b
)

if exist "shadowstar.exe" (
    echo Запуск ShadowStar (Wintun L3 + UDP Game Associate)...
    "shadowstar.exe" -c "shadowstar-client.personal.json" -tun -connect -tray
) else if exist "bin\shadowstar-v2.exe" (
    echo Запуск ShadowStar v2 (Wintun L3 + UDP Game Associate)...
    "bin\shadowstar-v2.exe" -c "shadowstar-client.personal.json" -tun -connect -tray
) else if exist "bin\shadowstar.exe" (
    echo Запуск ShadowStar (Wintun L3 + UDP Game Associate)...
    "bin\shadowstar.exe" -c "shadowstar-client.personal.json" -tun -connect -tray
) else (
    echo [ОШИБКА] shadowstar.exe не найден. Запустите сборку.
    pause
)
