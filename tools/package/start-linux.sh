#!/bin/sh
# Запускает тренажёр и открывает его в браузере.
cd "$(dirname "$0")"
export RP_ADDR=127.0.0.1:8087 RP_BANK_DIR=bank RP_DATA_DIR=data RP_FRONTEND_DIR=public
echo "Тренажёр ПДД: http://127.0.0.1:8087 — остановить: Ctrl+C"
(sleep 2; xdg-open http://127.0.0.1:8087 >/dev/null 2>&1) &
exec bin/roadpolice-web-linux-amd64
