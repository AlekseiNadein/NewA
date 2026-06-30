#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Generate architecture description PDF (NAV SaaS MVP)."""

import re
import textwrap
from datetime import date
from pathlib import Path

from fpdf import FPDF

ROOT = Path(__file__).resolve().parents[1]
DOCS_DIR = ROOT / "docs"
FONT_REGULAR = Path(r"C:\Windows\Fonts\arial.ttf")
FONT_BOLD = Path(r"C:\Windows\Fonts\arialbd.ttf")

# A4 printable width at 10pt Arial ~ 92 chars; mono 9pt ~ 100 chars
BODY_WRAP = 88
MONO_WRAP = 96
BULLET_PREFIX = "- "


def break_long_tokens(text: str, limit: int = 42) -> str:
    """Insert zero-width spaces so fpdf can wrap unbreakable tokens (paths, URLs)."""
    zwsp = "\u200b"

    def split_token(match: re.Match[str]) -> str:
        token = match.group(0)
        if len(token) <= limit:
            return token
        chunks = [token[i : i + limit] for i in range(0, len(token), limit)]
        return zwsp.join(chunks)

    return re.sub(r"\S{" + str(limit + 1) + r",}", split_token, text)


class ArchPDF(FPDF):
    def __init__(self) -> None:
        super().__init__(format="A4", unit="mm")
        self.set_margins(left=22, top=18, right=22)
        self.set_auto_page_break(auto=True, margin=20)
        self.add_font("Arial", "", str(FONT_REGULAR))
        self.add_font("Arial", "B", str(FONT_BOLD))

    def footer(self) -> None:
        self.set_y(-14)
        self.set_font("Arial", "", 8)
        self.set_text_color(120, 120, 120)
        self.cell(0, 8, f"Стр. {self.page_no()}", align="C")
        self.set_text_color(0, 0, 0)

    def _at_margin(self) -> None:
        self.set_x(self.l_margin)

    def _write_block(self, text: str, size: int, style: str, line_h: float, fill: bool = False) -> None:
        self.set_font("Arial", style, size)
        if fill:
            self.set_fill_color(245, 245, 245)
        for paragraph in text.split("\n"):
            if paragraph == "":
                self.ln(line_h * 0.6)
                continue
            wrapped = textwrap.fill(
                break_long_tokens(paragraph),
                width=MONO_WRAP if fill else BODY_WRAP,
                break_long_words=True,
                break_on_hyphens=False,
            )
            for line in wrapped.splitlines():
                self._at_margin()
                self.multi_cell(self.epw, line_h, line, fill=fill)
        if fill:
            self.ln(2)
        else:
            self.ln(1)

    def write_doc_title(self, text: str, version_label: str) -> None:
        self._at_margin()
        self.set_font("Arial", "B", 16)
        self.multi_cell(self.epw, 8, text)
        self._at_margin()
        self.set_font("Arial", "", 10)
        self.set_text_color(80, 80, 80)
        self.cell(
            self.epw,
            6,
            version_label,
            align="R",
            new_x="LMARGIN",
            new_y="NEXT",
        )
        self.set_text_color(0, 0, 0)
        self.ln(3)

    def lead(self, text: str) -> None:
        self._write_block(text, 10, "", 5.5)
        self.ln(1)

    def section_title(self, text: str) -> None:
        self.ln(3)
        self._at_margin()
        self.set_font("Arial", "B", 13)
        self.multi_cell(self.epw, 7, text)
        self.ln(1)

    def subsection_title(self, text: str) -> None:
        self.ln(2)
        self._at_margin()
        self.set_font("Arial", "B", 11)
        self.multi_cell(self.epw, 6, text)
        self.ln(0.5)

    def body(self, text: str) -> None:
        self._write_block(text, 10, "", 5.5)

    def bullet(self, text: str) -> None:
        self.set_font("Arial", "", 10)
        wrapped = textwrap.fill(
            break_long_tokens(text),
            width=BODY_WRAP - len(BULLET_PREFIX),
            break_long_words=True,
            break_on_hyphens=False,
        )
        lines = wrapped.splitlines()
        for i, line in enumerate(lines):
            self._at_margin()
            prefix = BULLET_PREFIX if i == 0 else " " * len(BULLET_PREFIX)
            self.multi_cell(self.epw, 5.5, prefix + line)

    def mono_block(self, text: str) -> None:
        self._write_block(text, 9, "", 4.8, fill=True)


def build_pdf() -> None:
    version_date = date.today().strftime("%Y-%m-%d")
    out_path = DOCS_DIR / f"архитектура-проекта-{version_date}.pdf"

    pdf = ArchPDF()
    pdf.add_page()

    pdf.write_doc_title("Архитектура проекта NAV SaaS MVP", f"Версия {version_date}")
    pdf.lead(
        "Пробный вертикальный срез SaaS-платформы для автоматизации выпуска смет "
        "(ГСН-2022, ФГИС ЦС). Целевая схема: ADMIN FR / USER FR -> BK -> AC/JWT -> storage."
    )

    pdf.section_title("Общая схема")
    pdf.body(
        "Два основных процесса: API-сервер (run.bat, порт 8080) и отдельный calc worker "
        "(run-calc-worker.bat). Браузерный UI (web/) обращается к API по REST с JWT в cookie. "
        "Сервер раздаёт статику, обслуживает auth и CRUD, читает справочник ГСН. "
        "Worker забирает задачи из очереди PostgreSQL и обогащает строки смет данными ГСН."
    )
    pdf.mono_block(
        "Клиенты: web/ (основной UI), web/admin.html (админка)\n"
        "Процессы: backend/cmd/server, backend/cmd/calc_worker\n"
        "Хранилища: data/app.json, PostgreSQL app_*, PostgreSQL gsn.*/fgis_cs.*"
    )

    pdf.subsection_title("Процессы")
    pdf.bullet("API + frontend (run.bat) — HTTP-сервер, auth, CRUD, GSN API, раздача web/")
    pdf.bullet("Calc worker (run-calc-worker.bat) — асинхронный расчёт позиций смет из очереди")

    pdf.section_title("Backend (Go)")
    pdf.body(
        "Монолитный HTTP-сервер с модульной внутренней структурой. "
        "Точка входа — backend/cmd/server/main.go."
    )
    pdf.mono_block(
        "backend/\n"
        "  cmd/server/          — основной API\n"
        "  cmd/calc_worker/     — воркер расчёта смет\n"
        "  cmd/import_*         — CLI-импорт справочников\n"
        "  internal/api/        — маршруты, middleware, handlers\n"
        "  internal/auth/       — JWT (HMAC SHA-256), пароли, cookie-сессии\n"
        "  internal/domain/     — доменные модели\n"
        "  internal/store/      — FileStore: гибрид JSON + PostgreSQL\n"
        "  internal/gsn/        — справочник ГСН-2022 и ФГИС\n"
        "  internal/calcworker/ — логика воркера и Manager пула goroutine\n"
        "  internal/presence/   — блокировки смет, лицензионные сессии (in-memory)"
    )

    pdf.subsection_title("API-сервер (internal/api/server.go)")
    pdf.bullet("http.ServeMux с префиксом /api/")
    pdf.bullet("Middleware: CORS, JWT из cookie, проверки authorized / admin")
    pdf.bullet("Статика из APP_WEB_DIR; админ-страница /admin")
    pdf.body("Группы эндпоинтов:")
    pdf.bullet("Auth: /api/auth/login, logout, register, /api/me")
    pdf.bullet("Организация: /api/companies, /api/users")
    pdf.bullet("Предметная область: /api/constructions, /api/objects, /api/estimates")
    pdf.bullet("Расчёт: GET /api/estimates/{id}/calc-status")
    pdf.bullet("GSN: /api/gsn/hierarchy, record, regions, fgis-sets и др.")
    pdf.bullet("Админ: /api/admin/estimate-locks, /api/admin/licenses")
    pdf.bullet("Настройки: GET/PUT /api/settings (число calc worker-ов)")

    pdf.subsection_title("Аутентификация (internal/auth)")
    pdf.bullet("Роли: super_admin, company_admin, user")
    pdf.bullet("Логин по компании + ФИО или e-mail + пароль")
    pdf.bullet("JWT в HTTP-only cookie; пароли SHA-256 с солью")

    pdf.subsection_title("Хранилище (internal/store/file_store.go)")
    pdf.body("Гибридная модель:")
    pdf.bullet("Пользователи, компании, лицензии, настройки — data/app.json")
    pdf.bullet(
        "Стройки, объекты, сметы, строки, очередь — PostgreSQL (app_*, estimate_calc_jobs) "
        "при APP_DATABASE_URL"
    )
    pdf.body("Целевая схема описана в db/schema.sql.")

    pdf.subsection_title("Модуль ГСН (internal/gsn)")
    pdf.body("Отдельное подключение APP_GSN_DATABASE_URL. Схема — db/gsn_schema.sql:")
    pdf.bullet("gsn.supplements, gsn.hierarchy, gsn.base_info — иерархия ГСН-2022")
    pdf.bullet("fgis_cs.* — наборы и строки ФГИС ЦС (цены по районам)")
    pdf.bullet("Импорт — CLI import_regions, import_resource_codifier, import_fgis_cs и scripts/")

    pdf.subsection_title("Presence (internal/presence)")
    pdf.bullet("EstimateLocks — эксклюзивная блокировка сметы при редактировании (TTL ~90 с)")
    pdf.bullet("LicenseSessions — учёт активных лицензий по подразделам ГСН (in-memory)")

    pdf.section_title("Асинхронный расчёт смет")
    pdf.body(
        "Ключевая идея: источник истины — текстовая строка (raw_text), таблица — проекция, "
        "очередь — транспорт для worker-ов."
    )
    pdf.mono_block(
        "1. UI -> PUT /api/estimates (parsed lines)\n"
        "2. API -> app_estimate_lines (calc_status=queued) + estimate_calc_jobs\n"
        "3. Calc worker -> ClaimEstimateCalcJob (FOR UPDATE SKIP LOCKED)\n"
        "4. Worker -> gsn.GetRecordDetail(code, fgisSet, district)\n"
        "5. Worker -> calc_json, calc_status=done\n"
        "6. UI polling GET calc-status -> обогащение таблицы"
    )
    pdf.bullet("Очередь estimate_calc_jobs: queued | leased | done | failed | dead")
    pdf.bullet("Lease ~45 с; retry с backoff; после max_attempts -> dead")
    pdf.bullet("Worker не встроен в server — только calc_worker (1–16 goroutine по настройке)")
    pdf.bullet("Парсинг объёма из raw_text — на backend (normalizeEstimateItem, quantity_expr.go)")

    pdf.section_title("Frontend (web/)")
    pdf.body("SPA без фреймворка — vanilla JS + HTML + CSS.")
    pdf.bullet("index.html + app.js — основное приложение")
    pdf.bullet("admin.html + admin.js — админ-панель")
    pdf.bullet("shared.js, login-draft.js, styles.css, regions.json")
    pdf.body("Разделы UI:")
    pdf.bullet("База -> ГСН-2022 (иерархия, поиск), позиции пользователя")
    pdf.bullet("Стройки -> иерархия Стройка -> Объект -> Смета")
    pdf.bullet("Настройки (число worker-ов расчёта)")
    pdf.body(
        "Редактор сметы: текстовый режим (формат «Исходные данные») и табличный "
        "(до расчёта — шифр + объём; после worker — полная строка из calc_json). "
        "Состояние редактора — sessionStorage."
    )

    pdf.section_title("Базы данных")
    pdf.mono_block(
        "APP_DATABASE_URL:\n"
        "  app_constructions, app_construction_objects, app_estimates,\n"
        "  app_estimate_lines, estimate_calc_jobs, app_settings\n"
        "APP_GSN_DATABASE_URL:\n"
        "  gsn.supplements, gsn.hierarchy, gsn.base_info, fgis_cs.*"
    )
    pdf.body(
        "Переменные окружения: APP_ADDR, APP_WEB_DIR, APP_DATA_PATH, "
        "APP_DATABASE_URL, APP_GSN_DATABASE_URL, APP_JWT_SECRET."
    )

    pdf.section_title("Вспомогательные инструменты")
    pdf.bullet("scripts/import_gsn_*.py — импорт ГСН в PostgreSQL")
    pdf.bullet("prep_gsn_books.py — подготовка книг ГСН")
    pdf.bullet("tools/ — тестовые и отладочные скрипты")
    pdf.bullet("backup-sources.ps1 — резервные копии исходников")

    pdf.section_title("Текущее состояние и эволюция")
    pdf.bullet("Сделано: PG для смет и очереди; отдельный calc worker; GSN-модуль; JWT; мультитенантность")
    pdf.bullet("Частично: users/companies в JSON (app.json), схема PG есть в db/schema.sql")
    pdf.bullet("Запланировано: полный переход store на PG; вынос auth; outbox/events; granular permissions")
    pdf.body(
        "Архитектура MVP: один Go-бинарник + worker-процесс + статический frontend. "
        "Очередь — таблица PostgreSQL с FOR UPDATE SKIP LOCKED, без message broker."
    )

    out_path.parent.mkdir(parents=True, exist_ok=True)
    pdf.output(str(out_path))
    print(f"Written: {out_path}")


if __name__ == "__main__":
    build_pdf()
