from __future__ import annotations

import argparse
import csv
import datetime as dt
import re
import subprocess
from collections import defaultdict
from pathlib import Path


DEFAULT_SOURCE = Path(
    r"D:\OneDrive\Bases\Разработка\000 - RU ГСН-2022\2026-06-15 Сокращенная база\Books"
)
ENCODING = "cp1251"
REPO_ROOT = Path(__file__).resolve().parents[1]
SCHEMA_SQL = REPO_ROOT / "db" / "gsn_schema.sql"

SYSTEM_FILES = {
    "00~f.txt",
    "00~hierarchy.txt",
    "00~nsi.txt",
    "00~resurs.txt",
    "00~ppr.txt",
    "00~uvr.txt",
}

NORM_RANGE_RE = re.compile(r"^[ЕЦУE]\d{4,5}-\d{3}-\d{2}$")
RECORD_CODE_RE = re.compile(r"^[A-ZА-ЯЁ][A-ZА-ЯЁ0-9-]*\d", re.IGNORECASE)
ORIGINAL_CODE_RE = re.compile(r"\(Ш([^)]*)\)")


def read_lines(path: Path) -> list[str]:
    return path.read_text(encoding=ENCODING).splitlines()


def split_record(line: str) -> list[str]:
    body = line.rstrip("\r\n")
    if body.endswith("*"):
        body = body[:-1]
    return body.split("'")


def clean_ref(value: str) -> str:
    return value.strip().strip(";")


def extract_record_code_from_head(head: str) -> str:
    code = head.split(" ", 1)[0].split("(", 1)[0].strip()
    return code if RECORD_CODE_RE.match(code) else ""


def extract_record_code(line: str) -> str:
    fields = split_record(line)
    if not fields:
        return ""
    return extract_record_code_from_head(fields[0])


def extract_original_code(head: str) -> str:
    match = ORIGINAL_CODE_RE.search(head)
    return match.group(1).strip() if match else ""


def expand_range(left: str, right: str) -> list[str]:
    left = clean_ref(left)
    right = clean_ref(right)
    if not NORM_RANGE_RE.match(left) or not NORM_RANGE_RE.match(right):
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


def expand_record_refs(raw_refs: str) -> list[str]:
    if not raw_refs:
        return []
    if ";" in raw_refs:
        refs: list[str] = []
        for group in raw_refs.split(";"):
            refs.extend(expand_record_refs(group))
        return refs
    parts = [clean_ref(part) for part in raw_refs.split("/") if clean_ref(part)]
    if len(parts) == 2 and all(NORM_RANGE_RE.match(part) for part in parts):
        return expand_range(parts[0], parts[1])
    return parts


def parse_resource_tokens(raw_resources: str) -> list[tuple[str, str]]:
    tokens: list[tuple[str, str]] = []
    for token in raw_resources.split("/"):
        token = token.strip()
        if not token:
            continue
        if "." in token:
            code, quantity = token.split(".", 1)
        else:
            code, quantity = token, ""
        code = clean_ref(code)
        if code:
            tokens.append((code, quantity.strip()))
    return tokens


def psql_path() -> str:
    default = Path(r"C:\Program Files\PostgreSQL\16\bin\psql.exe")
    return str(default) if default.exists() else "psql"


def prf_txt_files(source: Path) -> list[str]:
    return [Path(line.strip()).with_suffix(".txt").name for line in read_lines(source / "00.prf") if line.strip()]


def discover_record_files(source: Path) -> list[Path]:
    files: list[Path] = []
    for file_name in prf_txt_files(source):
        if file_name in SYSTEM_FILES:
            continue
        path = source / file_name
        if not path.exists():
            continue
        first_lines = read_lines(path)[:20]
        if any(extract_record_code(line) for line in first_lines):
            files.append(path)
    return files


def record_kind(source_file: str) -> str:
    lower = source_file.lower()
    if lower.startswith("00~c"):
        return "resource_catalog"
    if lower == "00~f.txt":
        return "metadata"
    if lower.startswith("00~k"):
        return "coefficient_catalog"
    return "norm"


def write_tsv(path: Path, fieldnames: list[str], rows: list[dict[str, object]]) -> None:
    with path.open("w", encoding="utf-8", newline="") as file:
        writer = csv.DictWriter(
            file,
            fieldnames=fieldnames,
            delimiter="\t",
            lineterminator="\n",
            extrasaction="ignore",
        )
        writer.writeheader()
        for row in rows:
            writer.writerow(
                {
                    field: (r"\N" if row.get(field) is None else row.get(field, ""))
                    for field in fieldnames
                }
            )


def parse_hierarchy(source: Path) -> tuple[list[dict[str, object]], list[dict[str, object]]]:
    rows: list[dict[str, object]] = []
    refs: list[dict[str, object]] = []
    stack: list[tuple[int, str]] = []

    for line_no, line in enumerate(read_lines(source / "00~hierarchy.txt"), start=1):
        if not line.strip():
            continue
        fields = split_record(line)
        while len(fields) < 5:
            fields.append("")
        code = fields[0]
        level = int(fields[1] or 0)
        while stack and stack[-1][0] >= level:
            stack.pop()
        parent_code = stack[-1][1] if stack else None
        rows.append(
            {
                "code": code,
                "parent_code": parent_code,
                "line_no": line_no,
                "level": level,
                "name": fields[3],
                "unit": fields[4],
                "raw_norm_list": fields[2],
                "raw_line": line,
            }
        )
        for ordinal, ref_code in enumerate(expand_record_refs(fields[2]), start=1):
            refs.append({"hierarchy_code": code, "record_code": ref_code, "ordinal": ordinal})
        stack.append((level, code))

    return rows, refs


def parse_records(source: Path) -> tuple[list[dict[str, object]], list[dict[str, object]], list[dict[str, object]]]:
    records: list[dict[str, object]] = []
    resources: list[dict[str, object]] = []
    duplicates: list[dict[str, object]] = []
    seen: dict[str, dict[str, object]] = {}

    for path in discover_record_files(source):
        for line_no, line in enumerate(read_lines(path), start=1):
            fields = split_record(line)
            if not fields:
                continue
            code = extract_record_code_from_head(fields[0])
            if not code:
                continue
            while len(fields) < 7:
                fields.append("")
            row = {
                "code": code,
                "source_file": path.name,
                "line_no": line_no,
                "original_code": extract_original_code(fields[0]),
                "determinant": fields[1],
                "cost_indicators": fields[2],
                "name": fields[3],
                "unit": fields[4],
                "mass": fields[5],
                "resource_list": fields[6],
                "record_kind": record_kind(path.name),
                "raw_line": line,
            }
            if code in seen:
                duplicates.append(
                    {
                        "code": code,
                        "first_source_file": seen[code]["source_file"],
                        "first_line_no": seen[code]["line_no"],
                        "duplicate_source_file": path.name,
                        "duplicate_line_no": line_no,
                    }
                )
                continue
            seen[code] = row
            records.append(row)
            for ordinal, (resource_code, quantity_text) in enumerate(parse_resource_tokens(fields[6]), start=1):
                resources.append(
                    {
                        "record_code": code,
                        "resource_code": resource_code,
                        "quantity_text": quantity_text,
                        "ordinal": ordinal,
                    }
                )

    return records, resources, duplicates


def parse_nsi(source: Path) -> list[dict[str, object]]:
    rows: list[dict[str, object]] = []
    for line_no, line in enumerate(read_lines(source / "00~nsi.txt"), start=1):
        fields = split_record(line)
        if not fields:
            continue
        while len(fields) < 6:
            fields.append("")
        head = fields[0]
        code = extract_record_code_from_head(head)
        if not code:
            continue
        modifiers = head
        if modifiers.startswith(code):
            modifiers = modifiers[len(code) :]
        modifiers = ORIGINAL_CODE_RE.sub("", modifiers).strip()
        rows.append(
            {
                "record_code": code,
                "line_no": line_no,
                "original_code": extract_original_code(head),
                "modifiers": modifiers,
                "cost_indicators": fields[2],
                "work_composition": fields[5],
                "raw_line": line,
            }
        )
    return rows


def parse_ppr(source: Path) -> tuple[list[dict[str, object]], list[dict[str, object]]]:
    amendments: list[dict[str, object]] = []
    impacts: list[dict[str, object]] = []
    for line_no, line in enumerate(read_lines(source / "00~ppr.txt"), start=1):
        fields = split_record(line)
        if not fields:
            continue
        code = fields[0].strip()
        norm_code_addition = ""
        name_addition = ""
        interface_name = ""
        impact_ordinal = 1
        for field in fields[1:]:
            field = field.strip()
            if field.startswith(("T1+", "Т1+")):
                norm_code_addition = field.split("+", 1)[1].strip()
            elif field.startswith(("T2+", "Т2+")):
                name_addition = field.split("+", 1)[1].strip()
            elif field.startswith(("T3=", "Т3=")):
                interface_name = field.split("=", 1)[1].strip()
            elif field:
                if "=" in field:
                    impact_code, impact_value = field.split("=", 1)
                else:
                    impact_code, impact_value = field, ""
                impacts.append(
                    {
                        "amendment_code": code,
                        "impact_code": impact_code.strip(),
                        "impact_value": impact_value.strip(),
                        "ordinal": impact_ordinal,
                    }
                )
                impact_ordinal += 1
        amendments.append(
            {
                "code": code,
                "norm_code_addition": norm_code_addition,
                "name_addition": name_addition,
                "interface_name": interface_name,
                "raw_line": line,
                "line_no": line_no,
            }
        )
    return amendments, impacts


def parse_amen(source: Path) -> list[dict[str, object]]:
    rows: list[dict[str, object]] = []
    path = source / "00~amen.txt"
    if not path.exists():
        return rows
    for line_no, line in enumerate(read_lines(path), start=1):
        fields = split_record(line)
        if len(fields) < 2:
            continue
        record_code = clean_ref(fields[0])
        for ordinal, amendment_code in enumerate((clean_ref(field) for field in fields[1:]), start=1):
            if amendment_code:
                rows.append(
                    {
                        "record_code": record_code,
                        "amendment_code": amendment_code,
                        "line_no": line_no,
                        "ordinal": ordinal,
                    }
                )
    return rows


def parse_base_info(source: Path) -> tuple[list[dict[str, object]], list[dict[str, object]]]:
    info_rows: list[dict[str, object]] = []
    param_rows: list[dict[str, object]] = []
    path = source / "00~f.txt"
    if not path.exists():
        return info_rows, param_rows

    for line_no, line in enumerate(read_lines(path), start=1):
        fields = split_record(line)
        if not fields:
            continue
        code = fields[0].split("(", 1)[0].strip()
        if not code:
            continue
        info_rows.append(
            {
                "code": code,
                "source_file": path.name,
                "line_no": line_no,
                "raw_line": line,
            }
        )
        for ordinal, field in enumerate(fields[1:], start=1):
            if "=" in field:
                param_key, param_value = field.split("=", 1)
            else:
                param_key, param_value = field, ""
            param_key = param_key.strip()
            if param_key:
                param_rows.append(
                    {
                        "base_code": code,
                        "param_key": param_key,
                        "param_value": param_value.strip(),
                        "ordinal": ordinal,
                    }
                )
    return info_rows, param_rows


def write_load_sql(out: Path) -> Path:
    def sql_path(name: str) -> str:
        return str((out / name).resolve()).replace("\\", "/")

    load_sql = out / "load_gsn.sql"
    load_sql.write_text(
        "\n".join(
            [
                "BEGIN;",
                "TRUNCATE gsn.record_amendments, gsn.amendment_impacts, gsn.amendments,",
                "    gsn.nsi, gsn.record_resources, gsn.records,",
                "    gsn.hierarchy_record_refs, gsn.hierarchy,",
                "    gsn.base_info_params, gsn.base_info, gsn.import_batches CASCADE;",
                rf"\copy gsn.import_batches(id,source_path,prepared_path,imported_at) FROM '{sql_path('import_batches.tsv')}' WITH (FORMAT csv, HEADER true, DELIMITER E'\t', NULL '\N');",
                rf"\copy gsn.base_info(code,source_file,line_no,raw_line) FROM '{sql_path('base_info.tsv')}' WITH (FORMAT csv, HEADER true, DELIMITER E'\t', NULL '\N');",
                rf"\copy gsn.base_info_params(base_code,param_key,param_value,ordinal) FROM '{sql_path('base_info_params.tsv')}' WITH (FORMAT csv, HEADER true, DELIMITER E'\t', NULL '\N');",
                rf"\copy gsn.hierarchy(code,parent_code,line_no,level,name,unit,raw_norm_list,raw_line) FROM '{sql_path('hierarchy.tsv')}' WITH (FORMAT csv, HEADER true, DELIMITER E'\t', NULL '\N');",
                rf"\copy gsn.hierarchy_record_refs(hierarchy_code,record_code,ordinal) FROM '{sql_path('hierarchy_record_refs.tsv')}' WITH (FORMAT csv, HEADER true, DELIMITER E'\t', NULL '\N');",
                rf"\copy gsn.records(code,source_file,line_no,original_code,determinant,cost_indicators,name,unit,mass,resource_list,record_kind,raw_line) FROM '{sql_path('records.tsv')}' WITH (FORMAT csv, HEADER true, DELIMITER E'\t', NULL '\N');",
                rf"\copy gsn.record_resources(record_code,resource_code,quantity_text,ordinal) FROM '{sql_path('record_resources.tsv')}' WITH (FORMAT csv, HEADER true, DELIMITER E'\t', NULL '\N');",
                rf"\copy gsn.nsi(record_code,line_no,original_code,modifiers,cost_indicators,work_composition,raw_line) FROM '{sql_path('nsi.tsv')}' WITH (FORMAT csv, HEADER true, DELIMITER E'\t', NULL '\N');",
                rf"\copy gsn.amendments(code,norm_code_addition,name_addition,interface_name,raw_line,line_no) FROM '{sql_path('amendments.tsv')}' WITH (FORMAT csv, HEADER true, DELIMITER E'\t', NULL '\N');",
                rf"\copy gsn.amendment_impacts(amendment_code,impact_code,impact_value,ordinal) FROM '{sql_path('amendment_impacts.tsv')}' WITH (FORMAT csv, HEADER true, DELIMITER E'\t', NULL '\N');",
                rf"\copy gsn.record_amendments(record_code,amendment_code,line_no,ordinal) FROM '{sql_path('record_amendments.tsv')}' WITH (FORMAT csv, HEADER true, DELIMITER E'\t', NULL '\N');",
                "COMMIT;",
                "",
            ]
        ),
        encoding="utf-8",
    )
    return load_sql


def run_psql(database_url: str, sql_file: Path) -> None:
    subprocess.run([psql_path(), "-v", "ON_ERROR_STOP=1", "-f", str(sql_file), database_url], check=True)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--source", type=Path, default=DEFAULT_SOURCE)
    parser.add_argument("--out", type=Path)
    parser.add_argument("--database-url", default="")
    parser.add_argument("--load", action="store_true", help="Apply schema and load generated TSV files with psql.")
    args = parser.parse_args()

    source = args.source
    out = args.out or (source / "_postgres_import")
    out.mkdir(parents=True, exist_ok=True)

    hierarchy, hierarchy_refs = parse_hierarchy(source)
    base_info, base_info_params = parse_base_info(source)
    records, record_resources, duplicates = parse_records(source)
    nsi = parse_nsi(source)
    amendments, amendment_impacts = parse_ppr(source)
    record_amendments = parse_amen(source)
    amendment_codes = {row["code"] for row in amendments}
    missing_amendments = [
        {"amendment_code": amendment_code}
        for amendment_code in sorted({row["amendment_code"] for row in record_amendments} - amendment_codes)
    ]

    batch_id = dt.datetime.now().strftime("gsn-2022-%Y%m%d-%H%M%S")
    write_tsv(
        out / "import_batches.tsv",
        ["id", "source_path", "prepared_path", "imported_at"],
        [
            {
                "id": batch_id,
                "source_path": str(source),
                "prepared_path": str(source / "_prepared_text_2026-06-15"),
                "imported_at": dt.datetime.now(dt.UTC).isoformat(),
            }
        ],
    )
    write_tsv(out / "base_info.tsv", ["code", "source_file", "line_no", "raw_line"], base_info)
    write_tsv(out / "base_info_params.tsv", ["base_code", "param_key", "param_value", "ordinal"], base_info_params)
    write_tsv(out / "hierarchy.tsv", ["code", "parent_code", "line_no", "level", "name", "unit", "raw_norm_list", "raw_line"], hierarchy)
    write_tsv(out / "hierarchy_record_refs.tsv", ["hierarchy_code", "record_code", "ordinal"], hierarchy_refs)
    write_tsv(
        out / "records.tsv",
        [
            "code",
            "source_file",
            "line_no",
            "original_code",
            "determinant",
            "cost_indicators",
            "name",
            "unit",
            "mass",
            "resource_list",
            "record_kind",
            "raw_line",
        ],
        records,
    )
    write_tsv(out / "record_resources.tsv", ["record_code", "resource_code", "quantity_text", "ordinal"], record_resources)
    write_tsv(out / "nsi.tsv", ["record_code", "line_no", "original_code", "modifiers", "cost_indicators", "work_composition", "raw_line"], nsi)
    write_tsv(out / "amendments.tsv", ["code", "norm_code_addition", "name_addition", "interface_name", "raw_line", "line_no"], amendments)
    write_tsv(out / "amendment_impacts.tsv", ["amendment_code", "impact_code", "impact_value", "ordinal"], amendment_impacts)
    write_tsv(out / "record_amendments.tsv", ["record_code", "amendment_code", "line_no", "ordinal"], record_amendments)
    write_tsv(out / "duplicate_records.tsv", ["code", "first_source_file", "first_line_no", "duplicate_source_file", "duplicate_line_no"], duplicates)
    write_tsv(out / "missing_amendments.tsv", ["amendment_code"], missing_amendments)

    load_sql = write_load_sql(out)
    print(f"Generated TSV files in: {out}")
    print(f"Generated psql load script: {load_sql}")
    print(f"Rows: base_info={len(base_info)}, base_info_params={len(base_info_params)}, hierarchy={len(hierarchy)}, records={len(records)}, resources={len(record_resources)}, nsi={len(nsi)}, amendments={len(amendments)}, incidences={len(record_amendments)}")
    print(f"Missing amendment dictionary records referenced by amen: {len(missing_amendments)}")

    if args.load:
        if not args.database_url:
            raise SystemExit("--database-url is required with --load")
        run_psql(args.database_url, SCHEMA_SQL)
        run_psql(args.database_url, load_sql)
        print("Loaded GSN data into PostgreSQL.")


if __name__ == "__main__":
    main()
