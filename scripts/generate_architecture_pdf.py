#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Generate architecture description PDF (NAV SaaS MVP)."""

import re
import sys
import textwrap
from datetime import date
from pathlib import Path

from fpdf import FPDF
from PIL import Image

_SCRIPTS_DIR = Path(__file__).resolve().parent
if str(_SCRIPTS_DIR) not in sys.path:
    sys.path.insert(0, str(_SCRIPTS_DIR))

from architecture_diagram import render_architecture_diagram

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

    def embed_diagram(self, image_path: Path) -> None:
        self._at_margin()
        with Image.open(image_path) as im:
            w_px, h_px = im.size
        width_mm = self.epw
        height_mm = width_mm * h_px / w_px
        self.image(str(image_path), w=width_mm, h=height_mm)
        self.ln(2)

TECH_STACK = (
    "Клиент: HTML5, CSS3, Vanilla JavaScript, sessionStorage\n"
    "Прокси: Nginx (deploy/nginx.conf)\n"
    "Серверы: Go 1.25, net/http, pgx/v5 (PostgreSQL)\n"
    "Auth: JWT HMAC-SHA256, HTTP-only cookie\n"
    "Очередь: RabbitMQ (AMQP), transactional outbox\n"
    "Сборка и запуск: go build, run.bat (Windows)"
)


def build_pdf() -> None:
    version_date = date.today().strftime("%Y-%m-%d")
    out_path = DOCS_DIR / f"архитектура-проекта-{version_date}.pdf"

    pdf = ArchPDF()
    pdf.add_page()

    pdf.write_doc_title("Архитектура проекта NAV SaaS MVP", f"Версия {version_date}")
    pdf.lead(
        "SaaS-платформа для автоматизации выпуска смет (ГСН-2022, ФГИС ЦС). "
        "Мультисервисный контур: nginx (единая точка входа) -> auth-сервис + UserData-server + calc worker."
    )

    pdf.section_title("Схема приложения")
    diagram_path = DOCS_DIR / "_architecture-diagram.png"
    render_architecture_diagram(diagram_path)
    pdf.embed_diagram(diagram_path)
    pdf.mono_block(TECH_STACK)

    pdf.section_title("Общая схема")
    pdf.body(
        "Запуск одной командой run.bat поднимает четыре компонента. "
        "Публичный вход — nginx на :8080 (UI и маршрутизация). "
        "Auth API проксируется на :8081, бизнес-API и статика — на UserData-server :8090. "
        "Calc worker работает в фоне и не использует JWT."
    )
    pdf.mono_block(
        "Клиенты: web/ (основной UI), web/admin.html (админка)\n"
        "Nginx :8080 -> auth :8081 | app :8090\n"
        "Процессы: nav-auth-server, UserData-server (nav-server.exe), nav-calc-worker, nginx\n"
        "Хранилища: PostgreSQL auth.*, app_*, gsn.*/fgis_cs.*; RabbitMQ; data/app.json (legacy)"
    )

    pdf.subsection_title("Процессы (run.bat)")
    pdf.bullet("Nginx (:8080) — единая точка входа; deploy/nginx.conf")
    pdf.bullet("Auth service (:8081) — login, users, companies, licenses; cmd/auth_server")
    pdf.bullet("UserData-server (:8090) — бизнес-API, статика, outbox publisher, healthz; cmd/server")
    pdf.bullet("Calc worker (фон) — потребитель очереди; пишет calc_json/calc_status в app.*")

    pdf.section_title("Backend (Go)")
    pdf.body(
        "Несколько процессов с общими пакетами internal/. "
        "App-сервер: backend/cmd/server/main.go. Auth: backend/cmd/auth_server."
    )
    pdf.mono_block(
        "backend/\n"
        "  cmd/server/           — app API + outbox publisher\n"
        "  cmd/auth_server/      — auth API (:8081)\n"
        "  cmd/calc_worker/      — расчёт позиций (Rabbit / legacy DB)\n"
        "  cmd/migrate_auth/     — импорт users из JSON в auth.*\n"
        "  internal/api/         — маршруты app, healthz, queue-stats\n"
        "  internal/authapi/     — handlers auth API\n"
        "  internal/auth/        — JWT verify (app), пароли (auth)\n"
        "  internal/authstore/   — PG: auth.companies, auth.users, licenses\n"
        "  internal/store/       — сметы, стройки, outbox, очередь DB\n"
        "  internal/gsn/         — справочник ГСН-2022 и ФГИС\n"
        "  internal/calcworker/  — worker, Rabbit consumer, Manager\n"
        "  internal/outbox/      — publisher outbox -> RabbitMQ\n"
        "  internal/queue/       — инспекция очередей RabbitMQ\n"
        "  internal/presence/    — блокировки смет, лиценз. сессии (in-memory)"
    )

    pdf.subsection_title("Auth-контур")
    pdf.bullet("users/companies/licenses — только PostgreSQL auth.* (db/auth_schema.sql)")
    pdf.bullet("JWT в cookie; claims: name, authorized; app проверяет доступ по claims")
    pdf.bullet("APP_AUTH_DATABASE_URL обязателен; APP_JWT_SECRET одинаковый у auth и app")
    pdf.bullet("Nginx маршрутизирует /api/auth/*, /api/me, /api/users, /api/companies -> :8081")
    pdf.bullet("Go reverse proxy в app удалён; auth не участвует в расчёте смет")

    pdf.subsection_title("App API (internal/api/server.go)")
    pdf.bullet("Стройки, объекты, сметы, GSN, settings, estimate-locks, license-sessions")
    pdf.bullet("GET /api/healthz — состояние очереди, publisher/consumer, outbox, DLQ")
    pdf.bullet("GET /api/admin/queue-stats — мониторинг RabbitMQ (только админы)")
    pdf.bullet("GET /api/estimates/{id}/calc-status — polling статуса расчёта")
    pdf.bullet("Статика web/; админка /admin")

    pdf.subsection_title("Хранилище (internal/store)")
    pdf.body("Предметные данные — PostgreSQL app_* при APP_DATABASE_URL:")
    pdf.bullet("app_constructions, app_construction_objects, app_estimates, app_estimate_lines")
    pdf.bullet("app_settings, outbox_events, calc_message_receipts (идемпотентность)")
    pdf.bullet("estimate_calc_jobs — только при APP_QUEUE_MODE=db (legacy fallback)")
    pdf.body(
        "data/app.json — legacy snapshot настроек (если нет PG); учётки вынесены в auth.*."
    )

    pdf.subsection_title("Модуль ГСН (internal/gsn)")
    pdf.body("Отдельное подключение APP_GSN_DATABASE_URL. Схема — db/gsn_schema.sql:")
    pdf.bullet("gsn.supplements, gsn.hierarchy, gsn.base_info — иерархия ГСН-2022")
    pdf.bullet("fgis_cs.* — наборы и строки ФГИС ЦС (цены по районам)")
    pdf.bullet("Импорт — CLI import_regions, import_resource_codifier, import_fgis_cs")

    pdf.subsection_title("Presence (internal/presence)")
    pdf.bullet("EstimateLocks — эксклюзивная блокировка сметы (TTL ~90 с)")
    pdf.bullet("LicenseSessions — учёт активных лицензий по подразделам ГСН (in-memory)")

    pdf.section_title("Асинхронный расчёт смет")
    pdf.body(
        "Источник истины — текстовая строка (raw_text). Таблица — проекция. "
        "Основной транспорт — RabbitMQ (APP_QUEUE_MODE=rabbit)."
    )
    pdf.mono_block(
        "1. UI -> PUT /api/estimates\n"
        "2. API -> app_estimate_lines (calc_status=queued) + outbox_events (TX)\n"
        "3. Outbox publisher -> RabbitMQ exchange estimate.calc\n"
        "4. Queue estimate.calc.main -> calc_worker (RunRabbit)\n"
        "5. Worker -> GSN (GetRecordDetail) + app.* (calc_json, calc_status=done)\n"
        "6. UI polling GET calc-status -> обогащение таблицы"
    )
    pdf.bullet("Идемпотентность: calc_message_receipts (estimate_id, line_id, revision)")
    pdf.bullet("Очереди: main, retry (5s/30s/120s), DLQ; ops: purge/replay DLQ")
    pdf.bullet("Consumer heartbeat в app_settings; healthz: pipelineReady, releaseReady")
    pdf.bullet("Legacy: APP_QUEUE_MODE=db -> estimate_calc_jobs + FOR UPDATE SKIP LOCKED")
    pdf.bullet("Worker: таймаут позиции 40 с, max 8 goroutine, GSN pool по числу worker-ов")
    pdf.bullet("Парсинг объёма из raw_text — backend (quantity_expr.go, normalizeEstimateItem)")

    pdf.section_title("Frontend (web/)")
    pdf.body("SPA без фреймворка — vanilla JS + HTML + CSS.")
    pdf.bullet("index.html + app.js — основное приложение")
    pdf.bullet("admin.html + admin.js — админ-панель")
    pdf.body("Разделы UI:")
    pdf.bullet("База -> ГСН-2022 (иерархия, поиск), позиции пользователя")
    pdf.bullet("Стройки -> иерархия Стройка -> Объект -> Смета")
    pdf.bullet("Настройки (число worker-ов расчёта, 1–8)")
    pdf.body("Админка (/admin): пользователи, лицензии, блокировки смет, мониторинг очереди.")
    pdf.body(
        "Редактор сметы: текстовый и табличный режимы; до расчёта — шифр + объём; "
        "после worker — полная строка из calc_json. Состояние — sessionStorage."
    )

    pdf.section_title("Базы данных")
    pdf.mono_block(
        "APP_AUTH_DATABASE_URL:\n"
        "  auth.companies, auth.users, auth.company_licenses\n"
        "APP_DATABASE_URL:\n"
        "  app_*, outbox_events, calc_message_receipts, estimate_calc_jobs (legacy)\n"
        "APP_GSN_DATABASE_URL:\n"
        "  gsn.*, fgis_cs.*"
    )

    pdf.subsection_title("Переменные окружения")
    pdf.bullet("APP_ADDR (:8090), APP_WEB_DIR, APP_DATA_PATH")
    pdf.bullet("APP_DATABASE_URL, APP_AUTH_DATABASE_URL, APP_GSN_DATABASE_URL")
    pdf.bullet("APP_JWT_SECRET (общий для auth и app)")
    pdf.bullet("APP_QUEUE_MODE=rabbit, APP_RABBITMQ_URL, APP_RABBITMQ_EXCHANGE")
    pdf.bullet("APP_RABBITMQ_PREFETCH, APP_OUTBOX_PUBLISH_BATCH, APP_OUTBOX_PUBLISH_INTERVAL")

    pdf.section_title("Вспомогательные инструменты")
    pdf.bullet("scripts/restart-{auth-server,calc-worker,nginx}.bat")
    pdf.bullet("scripts/{smoke,load}-rabbit-calc.ps1, rabbit-queue-status.bat")
    pdf.bullet("scripts/{purge,replay}-rabbit-dlq.bat, rollback-queue-db.bat")
    pdf.bullet("scripts/import_gsn_*.py, backup-sources.ps1")

    pdf.section_title("Текущее состояние")
    pdf.bullet("Реализовано: вынос auth в отдельный сервис + auth.* PG + nginx")
    pdf.bullet("Реализовано: RabbitMQ + outbox + идемпотентность + мониторинг в админке")
    pdf.bullet("Реализовано: calc worker отдельно; защита от зависания (таймаут, cap 8)")
    pdf.body(
        "Следующие шаги (опционально): оптимизация listRecordResources (N+1), "
        "алерты при росте DLQ, Prometheus/Grafana поверх healthz."
    )

    out_path.parent.mkdir(parents=True, exist_ok=True)
    pdf.output(str(out_path))
    print(f"Written: {out_path}")


if __name__ == "__main__":
    build_pdf()
