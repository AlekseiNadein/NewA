from __future__ import annotations

import argparse
import csv
import datetime as dt
import re
from collections import Counter, defaultdict
from dataclasses import dataclass
from pathlib import Path


DEFAULT_SOURCE = Path(
    r"D:\OneDrive\Bases\Разработка\000 - RU ГСН-2022\2026-06-15 Сокращенная база\Books"
)
ENCODING = "cp1251"

SYSTEM_TEXT_FILES = {
    "00~hierarchy.txt",
    "00~nsi.txt",
    "00~resurs.txt",
    "00~uvr.txt",
    "00~ppr.txt",
}

NORM_CODE_RE = re.compile(r"^[ЕЦУE]\d{4,5}-\d{3}-\d{2}$")
RESOURCE_TOKEN_RE = re.compile(r"([А-ЯA-Z]?\d{1,6}(?:-\d{1,6})?)\.")
ORIGINAL_CODE_RE = re.compile(r"\s*\(Ш[^)]*\)")
RESOURCE_CATALOG_RE = re.compile(r"^00~c\d+\.txt$", re.IGNORECASE)
RESOURCE_PREFIXES = ("С", "C", "М", "M", "Т", "T")
RECORD_CODE_RE = re.compile(r"^[A-ZА-ЯЁ][A-ZА-ЯЁ0-9-]*\d", re.IGNORECASE)


@dataclass(frozen=True)
class HierarchyRecord:
    line_no: int
    raw: str
    code: str
    level: int
    norm_list: str
    name: str
    unit: str


@dataclass(frozen=True)
class NormRecord:
    file_name: str
    line_no: int
    raw: str
    code: str
    resources: tuple[str, ...]


def read_lines(path: Path) -> list[str]:
    return path.read_text(encoding=ENCODING).splitlines()


def write_cp1251(path: Path, lines: list[str]) -> None:
    path.write_text("\n".join(lines) + "\n", encoding=ENCODING)


def split_record(line: str) -> list[str]:
    body = line.rstrip("\r\n")
    if body.endswith("*"):
        body = body[:-1]
    return body.split("'")


def clean_hierarchy_line(line: str) -> str:
    fields = split_record(line)
    while len(fields) < 5:
        fields.append("")
    return "'".join(fields[:5]) + "*"


def clean_nsi_head(head: str) -> str:
    return ORIGINAL_CODE_RE.sub("", head).strip()


def clean_nsi_line(line: str) -> str:
    fields = split_record(line)
    while len(fields) < 6:
        fields.append("")
    head = clean_nsi_head(fields[0])
    cost = fields[2]
    work_composition = fields[5]
    return "'".join([head, cost, work_composition]) + "*"


def parse_hierarchy(lines: list[str]) -> list[HierarchyRecord]:
    records: list[HierarchyRecord] = []
    for line_no, line in enumerate(lines, start=1):
        if not line.strip():
            continue
        fields = split_record(line)
        if len(fields) < 5:
            continue
        try:
            level = int(fields[1] or 0)
        except ValueError:
            level = 0
        records.append(
            HierarchyRecord(
                line_no=line_no,
                raw=line,
                code=fields[0],
                level=level,
                norm_list=fields[2],
                name=fields[3],
                unit=fields[4],
            )
        )
    return records


def expand_norm_range(left: str, right: str) -> list[str]:
    left = left.strip()
    right = right.strip()
    if not NORM_CODE_RE.match(left) or not NORM_CODE_RE.match(right):
        return [left, right]

    left_prefix, left_no = left.rsplit("-", 1)
    right_prefix, right_no = right.rsplit("-", 1)
    if left_prefix != right_prefix:
        return [left, right]

    start = int(left_no)
    end = int(right_no)
    if end < start or end - start > 500:
        return [left, right]
    width = max(len(left_no), len(right_no))
    return [f"{left_prefix}-{number:0{width}d}" for number in range(start, end + 1)]


def clean_norm_ref(norm_ref: str) -> str:
    return norm_ref.strip().strip(";")


def expand_norm_list(norm_list: str) -> list[str]:
    if not norm_list:
        return []

    parts = [clean_norm_ref(part) for part in norm_list.split("/") if clean_norm_ref(part)]
    if len(parts) == 2 and all(NORM_CODE_RE.match(part) for part in parts):
        return expand_norm_range(parts[0], parts[1])
    return parts


def extract_record_code(line: str) -> str | None:
    fields = split_record(line)
    if not fields:
        return None
    code = fields[0].split(" ", 1)[0].split("(", 1)[0].strip()
    if not code or not RECORD_CODE_RE.match(code):
        return None
    return code


def extract_resource_codes(resource_list: str) -> tuple[str, ...]:
    codes: list[str] = []
    for token in resource_list.split("/"):
        token = token.strip()
        if not token:
            continue
        if "." in token:
            codes.append(token.split(".", 1)[0])
        else:
            match = RESOURCE_TOKEN_RE.match(token)
            if match:
                codes.append(match.group(1))
    return tuple(codes)


def parse_norm_file(path: Path) -> list[NormRecord]:
    records: list[NormRecord] = []
    for line_no, line in enumerate(read_lines(path), start=1):
        code = extract_record_code(line)
        if not code:
            continue
        fields = split_record(line)
        resource_list = fields[-1] if fields else ""
        records.append(
            NormRecord(
                file_name=path.name,
                line_no=line_no,
                raw=line,
                code=code,
                resources=extract_resource_codes(resource_list),
            )
        )
    return records


def discover_norm_files(source: Path, allowed_file_names: set[str] | None = None) -> list[Path]:
    files: list[Path] = []
    candidates = (
        sorted(source / file_name for file_name in allowed_file_names)
        if allowed_file_names is not None
        else sorted(source.glob("00~*.txt"))
    )
    for path in candidates:
        if not path.exists():
            continue
        if path.name in SYSTEM_TEXT_FILES:
            continue
        try:
            first_lines = read_lines(path)[:20]
        except UnicodeDecodeError:
            continue
        if any(extract_record_code(line) for line in first_lines):
            files.append(path)
    return files


def parse_resource_codes(line: str) -> set[str]:
    fields = split_record(line)
    codes: set[str] = set()
    if fields and fields[0].isdigit():
        codes.add(fields[0])
    if len(fields) > 2:
        for part in fields[2].split("#"):
            part = part.strip()
            if part:
                codes.add(part)
    return codes


def resource_variants(code: str) -> set[str]:
    variants = {code}
    if code.startswith(RESOURCE_PREFIXES) and code[1:].isdigit():
        variants.add(code[1:])
    return variants


def parse_resource_catalog_codes(line: str) -> set[str]:
    fields = split_record(line)
    codes = parse_resource_codes(line)
    if fields:
        head = fields[0].split(" ", 1)[0].split("(", 1)[0].strip()
        if head:
            codes.add(head)
            codes.update(resource_variants(head))
    if fields:
        for code in extract_resource_codes(fields[-1]):
            codes.add(code)
            codes.update(resource_variants(code))
    return codes


def discover_resource_catalog_files(source: Path) -> list[Path]:
    paths = [source / "00~resurs.txt"]
    paths.extend(sorted(path for path in source.glob("00~*.txt") if RESOURCE_CATALOG_RE.match(path.name)))
    return [path for path in paths if path.exists()]


def write_tsv(path: Path, rows: list[dict[str, object]], fieldnames: list[str]) -> None:
    with path.open("w", encoding="utf-8-sig", newline="") as file:
        writer = csv.DictWriter(file, fieldnames=fieldnames, delimiter="\t")
        writer.writeheader()
        writer.writerows(rows)


def collect_expected_prf_files(source: Path) -> tuple[list[str], list[str], list[str]]:
    prf = source / "00.prf"
    if not prf.exists():
        return [], [], []

    referenced = [line.strip() for line in read_lines(prf) if line.strip()]
    expected_txt = [Path(name).with_suffix(".txt").name for name in referenced]
    present = {path.name.lower() for path in source.iterdir() if path.is_file()}
    missing = [name for name in expected_txt if name.lower() not in present]
    mapped_present = [name for name in expected_txt if name.lower() in present]
    return referenced, mapped_present, missing


def mark_used_hierarchy(records: list[HierarchyRecord], found_norms: set[str]) -> list[dict[str, object]]:
    descendant_norm_count: Counter[str] = Counter()
    matched_norm_count: Counter[str] = Counter()
    rows: list[dict[str, object]] = []

    # A section owns following records until a record with the same or lower level
    # appears. Count descendant norm references in one pass instead of materializing
    # descendant lists for every section.
    stack: list[HierarchyRecord] = []
    for record in records:
        while stack and stack[-1].level >= record.level:
            stack.pop()

        norm_codes = expand_norm_list(record.norm_list)
        if norm_codes:
            targets = [parent for parent in stack if parent.level < 8]
            if record.level < 8:
                targets.append(record)
            matched = sum(1 for norm_code in norm_codes if norm_code in found_norms)
            for target in targets:
                descendant_norm_count[target.code] += len(norm_codes)
                matched_norm_count[target.code] += matched

        stack.append(record)

    for record in records:
        if record.level >= 8:
            continue
        descendant_norms = descendant_norm_count[record.code]
        matched_norms = matched_norm_count[record.code]
        if descendant_norms and not matched_norms:
            rows.append(
                {
                    "hierarchy_code": record.code,
                    "level": record.level,
                    "name": record.name,
                    "descendant_norms": descendant_norms,
                    "matched_norms": matched_norms,
                }
            )
    return rows


def build_report(
    source: Path,
    out: Path,
    hierarchy_records: list[HierarchyRecord],
    norm_records: list[NormRecord],
    used_norm_codes: set[str],
    found_used_norm_codes: set[str],
    resource_usage: Counter[str],
    resource_codes_present: set[str],
    prf_referenced: list[str],
    prf_mapped_present: list[str],
    prf_missing: list[str],
    norm_files: list[Path],
    unused_sections_count: int,
    proposed_unused_sections_count: int,
) -> str:
    missing_norm_count = len(used_norm_codes - found_used_norm_codes)
    missing_resource_count = sum(
        1
        for code in set(resource_usage)
        if not any(variant in resource_codes_present for variant in resource_variants(code))
    )
    unused_resource_count = len(resource_codes_present - set(resource_usage))

    lines = [
        "# Подготовка текстовой базы ГСН-2022",
        "",
        f"- Исходная папка: `{source}`",
        f"- Папка результата: `{out}`",
        f"- Кодировка очищенных файлов: `{ENCODING}`",
        f"- Дата подготовки: `{dt.datetime.now().isoformat(timespec='seconds')}`",
        "",
        "## Итоги",
        "",
        f"- Строк иерархии: {len(hierarchy_records)}",
        f"- Нормативных записей в найденных файлах: {len(norm_records)}",
        f"- Кодов норм, упомянутых в иерархии: {len(used_norm_codes)}",
        f"- Найдено упомянутых норм: {len(found_used_norm_codes)}",
        f"- Не найдено упомянутых норм: {missing_norm_count}",
        f"- Уникальных ресурсов в найденных нормах: {len(resource_usage)}",
        f"- Ресурсов из норм без строки в ресурсных справочниках: {missing_resource_count}",
        f"- Вариантов кодов ресурсов в справочниках, не использованных найденными нормами: {unused_resource_count}",
        f"- Разделов иерархии-кандидатов на удаление: {unused_sections_count}",
        f"- Разделов, предложенных на удаление без уровня 7: {proposed_unused_sections_count}",
        "",
        "## Файлы `00.prf`",
        "",
        f"- Записей в `00.prf`: {len(prf_referenced)}",
        f"- Найдено соответствующих `.txt` после замены расширения `.ufd`: {len(prf_mapped_present)}",
        f"- Не найдено соответствующих `.txt`: {len(prf_missing)}",
        "",
        "## Найденные файлы норм",
        "",
    ]
    lines.extend(f"- `{path.name}`" for path in norm_files)
    lines.extend(
        [
            "",
            "## Подготовленные файлы",
            "",
            "- `00~hierarchy.clean.txt` — иерархия без ссылки на фрагмент и оригинального шифра.",
            "- `00~nsi.clean.txt` — дополнительные характеристики без оригинального шифра, определителя, наименования и измерителя.",
            "- `00~norms.used.txt` — нормативные записи, которые реально упомянуты в иерархии и найдены в файлах норм.",
            "- `00~resurs.used.txt` — строки ресурсных справочников, которые упоминаются найденными нормами.",
            "- `missing_norms.tsv` — нормы из иерархии, не найденные в нормативных файлах.",
            "- `missing_resources.tsv` — ресурсы из норм, не найденные в кодификаторе.",
            "- `unused_hierarchy_sections.tsv` — разделы с потомками-нормами, для которых нет найденных норм.",
            "- `unused_hierarchy_sections_proposed.tsv` — предложение на удаление без разделов уровня 7.",
            "- `unused_resources.tsv` — строки `00~resurs.txt`, которые не используются найденными нормами.",
            "- `file_inventory.tsv` — инвентаризация файлов и роль каждого файла.",
        ]
    )
    return "\n".join(lines) + "\n"


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--source", type=Path, default=DEFAULT_SOURCE)
    parser.add_argument("--out", type=Path)
    args = parser.parse_args()

    source = args.source
    out = args.out or (source / "_prepared_text_2026-06-15")
    out.mkdir(parents=True, exist_ok=True)

    hierarchy_path = source / "00~hierarchy.txt"
    nsi_path = source / "00~nsi.txt"
    resource_path = source / "00~resurs.txt"

    hierarchy_lines = read_lines(hierarchy_path)
    nsi_lines = read_lines(nsi_path)
    resource_lines = read_lines(resource_path)

    hierarchy_records = parse_hierarchy(hierarchy_lines)
    norm_lines_by_code: dict[str, list[int]] = defaultdict(list)
    for record in hierarchy_records:
        for norm_code in expand_norm_list(record.norm_list):
            if norm_code:
                norm_lines_by_code[norm_code].append(record.line_no)
    used_norm_codes = set(norm_lines_by_code)

    prf_referenced, prf_mapped_present, prf_missing = collect_expected_prf_files(source)
    prf_allowed_txt = {Path(name).with_suffix(".txt").name for name in prf_referenced}
    norm_files = discover_norm_files(source, prf_allowed_txt)
    norm_records: list[NormRecord] = []
    for norm_file in norm_files:
        norm_records.extend(parse_norm_file(norm_file))

    norm_by_code: dict[str, NormRecord] = {}
    duplicate_norms: dict[str, list[NormRecord]] = defaultdict(list)
    for record in norm_records:
        if record.code in norm_by_code:
            duplicate_norms[record.code].append(record)
        else:
            norm_by_code[record.code] = record

    found_used_norm_codes = used_norm_codes & set(norm_by_code)
    used_norm_records = [norm_by_code[code] for code in sorted(found_used_norm_codes)]

    resource_usage: Counter[str] = Counter()
    for record in used_norm_records:
        resource_usage.update(record.resources)

    resource_catalog_files = discover_resource_catalog_files(source)
    resource_codes_present: set[str] = set()
    resource_lines_by_code: dict[str, str] = {}
    for resource_catalog in resource_catalog_files:
        for line in read_lines(resource_catalog):
            codes = parse_resource_catalog_codes(line)
            resource_codes_present.update(codes)
            for code in codes:
                resource_lines_by_code.setdefault(code, line)

    def resource_exists(code: str) -> bool:
        return any(variant in resource_codes_present for variant in resource_variants(code))

    used_resource_lines = []
    seen_resource_lines = set()
    for code in sorted(resource_usage):
        line = next(
            (resource_lines_by_code[variant] for variant in resource_variants(code) if variant in resource_lines_by_code),
            None,
        )
        if line and line not in seen_resource_lines:
            used_resource_lines.append(line)
            seen_resource_lines.add(line)

    unused_sections = mark_used_hierarchy(hierarchy_records, found_used_norm_codes)
    proposed_unused_sections = [row for row in unused_sections if row["level"] != 7]

    write_cp1251(out / "00~hierarchy.clean.txt", [clean_hierarchy_line(line) for line in hierarchy_lines])
    write_cp1251(out / "00~nsi.clean.txt", [clean_nsi_line(line) for line in nsi_lines])
    write_cp1251(out / "00~norms.used.txt", [record.raw for record in used_norm_records])
    write_cp1251(out / "00~resurs.used.txt", used_resource_lines)

    write_tsv(
        out / "missing_norms.tsv",
        [
            {
                "norm_code": code,
                "hierarchy_lines": ", ".join(str(line_no) for line_no in norm_lines_by_code[code]),
            }
            for code in sorted(used_norm_codes - found_used_norm_codes)
        ],
        ["norm_code", "hierarchy_lines"],
    )
    write_tsv(
        out / "missing_resources.tsv",
        [
            {"resource_code": code, "used_in_norms": resource_usage[code]}
            for code in sorted(code for code in set(resource_usage) if not resource_exists(code))
        ],
        ["resource_code", "used_in_norms"],
    )
    write_tsv(
        out / "resource_usage.tsv",
        [
            {"resource_code": code, "used_in_norms": count}
            for code, count in sorted(resource_usage.items())
        ],
        ["resource_code", "used_in_norms"],
    )
    write_tsv(
        out / "unused_resources.tsv",
        [{"resource_code": code} for code in sorted(resource_codes_present - set(resource_usage))],
        ["resource_code"],
    )
    write_tsv(
        out / "unused_hierarchy_sections.tsv",
        unused_sections,
        ["hierarchy_code", "level", "name", "descendant_norms", "matched_norms"],
    )
    write_tsv(
        out / "unused_hierarchy_sections_proposed.tsv",
        proposed_unused_sections,
        ["hierarchy_code", "level", "name", "descendant_norms", "matched_norms"],
    )
    write_tsv(
        out / "duplicate_norms.tsv",
        [
            {
                "norm_code": code,
                "first_location": f"{norm_by_code[code].file_name}:{norm_by_code[code].line_no}",
                "duplicate_locations": "; ".join(
                    f"{record.file_name}:{record.line_no}" for record in duplicates
                ),
            }
            for code, duplicates in sorted(duplicate_norms.items())
        ],
        ["norm_code", "first_location", "duplicate_locations"],
    )

    inventory_rows = []
    prf_expected_txt = {Path(name).with_suffix(".txt").name.lower() for name in prf_referenced}
    norm_file_names = {path.name.lower() for path in norm_files}
    for path in sorted(source.iterdir()):
        if not path.is_file():
            continue
        name_lower = path.name.lower()
        if path.name == "00.prf":
            role = "list_of_norm_files"
        elif path.name == "00~hierarchy.txt":
            role = "hierarchy_source"
        elif path.name == "00~nsi.txt":
            role = "additional_characteristics_source"
        elif path.name == "00~resurs.txt":
            role = "resource_classifier_source"
        elif name_lower in norm_file_names:
            role = "norm_records"
        elif name_lower in prf_expected_txt:
            role = "referenced_by_prf_not_detected_as_norms"
        else:
            role = "extra_or_reference"
        inventory_rows.append({"file_name": path.name, "size_bytes": path.stat().st_size, "role": role})
    write_tsv(out / "file_inventory.tsv", inventory_rows, ["file_name", "size_bytes", "role"])

    report = build_report(
        source=source,
        out=out,
        hierarchy_records=hierarchy_records,
        norm_records=norm_records,
        used_norm_codes=used_norm_codes,
        found_used_norm_codes=found_used_norm_codes,
        resource_usage=resource_usage,
        resource_codes_present=resource_codes_present,
        prf_referenced=prf_referenced,
        prf_mapped_present=prf_mapped_present,
        prf_missing=prf_missing,
        norm_files=norm_files,
        unused_sections_count=len(unused_sections),
        proposed_unused_sections_count=len(proposed_unused_sections),
    )
    (out / "report.md").write_text(report, encoding="utf-8")

    print(f"Prepared text files and reports in: {out}")


if __name__ == "__main__":
    main()
