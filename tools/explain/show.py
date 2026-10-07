#!/usr/bin/env python3
"""Фрагменты пункта на обоих языках: show.py UNIT 'рус. начало' 'арм. начало' [длина]."""
import json, re, sys
from pathlib import Path
u = {x["id"]: x for x in json.loads((Path(__file__).resolve().parents[2] / "bank" / "rules.json").read_text())["units"]}
uid, ru_kw, hy_kw = sys.argv[1:4]
n = int(sys.argv[4]) if len(sys.argv) > 4 else 400
for lang, kw in (("ru", ru_kw), ("hy", hy_kw)):
    t = u[uid][lang]
    m = re.search(kw, t)
    print(f"[{lang}]", t[m.start():m.start() + n].replace("\n", "⏎") if m else "НЕТ")
print()
