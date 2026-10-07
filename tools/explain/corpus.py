#!/usr/bin/env python3
"""Собирает bank/rules.json — нормативную базу для пояснений.

Источник истины — официальный армянский текст с arlis.am: по нему проверяются
цитаты. Русский текст пункта берётся из неофициального перевода drv.am
(сопоставление по номеру пункта); где перевода нет, поле ru пустое.

Единица базы — пункт, на который можно сослаться:
  pdd:N        пункт правил (приложение 1 к 955-Н)
  sign:X.Y     дорожный знак (форма 1)
  marking:X.Y  дорожная разметка (форма 2)
  faults:N     пункт перечня неисправностей (приложение 2)
  law:N        статья закона о БДД

Запуск: tools/explain/corpus.py  (читает bank/rules-src/, пишет bank/rules.json)
"""
import json
import re
import sys
from html.parser import HTMLParser
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SRC = ROOT / "bank" / "rules-src"
OUT = ROOT / "bank" / "rules.json"


class _Text(HTMLParser):
    BLOCK = {"p", "br", "div", "tr", "li", "td", "h1", "h2", "h3", "h4", "h5", "ol", "ul"}

    def __init__(self):
        super().__init__()
        self.out, self.skip = [], 0

    def handle_starttag(self, tag, attrs):
        if tag in ("script", "style", "nav", "aside"):
            self.skip += 1
        if tag in self.BLOCK:
            self.out.append("\n")

    def handle_endtag(self, tag):
        if tag in ("script", "style", "nav", "aside"):
            self.skip -= 1
        if tag in self.BLOCK:
            self.out.append("\n")

    def handle_data(self, data):
        if not self.skip:
            self.out.append(data)


def text_of(path: Path, main_only=False) -> list[str]:
    h = path.read_text(encoding="utf-8")
    if main_only:
        m = re.search(r"<main.*?</main>", h, flags=re.S)
        h = m.group(0) if m else h
    p = _Text()
    p.feed(h)
    t = re.sub(r"[ \t\xa0]+", " ", "".join(p.out))
    return [l.strip() for l in t.split("\n") if l.strip()]


# строки-справки о поправках: «(пункт 12 изм. 20.11.08 N 1373-Н)», «(1-ին կետը լրաց. ... N 1373-Ն)»
AMEND = re.compile(r"^\(.*\d{2}\.\d{2}\.\d{2}.*N\s*\d+-[ՆН]\)?$")


def is_amend(line: str) -> bool:
    return bool(AMEND.match(line))


def split_points(lines, start, stop, num_re):
    """Режет строки [start, stop) на пункты по регулярке номера в начале строки."""
    out, cur = {}, None
    for l in lines[start:stop]:
        if is_amend(l):
            continue
        m = num_re.match(l)
        if m:
            cur = m.group(1)
            out.setdefault(cur, [])
            out[cur].append(l)
        elif cur is not None:
            out[cur].append(l)
    return {k: "\n".join(v) for k, v in out.items()}


def index_of(lines, pred, start=0):
    for i in range(start, len(lines)):
        if pred(lines[i]):
            return i
    raise ValueError("не найдено")


ROMAN = re.compile(r"^[IVXL]+\.\s")


def drop_headers(points):
    # заголовки глав (римские) склеились с хвостом предыдущего пункта — срезаем
    return {k: re.split(r"\n[IVXL]+\.\s[^\n]*$", v)[0] for k, v in points.items()}


def expand(label: str) -> list[str]:
    """«4.2.1, 4.2.2» → [4.2.1, 4.2.2]; «1.4.1-1.4.6» → [1.4.1 … 1.4.6]."""
    out = []
    for part in re.split(r"\s*(?:,|և)\s*", label):
        m = re.match(r"^([\d.]+?)(\d+)\s*-\s*(?:[\d.]+?)?(\d+)$", part)
        if m:
            out += [f"{m.group(1)}{i}" for i in range(int(m.group(2)), int(m.group(3)) + 1)]
        elif part:
            out.append(part)
    return out


def main():
    units = []

    # ---------- ПДД и знаки: arlis 212587 ----------
    hy = text_of(SRC / "arlis-212587.html")
    i_rules = index_of(hy, lambda l: l.startswith("I. ԸՆԴՀԱՆՈՒՐ ԴՐՈՒՅԹՆԵՐ"))
    i_form1 = index_of(hy, lambda l: l == "Ձև N 1", i_rules)
    i_marks = index_of(hy, lambda l: l.startswith("I. ՀՈՐԻԶՈՆԱԿԱՆ ԳԾԱՆՇՈՒՄ"), i_form1)
    i_annex2 = index_of(hy, lambda l: l == "Հավելված N 2", i_marks)
    i_annex2_end = index_of(hy, lambda l: l.startswith("Նկար 1"), i_annex2)

    pdd_hy = drop_headers(split_points(hy, i_rules, i_form1, re.compile(r"^(\d{1,3}(?:\.\d)?)\.\s")))
    signs_hy = split_points(hy, i_form1, i_marks, re.compile(r"^(\d\.\d{1,2}(?:\.\d{1,2})?)\.?[\s«,՝`-]"))
    marks_hy = split_points(hy, i_marks, i_annex2, re.compile(r"^(\d\.\d{1,2}(?:\.\d{1,2})?)(?:[`՝,]|\s-)"))
    faults_hy = split_points(hy, i_annex2, i_annex2_end, re.compile(r"^(\d{1,2})\.\s"))

    # ---------- закон: arlis 230020 ----------
    law = text_of(SRC / "arlis-230020.html")
    i_law = index_of(law, lambda l: re.match(r"^Հոդված\s*1\.", l) is not None)
    law_hy = split_points(law, i_law, len(law), re.compile(r"^Հոդված\s*(\d{1,2}(?:\.\d)?)\."))
    # хвост последней статьи — служебный текст сайта; обрезаем по подписи
    for k, v in law_hy.items():
        law_hy[k] = re.split(r"\nՀայաստանի Հանրապետության\nՆախագահ", v)[0]

    # ---------- русские переводы drv.am ----------
    ru_rules = text_of(SRC / "drv-traffic-rules-of-armenia.html", main_only=True)
    i_ru = index_of(ru_rules, lambda l: l.startswith("1. Правилами дорожного движения"))
    pdd_ru = drop_headers(split_points(ru_rules, i_ru, len(ru_rules), re.compile(r"^(\d{1,3}(?:\.\d)?)\.\s")))

    ru_signs = text_of(SRC / "drv-road-signs.html", main_only=True)
    i_sg = index_of(ru_signs, lambda l: l == "1. Предупреждающие знаки")  # до него — оглавление
    i_mk = index_of(ru_signs, lambda l: l == "I. Горизонтальная разметка", i_sg)
    num = re.compile(r"^(\d\.\d{1,2}(?:\.\d{1,2})?)(?:[\s«.-]|$)")
    signs_ru = split_points(ru_signs, i_sg, i_mk, num)
    marks_ru = split_points(ru_signs, i_mk, len(ru_signs), num)

    ru_law = text_of(SRC / "drv-road-safety.html", main_only=True)
    # первые «Статья N.» — оглавление; берём последнее вхождение «Статья 1.»
    i_rl = max(i for i, l in enumerate(ru_law) if l.startswith("Статья 1."))
    law_ru = split_points(ru_law, i_rl, len(ru_law), re.compile(r"^Статья\s*(\d{1,2}(?:\.\d)?)\."))

    ru_f = text_of(SRC / "drv-vehicle-malfunctions.html", main_only=True)
    i_rf = index_of(ru_f, lambda l: l.startswith("1. "))
    faults_ru = split_points(ru_f, i_rf, len(ru_f), re.compile(r"^(\d{1,2})\.\s"))

    def add(kind, label_fmt, hy_map, ru_map):
        for k, v in hy_map.items():
            if len(v) < 120 and "ուժը կորցրել է" in v:
                continue  # пункт утратил силу — ссылаться не на что
            label = k
            if kind in ("sign", "marking"):
                # «4.2.1` «…», 4.2.2` «…»» или «1.4.1-1.4.6»: в подписи все номера записи
                first = v.split("\n", 1)[0]
                rng = re.match(r"^(\d[\d.]*\d)\s*-\s*(\d[\d.]*\d)", first)
                nums = re.findall(r"(?:^|,\s*|և\s*)(\d\.\d{1,2}(?:\.\d{1,2})?)(?=[`՝.,]?\s*[«,`՝]|\s*-)", first)
                if rng:
                    label = f"{rng.group(1)}-{rng.group(2)}"
                elif len(nums) > 1:
                    label = ", ".join(dict.fromkeys(nums))
            ru = ru_map.get(k, "")
            if kind in ("sign", "marking"):
                ru = "\n".join(t for t in (ru_map.get(n, "") for n in expand(label)) if t) or ru
            units.append({
                "id": f"{kind}:{k}",
                "label": label_fmt.format(label),
                "hy": v,
                "ru": ru,
            })

    add("pdd", "ПДД, п. {}", pdd_hy, pdd_ru)
    add("sign", "Знак {}", signs_hy, signs_ru)
    add("marking", "Разметка {}", marks_hy, marks_ru)
    add("faults", "Перечень неисправностей, п. {}", faults_hy, faults_ru)
    add("law", "Закон о БДД, ст. {}", law_hy, law_ru)

    stats = {}
    for u in units:
        kind = u["id"].split(":")[0]
        s = stats.setdefault(kind, [0, 0])
        s[0] += 1
        s[1] += bool(u["ru"])
    for kind, (n, ru) in stats.items():
        print(f"{kind:8} пунктов {n:4}, с русским переводом {ru:4}", file=sys.stderr)

    OUT.write_text(json.dumps({
        "sources": {
            "hy": ["https://www.arlis.am/hy/acts/212587", "https://www.arlis.am/hy/acts/230020"],
            "ru": "https://drv.am (неофициальный перевод)",
        },
        "units": units,
    }, ensure_ascii=False, indent=1))
    print(f"записано {OUT}", file=sys.stderr)


if __name__ == "__main__":
    main()
