from __future__ import annotations

import re
from pathlib import Path

SOURCE = Path(
    r"D:\OneDrive\Bases\Разработка\000 - RU ГСН-2022\2026-06-15 Сокращенная база"
)
HIERARCHY = SOURCE / "Books" / "00~hierarchy.txt"
PDF_DIR = SOURCE / "PDF"

PDF_RE = re.compile(r"[\w~.-]+\.PDF", re.IGNORECASE)


def split_record(line: str) -> list[str]:
    body = line.rstrip("\r\n")
    if body.endswith("*"):
        body = body[:-1]
    return body.split("'")


def main() -> None:
    lines = HIERARCHY.read_text(encoding="cp1251").splitlines()
    refs: list[tuple[int, str, str, int]] = []
    for line_no, line in enumerate(lines, start=1):
        match = PDF_RE.search(line)
        if not match:
            continue
        fields = split_record(line)
        pdf_field = -1
        for idx, field in enumerate(fields):
            if PDF_RE.fullmatch(field.strip()):
                pdf_field = idx
                break
        refs.append((line_no, fields[0], match.group(0).upper(), pdf_field))

    pdf_files = {p.name.upper(): p for p in PDF_DIR.iterdir() if p.is_file()}
    referenced = sorted({r[2] for r in refs})
    matched = [name for name in referenced if name in pdf_files]
    missing = [name for name in referenced if name not in pdf_files]
    unused = sorted(name for name in pdf_files if name not in set(referenced))

    print(f"hierarchy_pdf_nodes={len(refs)}")
    print(f"unique_referenced={len(referenced)}")
    print(f"matched_on_disk={len(matched)}")
    print(f"missing_on_disk={len(missing)}")
    print(f"unused_on_disk={len(unused)}")
    if missing[:10]:
        print("missing sample:", missing[:10])
    if unused[:10]:
        print("unused sample:", unused[:10])


if __name__ == "__main__":
    main()
