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
bank/       bank.json + images/ — в git не входит
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

## Разработка

Банк берётся из собранного десктопного проекта:

```sh
mkdir -p bank && cp -r ../roadpolice-trainer/assets/{bank.json,images} bank/
```

```sh
cd backend && go test ./... && go run .   # :8080, база в ./data
cd frontend && npm install && npm run dev  # проксирует /api и /img на :8080
```

## Запуск на snnas

Каталог `/srv/data/appdata/roadpolice-web`: его бэкапит backrest (план `appdata`).

```sh
git clone https://github.com/Grenfis/roadpolice-web.git /srv/data/appdata/roadpolice-web
cd /srv/data/appdata/roadpolice-web
# банк — с машины, где собран roadpolice-trainer:
#   rsync -a assets/bank.json assets/images snnas:/srv/data/appdata/roadpolice-web/bank/
mkdir -p data   # контейнер работает от uid 1000 (snippy), см. docker-compose.yml
docker compose up -d --build
tailscale serve --bg --https=10443 8087
```

Адрес: `https://snnas.tailf00d9e.ts.net:10443`. Контейнер слушает только
`127.0.0.1:8087`, в локальную сеть порт не открыт.

Обновление: `git pull && docker compose up -d --build`.

Обновился банк (roadpolice.am перевыкладывает билеты): заменить файлы в
`bank/` и выполнить `docker compose restart`. Если из банка пропал вопрос,
который стоит в незавершённом экзамене, этот экзамен не откроется. Его нужно
бросить.
