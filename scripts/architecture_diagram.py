# -*- coding: utf-8 -*-
"""Render NAV SaaS architecture diagram as PNG."""

from __future__ import annotations

from pathlib import Path

import matplotlib.pyplot as plt
from matplotlib import font_manager
from matplotlib.patches import FancyArrowPatch, FancyBboxPatch

FONT_REGULAR = Path(r"C:\Windows\Fonts\arial.ttf")

# Layer colors
C_CLIENT = "#dbeafe"
C_PROXY = "#e2e8f0"
C_AUTH = "#fce7f3"
C_APP = "#d1fae5"
C_WORKER = "#ccfbf1"
C_DB = "#ffedd5"
C_QUEUE = "#ede9fe"
C_EDGE = "#334155"
C_MUTED = "#64748b"
C_TEXT = "#0f172a"
C_SUB = "#475569"
C_BG = "#f8fafc"


def _font(size: int, weight: str = "normal"):
    return font_manager.FontProperties(fname=str(FONT_REGULAR), size=size, weight=weight)


def _box(ax, x, y, w, h, title: str, subtitle: str, fill: str) -> tuple[float, float]:
    patch = FancyBboxPatch(
        (x, y),
        w,
        h,
        boxstyle="round,pad=0.015,rounding_size=0.12",
        facecolor=fill,
        edgecolor=C_EDGE,
        linewidth=1.4,
        zorder=2,
    )
    ax.add_patch(patch)
    ax.text(
        x + w / 2,
        y + h * 0.64,
        title,
        ha="center",
        va="center",
        fontproperties=_font(11, "bold"),
        color=C_TEXT,
        zorder=3,
    )
    ax.text(
        x + w / 2,
        y + h * 0.28,
        subtitle,
        ha="center",
        va="center",
        fontproperties=_font(8),
        color=C_SUB,
        zorder=3,
    )
    return x + w / 2, y + h / 2


def _arrow(ax, x1, y1, x2, y2, label: str = "", rad: float = 0.0) -> None:
    style = f"arc3,rad={rad}" if rad else "arc3"
    ax.add_patch(
        FancyArrowPatch(
            (x1, y1),
            (x2, y2),
            arrowstyle="-|>",
            mutation_scale=12,
            linewidth=1.6,
            color=C_MUTED,
            connectionstyle=style,
            zorder=1,
        )
    )
    if label:
        mx, my = (x1 + x2) / 2, (y1 + y2) / 2
        ax.text(
            mx,
            my + 0.12,
            label,
            ha="center",
            va="center",
            fontproperties=_font(7),
            color=C_MUTED,
            bbox=dict(boxstyle="round,pad=0.25", facecolor="white", edgecolor="#e2e8f0", alpha=0.95),
            zorder=4,
        )


def _legend(ax, x, y) -> None:
    items = [
        (C_CLIENT, "Клиент"),
        (C_PROXY, "Прокси"),
        (C_AUTH, "Auth"),
        (C_APP, "UserData"),
        (C_WORKER, "Worker"),
        (C_QUEUE, "Очередь"),
        (C_DB, "PostgreSQL"),
    ]
    col_w = 1.35
    for i, (color, label) in enumerate(items):
        cx = x + i * col_w
        ax.add_patch(
            FancyBboxPatch(
                (cx, y),
                0.22,
                0.22,
                boxstyle="round,pad=0.01,rounding_size=0.04",
                facecolor=color,
                edgecolor=C_EDGE,
                linewidth=0.8,
                zorder=2,
            )
        )
        ax.text(cx + 0.32, y + 0.11, label, va="center", fontproperties=_font(7), color=C_SUB, zorder=3)


def render_architecture_diagram(output_path: Path) -> Path:
    fig, ax = plt.subplots(figsize=(10.2, 11.8), dpi=160)
    fig.patch.set_facecolor(C_BG)
    ax.set_facecolor(C_BG)
    ax.set_xlim(0, 10)
    ax.set_ylim(0, 12.5)
    ax.axis("off")

    # Top-down layout
    _box(ax, 3.4, 11.0, 3.2, 0.95, "Браузер", "HTML5 · CSS3 · Vanilla JS", C_CLIENT)
    _box(ax, 3.1, 9.55, 3.8, 0.9, "Nginx", "reverse proxy · :8080", C_PROXY)
    _box(ax, 0.55, 7.55, 3.35, 1.05, "auth-server", "Go 1.25 · JWT · :8081", C_AUTH)
    _box(ax, 5.93, 7.55, 3.75, 1.05, "UserData-server", "Go 1.25 · REST · outbox · :8090", C_APP)
    _box(ax, 0.55, 5.55, 3.35, 0.95, "PostgreSQL", "auth.* · pgx/v5", C_DB)
    _box(ax, 6.1, 5.55, 3.35, 0.95, "PostgreSQL", "app.* · user data", C_DB)
    _box(ax, 3.1, 3.85, 3.8, 0.9, "RabbitMQ", "AMQP · exchange estimate.calc", C_QUEUE)
    _box(ax, 2.85, 2.15, 4.3, 0.95, "calc worker", "Go · goroutines · RunRabbit", C_WORKER)
    _box(ax, 2.35, 0.55, 5.3, 0.95, "PostgreSQL GSN", "gsn.* · fgis_cs.* · pg_trgm", C_DB)

    # Vertical flow
    _arrow(ax, 5.0, 11.0, 5.0, 10.45, "HTTP · JWT cookie")
    _arrow(ax, 5.0, 9.55, 2.2, 8.6, "auth API", rad=-0.12)
    _arrow(ax, 5.0, 9.55, 7.8, 8.6, "app API + static", rad=0.12)
    _arrow(ax, 2.2, 7.55, 2.2, 6.5)
    _arrow(ax, 7.8, 7.55, 7.8, 6.5, "write outbox (TX)")
    _arrow(ax, 6.4, 7.55, 5.0, 4.75, "outbox publisher", rad=0.22)
    _arrow(ax, 5.0, 3.85, 5.0, 3.1)
    _arrow(ax, 5.0, 2.15, 5.0, 1.5, "GetRecordDetail")
    _arrow(ax, 6.9, 3.1, 7.8, 5.55, "calc results", rad=-0.14)

    _legend(ax, 0.45, 0.08)

    output_path.parent.mkdir(parents=True, exist_ok=True)
    fig.savefig(output_path, dpi=160, bbox_inches="tight", facecolor=fig.get_facecolor(), pad_inches=0.15)
    plt.close(fig)
    return output_path
