from __future__ import annotations

import argparse
import hashlib
import re
import shutil
import subprocess
import sys
from pathlib import Path

DEFAULT_BASE = Path(
    r"D:\OneDrive\Bases\Разработка\000 - RU ГСН-2022\2026-06-15 Сокращенная база"
)
PDF_REF_RE = re.compile(r"^[\w~.-]+\.PDF$", re.IGNORECASE)


def ensure_psycopg():
    try:
        import psycopg  # noqa: F401
    except ImportError:
        subprocess.check_call([sys.executable, "-m", "pip", "install", "psycopg[binary]"])
    import psycopg

    return psycopg


def build_pdf_index(pdf_dir: Path) -> dict[str, Path]:
    index: dict[str, Path] = {}
    for path in pdf_dir.iterdir():
        if path.is_file():
            index[path.name.upper()] = path
    return index


def resolve_pdf_path(ref: str, index: dict[str, Path]) -> Path | None:
    ref_upper = ref.strip().upper()
    if ref_upper in index:
        return index[ref_upper]
    if ref_upper.startswith("01~"):
        alt = "00~" + ref_upper[3:]
        if alt in index:
            return index[alt]
    if ref_upper.startswith("00~"):
        alt = "01~" + ref_upper[3:]
        if alt in index:
            return index[alt]
    return None


def collect_referenced_files(
    conn,
    supplement_code: str | None,
    index: dict[str, Path],
) -> tuple[dict[str, Path], list[tuple[str, str, str]], list[str]]:
    """Returns mapping ref_upper -> disk path, and rows (supplement, hierarchy_code, document_file)."""
    with conn.cursor() as cur:
        if supplement_code:
            cur.execute(
                """
                SELECT supplement_code, code, document_ref
                FROM gsn.hierarchy
                WHERE node_type = 'Документ' AND document_ref <> ''
                  AND supplement_code = %s
                """,
                (supplement_code,),
            )
        else:
            cur.execute(
                """
                SELECT supplement_code, code, document_ref
                FROM gsn.hierarchy
                WHERE node_type = 'Документ' AND document_ref <> ''
                """
            )
        rows = cur.fetchall()

    ref_to_path: dict[str, Path] = {}
    updates: list[tuple[str, str, str]] = []
    missing_refs: list[str] = []

    for sup_code, hierarchy_code, document_ref in rows:
        resolved = resolve_pdf_path(document_ref, index)
        if not resolved:
            missing_refs.append(document_ref)
            continue
        ref_upper = document_ref.strip().upper()
        ref_to_path[ref_upper] = resolved
        updates.append((sup_code, hierarchy_code, resolved.name))

    return ref_to_path, updates, sorted(set(missing_refs))


def load_documents(conn, ref_to_path: dict[str, Path]) -> int:
    loaded = 0
    with conn.cursor() as cur:
        cur.execute("TRUNCATE gsn.documents;")
        for path in sorted({p.resolve() for p in ref_to_path.values()}, key=lambda p: p.name):
            content = path.read_bytes()
            digest = hashlib.sha256(content).hexdigest()
            cur.execute(
                """
                INSERT INTO gsn.documents (file_name, content, size_bytes, sha256)
                VALUES (%s, %s, %s, %s)
                ON CONFLICT (file_name) DO UPDATE
                SET content = EXCLUDED.content,
                    size_bytes = EXCLUDED.size_bytes,
                    sha256 = EXCLUDED.sha256,
                    imported_at = now()
                """,
                (path.name, content, len(content), digest),
            )
            loaded += 1
    conn.commit()
    return loaded


def update_hierarchy_files(conn, updates: list[tuple[str, str, str]]) -> int:
    with conn.cursor() as cur:
        for supplement_code, hierarchy_code, document_file in updates:
            cur.execute(
                """
                UPDATE gsn.hierarchy
                SET document_file = %s
                WHERE supplement_code = %s AND code = %s
                """,
                (document_file, supplement_code, hierarchy_code),
            )
    conn.commit()
    return len(updates)


def move_unused_pdfs(pdf_dir: Path, common_dir: Path, used_paths: set[Path]) -> int:
    common_dir.mkdir(parents=True, exist_ok=True)
    moved = 0
    for path in sorted(pdf_dir.iterdir()):
        if not path.is_file():
            continue
        if path.resolve() in used_paths:
            continue
        target = common_dir / path.name
        if target.exists():
            target.unlink()
        shutil.move(str(path), str(target))
        moved += 1
    return moved


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--base", type=Path, default=DEFAULT_BASE)
    parser.add_argument("--database-url", required=True)
    parser.add_argument("--supplement", default="", help="Limit to one supplement code.")
    parser.add_argument(
        "--move-unused",
        action=argparse.BooleanOptionalAction,
        default=True,
        help="Move unreferenced PDFs to PDF_Common (default: true).",
    )
    args = parser.parse_args()

    pdf_dir = args.base / "PDF"
    common_dir = args.base / "PDF_Common"
    if not pdf_dir.is_dir():
        raise SystemExit(f"PDF folder not found: {pdf_dir}")

    psycopg = ensure_psycopg()
    index = build_pdf_index(pdf_dir)

    with psycopg.connect(args.database_url) as conn:
        ref_to_path, updates, missing_refs = collect_referenced_files(
            conn,
            args.supplement or None,
            index,
        )
        loaded = load_documents(conn, ref_to_path)
        updated = update_hierarchy_files(conn, updates)

    used_paths = {p.resolve() for p in ref_to_path.values()}
    moved = move_unused_pdfs(pdf_dir, common_dir, used_paths) if args.move_unused else 0

    print(f"referenced_unique_pdfs={len(ref_to_path)}")
    print(f"documents_loaded={loaded}")
    print(f"hierarchy_nodes_updated={updated}")
    print(f"missing_refs={len(missing_refs)}")
    if missing_refs:
        print("missing sample:", missing_refs[:10])
    if args.move_unused:
        print(f"pdfs_moved_to_common={moved}")


if __name__ == "__main__":
    main()
