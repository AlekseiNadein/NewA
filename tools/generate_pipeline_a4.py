from pathlib import Path

from reportlab.lib.colors import Color, HexColor
from reportlab.lib.pagesizes import A4, landscape
from reportlab.pdfbase import pdfmetrics
from reportlab.pdfbase.ttfonts import TTFont
from reportlab.pdfgen import canvas


ROOT = Path(__file__).resolve().parents[1]
OUTPUT = ROOT / "output" / "pdf" / "newa-pipeline-a4.pdf"

PAGE_W, PAGE_H = landscape(A4)

FONT_REGULAR = "Arial"
FONT_BOLD = "Arial-Bold"
pdfmetrics.registerFont(TTFont(FONT_REGULAR, r"C:\Windows\Fonts\arial.ttf"))
pdfmetrics.registerFont(TTFont(FONT_BOLD, r"C:\Windows\Fonts\arialbd.ttf"))

INK = HexColor("#172033")
MUTED = HexColor("#5C667A")
GRID = HexColor("#D7DDEA")
PAPER = HexColor("#FFFFFF")

DEV = HexColor("#E8F0FF")
DEV_ACCENT = HexColor("#3B6FD8")
GITLAB = HexColor("#FFF0E7")
GITLAB_ACCENT = HexColor("#D9652B")
ARTIFACT = HexColor("#EEE9FF")
ARTIFACT_ACCENT = HexColor("#7558C7")
K3S = HexColor("#E5F7F1")
K3S_ACCENT = HexColor("#21876B")
SUCCESS = HexColor("#DFF4E7")
SUCCESS_ACCENT = HexColor("#23824D")
FAILURE = HexColor("#FCE6E6")
FAILURE_ACCENT = HexColor("#C44747")


def split_text(text: str, font: str, size: float, width: float) -> list[str]:
    words = text.split()
    lines: list[str] = []
    current = ""
    for word in words:
        candidate = word if not current else f"{current} {word}"
        if pdfmetrics.stringWidth(candidate, font, size) <= width:
            current = candidate
        else:
            if current:
                lines.append(current)
            current = word
    if current:
        lines.append(current)
    return lines


def draw_centered_lines(
    c: canvas.Canvas,
    lines: list[str],
    center_x: float,
    top_y: float,
    font: str,
    size: float,
    color: Color,
    leading: float,
) -> float:
    c.setFillColor(color)
    c.setFont(font, size)
    y = top_y
    for line in lines:
        c.drawCentredString(center_x, y, line)
        y -= leading
    return y


def rounded_box(
    c: canvas.Canvas,
    x: float,
    y: float,
    w: float,
    h: float,
    fill: Color,
    accent: Color,
    number: str,
    owner: str,
    title: str,
    details: list[str],
) -> None:
    c.setFillColor(fill)
    c.setStrokeColor(GRID)
    c.setLineWidth(0.8)
    c.roundRect(x, y, w, h, 10, fill=1, stroke=1)

    badge_x = x + 18
    badge_y = y + h - 18
    c.setFillColor(accent)
    c.circle(badge_x, badge_y, 11, fill=1, stroke=0)
    c.setFillColor(PAPER)
    c.setFont(FONT_BOLD, 9)
    c.drawCentredString(badge_x, badge_y - 3.2, number)

    c.setFillColor(accent)
    c.setFont(FONT_BOLD, 7.5)
    c.drawRightString(x + w - 10, y + h - 20, owner.upper())

    title_lines = split_text(title, FONT_BOLD, 10.5, w - 22)
    text_y = draw_centered_lines(
        c,
        title_lines,
        x + w / 2,
        y + h - 46,
        FONT_BOLD,
        10.5,
        INK,
        12,
    )

    c.setStrokeColor(Color(accent.red, accent.green, accent.blue, alpha=0.28))
    c.line(x + 11, text_y + 3, x + w - 11, text_y + 3)

    c.setFillColor(MUTED)
    c.setFont(FONT_REGULAR, 7.8)
    detail_y = text_y - 9
    for detail in details:
        wrapped = split_text(detail, FONT_REGULAR, 7.8, w - 24)
        for index, line in enumerate(wrapped):
            prefix = "• " if index == 0 else "  "
            c.drawString(x + 12, detail_y, prefix + line)
            detail_y -= 9.4
        detail_y -= 2


def arrow(c: canvas.Canvas, x1: float, y1: float, x2: float, y2: float, color: Color = MUTED) -> None:
    c.setStrokeColor(color)
    c.setFillColor(color)
    c.setLineWidth(1.6)
    c.line(x1, y1, x2 - 8, y2)
    c.line(x2 - 8, y2, x2 - 13, y2 + 4)
    c.line(x2 - 8, y2, x2 - 13, y2 - 4)


def downward_arrow(c: canvas.Canvas, x: float, y1: float, y2: float, color: Color = MUTED) -> None:
    c.setStrokeColor(color)
    c.setFillColor(color)
    c.setLineWidth(1.6)
    c.line(x, y1, x, y2 + 8)
    c.line(x, y2 + 8, x - 4, y2 + 13)
    c.line(x, y2 + 8, x + 4, y2 + 13)


def left_arrow(c: canvas.Canvas, x1: float, y: float, x2: float, color: Color = MUTED) -> None:
    c.setStrokeColor(color)
    c.setFillColor(color)
    c.setLineWidth(1.6)
    c.line(x1, y, x2 + 8, y)
    c.line(x2 + 8, y, x2 + 13, y + 4)
    c.line(x2 + 8, y, x2 + 13, y - 4)


def pill(c: canvas.Canvas, x: float, y: float, w: float, text: str, fill: Color, accent: Color) -> None:
    c.setFillColor(fill)
    c.setStrokeColor(accent)
    c.setLineWidth(0.8)
    c.roundRect(x, y, w, 24, 12, fill=1, stroke=1)
    c.setFillColor(INK)
    c.setFont(FONT_BOLD, 8)
    c.drawCentredString(x + w / 2, y + 8.3, text)


def generate() -> None:
    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    c = canvas.Canvas(str(OUTPUT), pagesize=(PAGE_W, PAGE_H))
    c.setTitle("NewA - этапы CI/CD")
    c.setAuthor("NewA project")

    margin_x = 28
    c.setFillColor(INK)
    c.setFont(FONT_BOLD, 20)
    c.drawString(margin_x, PAGE_H - 34, "NewA: путь изменения от кода до стабильного Staging")
    c.setFillColor(MUTED)
    c.setFont(FONT_REGULAR, 9)
    c.drawRightString(PAGE_W - margin_x, PAGE_H - 31, "Целевая схема внедрения CI/CD")

    c.setStrokeColor(GRID)
    c.setLineWidth(0.8)
    c.line(margin_x, PAGE_H - 48, PAGE_W - margin_x, PAGE_H - 48)

    box_w = 178
    box_h = 127
    gap_x = 23
    left = 31
    row1_y = 330
    row2_y = 164

    row1 = [
        (
            DEV,
            DEV_ACCENT,
            "1",
            "Dev",
            "Изменение кода",
            ["Ветка feature/* или fix/*", "Изменены сервисы, тесты и конфигурация"],
        ),
        (
            DEV,
            DEV_ACCENT,
            "2",
            "Dev",
            "Локальная проверка",
            ["Подняты нужные компоненты", "Build, unit и targeted integration tests"],
        ),
        (
            GITLAB,
            GITLAB_ACCENT,
            "3",
            "GitLab",
            "Push и Merge Request",
            ["Код пушится в ветку, не в Staging", "Review, approvals, protected main"],
        ),
        (
            GITLAB,
            GITLAB_ACCENT,
            "4",
            "Runner",
            "CI quality gates",
            ["Validate, test, contract и migration checks", "Security scan блокирует merge по policy"],
        ),
    ]

    row2 = [
        (
            ARTIFACT,
            ARTIFACT_ACCENT,
            "5",
            "Registry",
            "Build и публикация",
            ["Только затронутые сервисы и dependents", "Immutable tag, digest, SBOM, pipeline ID"],
        ),
        (
            K3S,
            K3S_ACCENT,
            "6",
            "Agent + k3s",
            "Deploy в Staging",
            ["Helm через GitLab Agent", "Namespace RBAC и resource_group"],
        ),
        (
            K3S,
            K3S_ACCENT,
            "7",
            "Kubernetes",
            "RollingUpdate",
            ["Новые Pods проходят startup/readiness", "Старые Pods удаляются после готовности новых"],
        ),
        (
            SUCCESS,
            SUCCESS_ACCENT,
            "8",
            "Verify",
            "Проверка результата",
            ["Rollout, smoke, integration и SLO signals", "Успех: зафиксировать last known good"],
        ),
    ]

    for index, data in enumerate(row1):
        x = left + index * (box_w + gap_x)
        rounded_box(c, x, row1_y, box_w, box_h, *data)
        if index < len(row1) - 1:
            arrow(c, x + box_w + 3, row1_y + box_h / 2, x + box_w + gap_x - 3, row1_y + box_h / 2)

    downward_arrow(
        c,
        left + 3 * (box_w + gap_x) + box_w / 2,
        row1_y - 7,
        row2_y + box_h + 8,
    )

    # Second row goes right-to-left visually, then the content continues left-to-right
    # by numbering. Draw boxes in natural reading order with an explicit connector.
    for index, data in enumerate(row2):
        x = left + index * (box_w + gap_x)
        rounded_box(c, x, row2_y, box_w, box_h, *data)
        if index < len(row2) - 1:
            arrow(c, x + box_w + 3, row2_y + box_h / 2, x + box_w + gap_x - 3, row2_y + box_h / 2)

    c.setStrokeColor(MUTED)
    c.setLineWidth(1.6)
    top_end_x = left + 3 * (box_w + gap_x) + box_w / 2
    bridge_y = row2_y + box_h + 16
    c.line(top_end_x, row1_y - 7, top_end_x, bridge_y)
    c.line(top_end_x, bridge_y, left - 11, bridge_y)
    c.line(left - 11, bridge_y, left - 11, row2_y + box_h / 2)
    arrow(c, left - 11, row2_y + box_h / 2, left - 2, row2_y + box_h / 2)

    failure_y = 115
    end_x = left + 3 * (box_w + gap_x) + box_w / 2

    branch_y = failure_y + 38
    c.setStrokeColor(FAILURE_ACCENT)
    c.setLineWidth(1.6)
    c.line(end_x - 35, row2_y - 7, end_x - 35, branch_y)
    left_arrow(c, end_x - 35, branch_y, 510, FAILURE_ACCENT)
    c.setFillColor(FAILURE_ACCENT)
    c.setFont(FONT_BOLD, 7.5)
    c.drawCentredString(584, branch_y + 6, "ОШИБКА")

    pill(c, 342, failure_y, 160, "Rollback last-known-good", FAILURE, FAILURE_ACCENT)
    pill(c, 525, failure_y, 142, "Повторная проверка", DEV, DEV_ACCENT)
    arrow(c, 505, failure_y + 12, 522, failure_y + 12, FAILURE_ACCENT)
    arrow(c, 670, failure_y + 12, 686, failure_y + 12, SUCCESS_ACCENT)

    c.setFillColor(SUCCESS_ACCENT)
    c.setFont(FONT_BOLD, 7.5)
    c.drawCentredString(end_x + 41, row2_y - 19, "УСПЕХ")
    pill(c, 689, failure_y, 122, "Новая stable", SUCCESS, SUCCESS_ACCENT)
    downward_arrow(c, end_x + 41, row2_y - 7, failure_y + 24, SUCCESS_ACCENT)

    c.setFillColor(INK)
    c.setFont(FONT_BOLD, 8.5)
    c.drawString(margin_x, 80, "Обязательные принципы")
    principles = [
        "Build once - один и тот же digest проходит среды без пересборки.",
        "Staging - environment, а не Git-ветка.",
        "Deploy не останавливает старые Pods до готовности новых.",
        "Rollback проверяется теми же health и smoke checks.",
        "Версия N-1 должна быть совместима со схемой БД после миграции.",
    ]
    c.setFont(FONT_REGULAR, 7.6)
    c.setFillColor(MUTED)
    column_w = (PAGE_W - 2 * margin_x - 18) / 2
    for index, text in enumerate(principles):
        col = index % 2
        row = index // 2
        x = margin_x + col * (column_w + 18)
        y = 65 - row * 12
        c.setFillColor(K3S_ACCENT if index % 2 == 0 else ARTIFACT_ACCENT)
        c.circle(x + 3, y + 2, 2.2, fill=1, stroke=0)
        c.setFillColor(MUTED)
        c.drawString(x + 10, y, text)

    c.setStrokeColor(GRID)
    c.line(margin_x, 27, PAGE_W - margin_x, 27)
    c.setFillColor(MUTED)
    c.setFont(FONT_REGULAR, 6.8)
    c.drawString(margin_x, 16, "Источник истины: commit + image digest + Helm/Kubernetes revision + результаты verify")
    c.drawRightString(PAGE_W - margin_x, 16, "Формат: A4, альбомная ориентация")

    c.showPage()
    c.save()
    print(OUTPUT)


if __name__ == "__main__":
    generate()
