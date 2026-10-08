#!/bin/sh
# Собирает архив для запуска тренажёра на своём компьютере без Docker,
# Go и Node: готовые программы под Windows, macOS и Linux, собранный фронт
# и банк вопросов с пояснениями.
#
#   tools/package/package.sh            # → dist/roadpolice-web-local.zip
#
# Нужны Go, Node (npm) и python3; данные берутся из bank/ (bank.json,
# images/, explanations.json — см. README). Прогресс (data/) в архив не
# попадает: у получателя база создаётся пустой.
set -eu
cd "$(dirname "$0")/../.."
ROOT=$(pwd)
NAME=roadpolice-web
OUT="$ROOT/dist"
STAGE="$OUT/$NAME"

for f in bank/bank.json bank/explanations.json; do
	[ -f "$f" ] || { echo "нет $f — сначала соберите банк и пояснения (README)" >&2; exit 1; }
done
[ -d bank/images ] || { echo "нет bank/images/" >&2; exit 1; }

rm -rf "$STAGE" "$OUT/$NAME-local.zip"
mkdir -p "$STAGE/bin" "$STAGE/bank"

echo "фронт…"
(cd frontend && npm ci --silent && npm run build --silent >/dev/null)
cp -r frontend/dist "$STAGE/public"

echo "программы…"
for target in windows/amd64 darwin/arm64 darwin/amd64 linux/amd64; do
	os=${target%/*}; arch=${target#*/}
	ext=; [ "$os" = windows ] && ext=.exe
	(cd backend && GOOS=$os GOARCH=$arch CGO_ENABLED=0 \
		go build -trimpath -ldflags='-s -w' -o "$STAGE/bin/$NAME-$os-$arch$ext" .)
done

echo "банк…"
cp bank/bank.json bank/explanations.json "$STAGE/bank/"
cp -r bank/images "$STAGE/bank/images"
cp tools/package/README.txt "$STAGE/"
cp tools/package/start-windows.bat tools/package/start-mac.command tools/package/start-linux.sh "$STAGE/"

echo "архив…"
python3 - "$OUT" "$NAME" <<'EOF'
import os, sys, zipfile
out, name = sys.argv[1], sys.argv[2]
exe = (".command", ".sh")
with zipfile.ZipFile(os.path.join(out, f"{name}-local.zip"), "w", zipfile.ZIP_DEFLATED) as z:
    for dirpath, _, files in os.walk(os.path.join(out, name)):
        for f in sorted(files):
            path = os.path.join(dirpath, f)
            arc = os.path.relpath(path, out)
            info = zipfile.ZipInfo.from_file(path, arc)
            # права на запуск для macOS и Linux
            runnable = f.endswith(exe) or (os.path.basename(dirpath) == "bin" and not f.endswith(".exe"))
            info.external_attr = ((0o755 if runnable else 0o644) | 0o100000) << 16
            info.compress_type = zipfile.ZIP_DEFLATED
            with open(path, "rb") as src:
                z.writestr(info, src.read())
EOF
rm -rf "$STAGE"
ls -lh "$OUT/$NAME-local.zip"
