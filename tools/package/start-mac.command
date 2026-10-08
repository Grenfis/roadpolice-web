#!/bin/sh
# Двойной щелчок в Finder запускает тренажёр и открывает его в браузере.
cd "$(dirname "$0")"
case "$(uname -m)" in arm64) BIN=bin/roadpolice-web-darwin-arm64 ;; *) BIN=bin/roadpolice-web-darwin-amd64 ;; esac
export RP_ADDR=127.0.0.1:8087 RP_BANK_DIR=bank RP_DATA_DIR=data RP_FRONTEND_DIR=public
echo "Тренажёр ПДД: http://127.0.0.1:8087 — чтобы остановить, закройте окно или нажмите Ctrl+C"
(sleep 2; open http://127.0.0.1:8087) &
exec "$BIN"
