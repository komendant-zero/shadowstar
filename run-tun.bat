@echo off
chcp 65001 >nul
title ShadowStar VPN (TUN Mode - Anti-DPI)

echo ====================================================
echo   ⭐ SHADOWSTAR STEALTH VPN (L3 TUN MODE)
echo ====================================================

:: Проверка прав администратора
net session >nul 2>&1
if %errorLevel% neq 0 (
    echo [ОШИБКА] Требуются права Администратора!
    echo Пожалуйста, запустите этот файл:
    echo   ПКМ по файлу -> "Запуск от имени администратора"
    echo.
    pause
    exit /b 1
)

:: Закрываем старые процессы
taskkill /f /im shadowstar.exe >nul 2>&1
taskkill /f /im tun2socks.exe >nul 2>&1

set BIN_DIR=%~dp0
set VPS_IP=179.253.74.36

:: Ищем физический шлюз по умолчанию
for /f "tokens=3" %%a in ('route print 0.0.0.0 ^| findstr "\<0.0.0.0\>" ^| findstr /v "10.8.1"') do (
    set DEFAULT_GW=%%a
    goto :gw_found
)
:gw_found

if "%DEFAULT_GW%"=="" (
    echo [ПРЕДУПРЕЖДЕНИЕ] Физический шлюз не определен автоматически.
    set DEFAULT_GW=192.168.50.1
)

echo [1/3] Запуск клиента ShadowStar (Stealth Anti-DPI)...
start "" /b "%BIN_DIR%shadowstar.exe" -headless -connect >nul 2>&1
timeout /t 2 /nobreak >nul

echo [2/3] Добавление прямого маршрута к VPS (%VPS_IP% через %DEFAULT_GW%)...
route add %VPS_IP% mask 255.255.255.255 %DEFAULT_GW% metric 1 >nul 2>&1

echo [3/3] Поднятие виртуального сетевого адаптера Wintun...
start "" /b "%BIN_DIR%tun2socks.exe" -device tun://ShadowStar -proxy socks5://127.0.0.1:10808 -loglevel error >nul 2>&1
timeout /t 2 /nobreak >nul

netsh interface ip set address name="ShadowStar" source=static addr=10.255.0.2 mask=255.255.255.0 gateway=none >nul 2>&1
route add 0.0.0.0 mask 128.0.0.0 10.255.0.1 metric 5 >nul 2>&1
route add 128.0.0.0 mask 128.0.0.0 10.255.0.1 metric 5 >nul 2>&1
netsh interface ip set dns name="ShadowStar" static 1.1.1.1 >nul 2>&1

echo.
echo ====================================================
echo   ✅ ТУННЕЛЬ УСПЕШНО АКТИВИРОВАН!
echo   Все программы на ПК (включая Antigravity и Gemini)
echo   теперь работают через stealth-протокол ShadowStar.
echo ====================================================
echo   Для отключения туннеля нажмите любую клавишу...
echo ====================================================
pause >nul

echo.
echo [ОТКЛЮЧЕНИЕ] Восстановление сетевых маршрутов...
taskkill /f /im tun2socks.exe >nul 2>&1
taskkill /f /im shadowstar.exe >nul 2>&1
route delete 0.0.0.0 mask 128.0.0.0 >nul 2>&1
route delete 128.0.0.0 mask 128.0.0.0 >nul 2>&1
route delete %VPS_IP% >nul 2>&1
echo [ГОТОВО] Туннель выключен, сетевой стек вернулся в исходное состояние.
timeout /t 2 >nul
