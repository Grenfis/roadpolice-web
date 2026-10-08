#!/usr/bin/env python3
"""Проверяет черновики пояснений и собирает bank/explanations.json.

Черновики — bank/explain-drafts/*.json, массив записей:

  {"qid": "abc-ru-g1-q1",
   "no_basis": false,                      # true — только для первой помощи
   "answer":  {"text": "...", "refs": [{"unit": "pdd:47", "hy": "...", "ru": "..."}]},
   "options": {"2": {"text": "...", "refs": [...]}, ...}}   # ключ — номер неверного варианта

Проверка кодом, а не на честном слове модели:
  * пункт unit существует в bank/rules.json;
  * hy — дословный фрагмент армянского текста пункта (источник истины);
  * ru — дословный фрагмент перевода drv.am этого пункта; если нужного места
    в переводе нет, вместо ru пишется ru_model, и в приложении он помечается
    как перевод модели;
  * у каждой части есть хотя бы одна ссылка, кроме no_basis.
Часть, не прошедшая проверку, в итог не попадает — лучше без пояснения,
чем с выдуманной ссылкой. Отчёт печатается в stderr.

Каждой части присваивается rev — хеш её содержимого: пометка «некорректно»
привязывается к rev и не переезжает на исправленный текст.
"""
import hashlib
import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
BANK = ROOT / "bank"


def norm(s: str) -> str:
    s = s.replace("ё", "е").replace("Ё", "Е")
    s = re.sub(r"[`՝’'‘]", "'", s)
    s = re.sub(r"[«»“”„\"]", '"', s)
    s = re.sub(r"[–—−]", "-", s)
    return re.sub(r"\s+", " ", s).strip().lower()


def rev_of(part) -> str:
    return hashlib.sha1(json.dumps(part, ensure_ascii=False, sort_keys=True).encode()).hexdigest()[:10]


def check_part(part, units, no_basis, where, errors):
    text = (part.get("text") or "").strip()
    if not text:
        errors.append(f"{where}: пустой текст")
        return None
    # армянская буква внутри русского слова — опечатка при наборе
    # (армянские названия на знаках, например «Թալին», допустимы)
    if re.search(r"[А-Яа-яЁё][԰-֏]|[԰-֏][А-Яа-яЁё]", text):
        errors.append(f"{where}: армянская буква в русском слове: {text[:60]!r}")
    refs_out = []
    for r in part.get("refs", []):
        u = units.get(r.get("unit"))
        if not u:
            errors.append(f"{where}: нет пункта {r.get('unit')!r}")
            continue
        hy = (r.get("hy") or "").strip()
        if len(hy) < 10 or norm(hy) not in norm(u["hy"]):
            errors.append(f"{where}: армянская цитата не найдена в {u['id']}: {hy[:60]!r}")
            continue
        ref = {"unit": u["id"], "label": u["label"], "hy": hy}
        # русский текст: дословный фрагмент перевода drv.am, а если нужного
        # места в переводе нет — перевод модели с пометкой
        ru, ru_model = (r.get("ru") or "").strip(), (r.get("ru_model") or "").strip()
        if ru and u["ru"] and len(ru) >= 10 and norm(ru) in norm(u["ru"]):
            ref["ru"], ref["ru_source"] = ru, "drv.am"
        elif ru_model:
            ref["ru"], ref["ru_source"] = ru_model, "model"
        else:
            errors.append(f"{where}: русская цитата не найдена в переводе {u['id']} и нет ru_model: {ru[:60]!r}")
            continue
        refs_out.append(ref)
    if not refs_out and not no_basis:
        errors.append(f"{where}: ни одной подтверждённой ссылки — часть отброшена")
        return None
    out = {"text": text, "refs": refs_out}
    out["rev"] = rev_of(out)
    return out


def main():
    bank = {q["id"]: q for q in json.loads((BANK / "bank.json").read_text())["questions"]}
    units = {u["id"]: u for u in json.loads((BANK / "rules.json").read_text())["units"]}
    out_path = BANK / "explanations.json"
    items = {}  # собираем с нуля: черновики — единственный источник

    errors, merged = [], 0
    for f in sorted((BANK / "explain-drafts").glob("*.json")):
        for d in json.loads(f.read_text()):
            qid = d["qid"]
            q = bank.get(qid)
            if not q:
                errors.append(f"{f.name}: нет вопроса {qid}")
                continue
            no_basis = bool(d.get("no_basis"))
            if no_basis and q["section"] != "first_aid":
                errors.append(f"{qid}: no_basis разрешён только для первой помощи")
                no_basis = False
            item = {"no_basis": no_basis, "options": {}}
            if "answer" in d:
                a = check_part(d["answer"], units, no_basis, f"{qid} ответ", errors)
                if a:
                    item["answer"] = a
            for n, part in (d.get("options") or {}).items():
                if not n.isdigit() or not (1 <= int(n) <= len(q["options"])) or int(n) == q["answer"]:
                    errors.append(f"{qid}: вариант {n} — не неверный вариант этого вопроса")
                    continue
                p = check_part(part, units, no_basis, f"{qid} вариант {n}", errors)
                if p:
                    item["options"][n] = p
            if "answer" in item or item["options"]:
                items[qid] = item
                merged += 1

    out_path.write_text(json.dumps({"version": 1, "items": items}, ensure_ascii=False, indent=1))
    print(f"вопросов с пояснениями: {len(items)} (из черновиков принято {merged})", file=sys.stderr)
    for e in errors:
        print("  ✗", e, file=sys.stderr)


if __name__ == "__main__":
    main()
