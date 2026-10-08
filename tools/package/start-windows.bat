@echo off
chcp 65001 >nul
cd /d "%~dp0"
set RP_ADDR=127.0.0.1:8087
set RP_BANK_DIR=bank
set RP_DATA_DIR=data
set RP_FRONTEND_DIR=public
echo Тренажёр ПДД: http://127.0.0.1:8087
echo Чтобы остановить, закройте это окно.
start "" cmd /c "timeout /t 2 >nul & start http://127.0.0.1:8087"
bin\roadpolice-web-windows-amd64.exe
pause
