# Тренажёр теории ПДД Армении — веб-версия

Веб-версия [roadpolice-trainer](https://github.com/Grenfis/roadpolice-trainer):
тот же банк вопросов roadpolice.am (категории A, B, C, русский) и те же режимы,
но работает на домашнем сервере, а заходить можно с телефона, планшета и
компьютера. Прогресс общий для всех устройств.

Пользователь один, входа нет: доступ ограничивает Tailscale.

## Устройство

```
backend/    Go, стандартный net/http
  internal/bank/     банк вопросов и сборка билета (из roadpolice-trainer)
  internal/store/    SQLite: ответы, ошибки, экзамены, незавершённый экзамен
  internal/trainer/  логика тренажёра
  internal/api/      JSON-маршруты /api/*
frontend/   Vue 3 + TypeScript + Vite
bank/       данные, в git не входят (см. «Данные»)
tools/explain/  сборка базы правил и проверка пояснений
tools/package/  архив для запуска на своём компьютере без Docker
data/       trainer.db — в git не входит
```

Собранный фронт отдаёт сам Go, в проде это один контейнер.

## Что отличается от десктопа

**Экзамен хранится на сервере.** Каждый выбранный ответ сразу сохраняется,
таймер считает сервер. Поэтому экзамен переживает перезагрузку страницы и
продолжается на другом устройстве.

- Таймер стоит, только пока страница закрыта: при закрытии или перезагрузке
  браузер шлёт `sendBeacon`. Если вкладка свёрнута или погас экран, время идёт.
  Если браузер убил вкладку без `pagehide` (бывает на телефонах), время тоже идёт.
- Ведёт экзамен то устройство, которое последним его открыло. Остальные при
  следующем запросе или опросе (раз в 10 секунд) видят «Экзамен продолжен на
  другом устройстве» и могут забрать его обратно.
- «Выйти» бросает экзамен: он не попадёт ни в историю, ни в список ошибок.
- Если при старте нового есть незавершённый, приложение спросит: продолжить
  его или бросить и начать новый.
- Если время вышло, пока страница была закрыта, экзамен засчитывается
  при следующем открытии.

**Тема у каждого браузера своя** (хранится в `localStorage`), а позиция
в тренировке общая.

## Пояснения

После ответа в тренировке (при верном ответе — по кнопке «Пояснение») и в
разборе экзамена показывается, почему верен правильный ответ и почему не
подходит выбранный вариант; разбор остальных вариантов — по кнопке.

Пояснение опирается на конкретный пункт правил, и это проверяет код, а не
модель:

1. `tools/explain/corpus.py` режет официальные армянские тексты с arlis.am
   (ПДД и перечень неисправностей — акт 212587, закон о БДД — акт 230020)
   на пункты и сопоставляет каждому русский текст из неофициального перевода
   drv.am → `bank/rules.json`.
2. Черновики пояснений (`bank/explain-drafts/*.json`) пишет модель: текст,
   номер пункта, дословная армянская цитата и русский фрагмент.
3. `tools/explain/merge.py` принимает часть пояснения, только если армянская
   цитата дословно есть в указанном пункте, а русская — в переводе drv.am
   (если нужного места в переводе нет — перевод модели с пометкой в
   приложении). Не прошло — части нет. Итог — `bank/explanations.json`.

Армянский текст на устройство не отдаётся. Первая помощь поясняется без
цитат и с пометкой: нормативного текста с пунктами для неё нет.

Каждую часть пояснения можно пометить «некорректно» с необязательным
комментарием и снять пометку. Пометка привязана к версии текста (`rev`):
после исправления пояснения она к новому тексту не переходит. Все пометки —
`GET /api/flags`.

```sh
tools/explain/corpus.py        # bank/rules-src/*.html → bank/rules.json
tools/explain/batch.py next 10 # следующая порция вопросов с пунктами-кандидатами
tools/explain/merge.py         # проверка черновиков → bank/explanations.json
```

## API

| Метод | Путь | |
|---|---|---|
| GET | `/api/state` | главный экран, включая `active_exam` |
| POST | `/api/exams` | новый экзамен (409 `exam_active`, если есть незавершённый) |
| POST | `/api/exams/{id}/resume` | открыть на этом устройстве: новый `lease`, снять паузу |
| PUT | `/api/exams/{id}/answers` | `{lease, question_id, chosen}` |
| GET | `/api/exams/{id}/status?lease=` | сколько осталось; 409 `lease_lost` |
| POST | `/api/exams/{id}/pause` | `{lease}`, шлётся через sendBeacon |
| POST | `/api/exams/{id}/finish` | `{lease}` |
| DELETE | `/api/exams/{id}` | бросить |
| POST | `/api/training` | выборка вопросов |
| PUT | `/api/training/position` | `{key, index}` |
| POST | `/api/answers` | ответ в тренировке: `{question_id, chosen, mode}` |
| GET | `/api/mistakes`, `/api/stats` | |
| GET | `/api/questions/{id}/explanation` | пояснение или `null`; 409 `exam_question`, пока вопрос в незавершённом экзамене |
| PUT / DELETE | `/api/questions/{id}/explanation/{part}/flag` | пометка «некорректно»: `{comment}`; part — `answer` или номер варианта |
| GET | `/api/flags` | все пометки с вопросом и помеченным текстом |

## Данные

Код лежит в git, данные — нет: банк вопросов собирается из чужих PDF, пояснения
пишутся в сессиях с моделью. Всё, что нужно приложению, лежит в `bank/`:

| Файл | Нужен для | Откуда |
|---|---|---|
| `bank.json`, `images/` | работы (обязательно) | собирает [roadpolice-trainer](https://github.com/Grenfis/roadpolice-trainer) из PDF roadpolice.am |
| `explanations.json` | пояснений (без него их 0) | `tools/explain/merge.py` из черновиков |
| `explain-drafts/`, `explain-skip.txt` | пересборки и дописывания пояснений | пишет модель |
| `rules-src/*.html` | пересборки `rules.json` | страницы, сохранённые 2026-10-07 (см. ниже) |
| `rules.json` | проверки пояснений | `tools/explain/corpus.py` из `rules-src/` |

Полная копия `bank/` лежит на snnas в `/srv/data/appdata/roadpolice-web/bank`
и бэкапится вместе с каталогом. Новая установка начинается с пустым прогрессом:
`data/trainer.db` переносится отдельно (копией или из бэкапа).

Исходники правил (`bank/rules-src/`):

| Файл | Страница |
|---|---|
| `arlis-212587.html` | https://www.arlis.am/hy/acts/212587 — ПДД, знаки, разметка, неисправности (официальный армянский текст) |
| `arlis-230020.html` | https://www.arlis.am/hy/acts/230020 — закон о безопасности дорожного движения |
| `drv-traffic-rules-of-armenia.html` | https://drv.am/traffic-rules-of-armenia/ru |
| `drv-road-signs.html` | https://drv.am/road-signs/ru |
| `drv-road-safety.html` | https://drv.am/road-safety/ru |
| `drv-vehicle-malfunctions.html` | https://drv.am/vehicle-malfunctions/ru |

Если тексты на сайтах поменяются, `corpus.py` даст другой `rules.json`, и
часть цитат в черновиках может перестать проходить проверку — поэтому
сохранённые страницы держим, а не скачиваем заново.

## Развёртывание с нуля

Проверено 2026-10-08 на чистых клонах. Нужно: на машине сборки — git, Python 3,
`cwebp` (пакет `webp`), curl; на сервере — Docker с compose и Tailscale.

**1. Банк вопросов** (≈30 секунд):

```sh
git clone https://github.com/Grenfis/roadpolice-trainer.git && cd roadpolice-trainer
mkdir -p tools/dataset/pdf
for i in $(seq 1 10); do
  curl -sf -o tools/dataset/pdf/abc_ru_$i.pdf https://roadpolice.am/exam/abc/ru/$i.pdf
done
tools/dataset/build.sh      # → assets/bank.json (1050 вопросов) и assets/images/ (699)
```

**2. Код на сервер:**

```sh
git clone https://github.com/Grenfis/roadpolice-web.git /srv/data/appdata/roadpolice-web
cd /srv/data/appdata/roadpolice-web && mkdir -p bank data
```

**3. Данные в `bank/`** — с машины сборки. На snnas нет rsync, поэтому tar
через ssh:

```sh
tar cf - -C roadpolice-trainer/assets bank.json images |
  ssh snnas 'tar xf - -C /srv/data/appdata/roadpolice-web/bank'
tar cf - -C roadpolice-web/bank explanations.json rules.json rules-src explain-drafts explain-skip.txt |
  ssh snnas 'tar xf - -C /srv/data/appdata/roadpolice-web/bank'
```

Если `bank/` восстанавливается из бэкапа snnas целиком — шаги 1 и 3 не нужны.

**4. Прогресс** (если переезд, а не новая установка): положить старый
`data/trainer.db` до запуска.

**5. Запуск.** Контейнер работает от uid 1000 (snippy), см. `docker-compose.yml`:

```sh
docker compose up -d --build
tailscale serve --bg --https=10443 8087
```

**6. Проверка:**

```sh
curl -s http://127.0.0.1:8087/api/health            # {"status":"ok"}
docker logs roadpolice-web 2>&1 | grep слушаю        # банк: 1050 вопросов, пояснений: 985
```

Адрес: `https://snnas.tailf00d9e.ts.net:10443`. Контейнер слушает только
`127.0.0.1:8087`, в локальную сеть порт не открыт.

## Обновление

- Код: `git pull && docker compose up -d --build`.
- Пояснения: `tools/explain/deploy.sh` с машины, где черновики (проверяет их,
  копирует `explanations.json`, `rules.json`, `explain-drafts/` на snnas и
  перезапускает контейнер). Список отложенных вопросов копируется отдельно:
  `scp bank/explain-skip.txt snnas:/srv/data/appdata/roadpolice-web/bank/`.
- Банк (roadpolice.am перевыкладывает билеты). Проверить, изменились ли PDF —
  сравнить размеры с локальными:
  `curl -sI https://roadpolice.am/exam/abc/ru/4.pdf | grep -i -e content-length -e last-modified`.
  Если да — положить новые PDF в `roadpolice-trainer/tools/dataset/pdf/`,
  выполнить `tools/dataset/build.sh` (пересжимаются только изменённые картинки),
  скопировать `bank.json` и `images/` в `bank/` и сделать `docker compose restart`.
  Если из банка пропал вопрос, который стоит в незавершённом экзамене, этот
  экзамен не откроется — его нужно бросить.

## Разработка

```sh
cd backend && go test ./... && go run .   # :8080, банк в ./bank, база в ./data
cd frontend && npm install && npm run dev  # проксирует /api и /img на :8080
```

Для `go run` нужен `backend/bank` (или `RP_BANK_DIR=../bank`). Docker в WSL
не настроен — образ собирается на snnas.

## Архив для запуска на своём компьютере

Чтобы отдать тренажёр кому-то без сервера, Docker, Go и Node:

```sh
tools/package/package.sh     # → dist/roadpolice-web-local.zip (≈45 МБ)
```

В архиве готовые программы под Windows (x64), macOS (Apple Silicon и Intel)
и Linux (x64), собранный фронт, `bank.json`, картинки и `explanations.json`,
скрипты запуска и инструкция для получателя (`tools/package/README.txt`).
Приложение слушает только `127.0.0.1:8087` и само открывает браузер,
прогресс хранится в `data/` рядом со скриптами. Собирать после обновления
банка или пояснений — архив берёт их из `bank/`.
