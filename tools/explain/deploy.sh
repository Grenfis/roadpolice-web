#!/bin/sh
# Выкладывает пояснения на snnas: проверка черновиков, копирование
# explanations.json (и черновиков — для бэкапа) и перезапуск контейнера.
set -e
cd "$(dirname "$0")/../.."
python3 tools/explain/merge.py
tar cf - -C bank explanations.json explain-drafts rules.json |
  ssh -o BatchMode=yes snnas 'tar xf - -C /srv/data/appdata/roadpolice-web/bank &&
    cd /srv/data/appdata/roadpolice-web && docker compose restart >/dev/null 2>&1 && sleep 3 &&
    docker logs --since 10s roadpolice-web 2>&1 | grep слушаю'
