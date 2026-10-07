#!/usr/bin/env python3
"""Порция вопросов для пояснений: вопрос, варианты, верный ответ, картинка и
пункты-кандидаты из bank/rules.json.

  batch.py next [N]        следующие N вопросов без пояснений (по умолчанию 10)
  batch.py ids ID...       конкретные вопросы
  batch.py unit ID...      полный текст пунктов (hy и ru) — чтобы цитировать дословно
  batch.py find REGEX      пункты, в русском тексте которых есть REGEX (без учёта регистра)

Кандидаты — грубый поиск по пересечению основ слов с русским текстом пункта,
в пределах раздела вопроса. Это подсказка, а не ограничение: сослаться можно
на любой пункт базы.
"""
import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
BANK = ROOT / "bank"

SECTION_UNITS = {
    "law": ("law",),
    "pdd": ("pdd", "sign", "marking"),
    "faults": ("faults",),
    "first_aid": (),
}


def stems(text: str) -> set[str]:
    words = re.findall(r"[а-яё]{4,}", text.lower().replace("ё", "е"))
    return {w[:5] for w in words}


def load():
    bank = json.loads((BANK / "bank.json").read_text())
    units = json.loads((BANK / "rules.json").read_text())["units"]
    done = set()
    p = BANK / "explanations.json"
    if p.exists():
        done = set(json.loads(p.read_text())["items"])
    # вопросы, отложенные без пояснения (не удалось уверенно разобрать), — по строке «id  причина»
    skip = BANK / "explain-skip.txt"
    if skip.exists():
        done |= {l.split()[0] for l in skip.read_text().splitlines() if l.strip()}
    drafts = BANK / "explain-drafts"
    if drafts.exists():
        for f in drafts.glob("*.json"):
            done |= {d["qid"] for d in json.loads(f.read_text())}
    return bank, units, done


def candidates(q, units, k=5):
    kinds = SECTION_UNITS[q["section"]]
    if not kinds:
        return []
    qs = stems(q["text"] + " " + " ".join(q["options"]))
    qs |= stems(q["options"][q["answer"] - 1])  # верный ответ весит больше
    # номера знаков/разметки, упомянутые в тексте, — прямые кандидаты
    nums = set(re.findall(r"\b(\d\.\d{1,2}(?:\.\d{1,2})?)\b", q["text"] + " ".join(q["options"])))
    scored = []
    for u in units:
        kind, num = u["id"].split(":", 1)
        if kind not in kinds:
            continue
        score = len(qs & stems(u["ru"] or ""))
        if num in nums:
            score += 50
        scored.append((score, u))
    scored.sort(key=lambda x: -x[0])
    return [u for s, u in scored[:k] if s > 0]


def show_question(q, units):
    print(f"### {q['id']}  (группа {q['group']}, раздел {q['section']})")
    print(q["text"])
    if q["image"]:
        print(f"[картинка] {BANK / 'images' / q['image']}")
    for i, o in enumerate(q["options"], 1):
        mark = "  ← верно" if i == q["answer"] else ""
        print(f"  {i}) {o}{mark}")
    c = candidates(q, units)
    if c:
        print("кандидаты:")
        for u in c:
            ru = re.sub(r"\s+", " ", u["ru"])[:160]
            print(f"  - {u['id']} [{u['label']}] {ru}")
    print()


def main():
    bank, units, done = load()
    qs = {q["id"]: q for q in bank["questions"]}
    cmd = sys.argv[1] if len(sys.argv) > 1 else "next"
    if cmd == "next":
        n = int(sys.argv[2]) if len(sys.argv) > 2 else 10
        todo = [q for q in bank["questions"] if q["id"] not in done]
        print(f"# осталось без пояснений: {len(todo)}\n")
        for q in todo[:n]:
            show_question(q, units)
    elif cmd == "ids":
        for i in sys.argv[2:]:
            show_question(qs[i], units)
    elif cmd == "unit":
        by = {u["id"]: u for u in units}
        for i in sys.argv[2:]:
            u = by.get(i)
            if not u:
                print(f"### {i}: нет такого пункта\n")
                continue
            print(f"### {u['id']} [{u['label']}]\nhy: {u['hy']}\nru: {u['ru']}\n")
    elif cmd == "find":
        rx = re.compile(sys.argv[2], re.I)
        for u in units:
            for m in rx.finditer(u["ru"]):
                a, b = max(0, m.start() - 120), min(len(u["ru"]), m.end() + 120)
                print(f"{u['id']:14} …{re.sub(chr(92) + 's+', ' ', u['ru'][a:b])}…")
                break
    else:
        sys.exit(__doc__)


if __name__ == "__main__":
    main()
