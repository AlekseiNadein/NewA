import fs from "fs";

const ESTIMATE_TEXT_DELIMITER = "'";

function unescapeEstimateTextField(value) {
  return String(value ?? "")
    .replace(/\\n/g, "\n")
    .replace(/\\\\/g, "\\");
}

function splitEstimateTextLine(line) {
  return String(line || "").split(ESTIMATE_TEXT_DELIMITER);
}

function splitSourceDataStrictFields(line) {
  return String(line || "").split(ESTIMATE_TEXT_DELIMITER);
}

function extractSourceDataPositionCipher(firstField) {
  const raw = String(firstField || "").trim();
  if (!raw) {
    return "";
  }
  let cutAt = raw.length;
  for (const separator of ["(", " ", "#"]) {
    const index = raw.indexOf(separator);
    if (index >= 0 && index < cutAt) {
      cutAt = index;
    }
  }
  return raw.slice(0, cutAt).trim();
}

function parseEstimateTextNumber(value) {
  const normalized = String(value ?? "")
    .trim()
    .replace(/\s/g, "")
    .replace(",", ".");
  if (!normalized) return 0;
  const number = Number(normalized);
  return Number.isFinite(number) ? number : 0;
}

function trimSourceDataRecordLine(line, lineNumber) {
  const trimmed = String(line || "").trimEnd();
  if (!trimmed.endsWith("*")) {
    throw new Error(`Line ${lineNumber}: must end with *`);
  }
  return trimmed.slice(0, -1);
}

function parseSourceDataConstructionLine(line) {
  const fields = splitSourceDataStrictFields(line.slice(1));
  return {
    code: fields[8] || "",
    title: fields[9] || "",
  };
}

function isSourceDataSkippedLine(line) {
  if (line === "К") return true;
  if (line.startsWith("К'")) return true;
  return line.startsWith("F(");
}

function estimateLineIsResourcePosition(item) {
  const code = item.code || "";
  return /^[СCМMТT]\d/.test(code);
}

function estimateLineIsWorkPosition(item) {
  if (item.type !== "position") return false;
  if (estimateLineIsResourcePosition(item)) return false;
  return /^[ЕУEУЦ]/.test(item.code || "");
}

function estimateItemsForApi(items) {
  return (items || []).map((item) => {
    if (item.source === "gsn") {
      return {
        id: item.id || "",
        type: item.type || "position",
        source: "gsn",
        code: item.code || "",
        quantity: Number(item.quantity || 0),
      };
    }
    return {
      id: item.id || "",
      type: item.type || "position",
      source: item.source || "",
      code: item.code || "",
      name: item.name || "",
      quantity: Number(item.quantity || 0),
      unit: item.unit || "",
      unitPrice: Number(item.unitPrice || 0),
      total: Number(item.total || 0),
    };
  });
}

function validateBackendItem(item) {
  const type = (item.type || "position").toLowerCase();
  if (!["section", "subsection", "position"].includes(type)) {
    return `invalid type: ${type}`;
  }
  const source = (item.source || "").toLowerCase();
  if (source === "gsn") {
    if (!String(item.code || "").trim()) return "gsn without code";
    return null;
  }
  if (!String(item.name || "").trim()) return `non-gsn without name (type=${type}, code=${item.code})`;
  return null;
}

const text = fs.readFileSync("Э10410.txt", "utf8");
const lines = text.split(/\r?\n/).map((l) => l.trimEnd()).filter(Boolean);
const construction = parseSourceDataConstructionLine(trimSourceDataRecordLine(lines[1], 2));
const items = [];

for (let lineIndex = 3; lineIndex < lines.length; lineIndex += 1) {
  const rawLine = lines[lineIndex].trimEnd();
  if (!rawLine) continue;
  const line = trimSourceDataRecordLine(rawLine, lineIndex + 1);
  if (isSourceDataSkippedLine(line)) continue;

  if (line.startsWith("ПР")) {
    items.push({ id: `line_${lineIndex}`, type: "subsection", source: "", code: "", name: unescapeEstimateTextField(line.slice(2)) });
    continue;
  }
  if (line.startsWith("Р")) {
    items.push({ id: `line_${lineIndex}`, type: "section", source: "", code: "", name: unescapeEstimateTextField(line.slice(1)) });
    continue;
  }

  const fields = splitEstimateTextLine(line).map(unescapeEstimateTextField);
  if (fields.length < 2) throw new Error(`line ${lineIndex + 1}: too few fields`);
  const [itemCode, quantityRaw, totalRaw = "", name = "", unit = ""] = fields;
  const quantityValue = 0;
  let totalValue = parseEstimateTextNumber(totalRaw);
  const normalizedCode = extractSourceDataPositionCipher(itemCode);
  let unitPriceValue = 0;
  if (quantityValue && totalValue) unitPriceValue = totalValue / quantityValue;

  const item = {
    id: `line_${lineIndex}`,
    type: "position",
    source: "",
    code: normalizedCode,
    name: name || "",
    unit: unit || "",
    quantity: quantityValue,
    unitPrice: unitPriceValue,
    total: totalValue,
  };
  if (estimateLineIsWorkPosition(item) || estimateLineIsResourcePosition(item)) {
    // not setting gsn source in parser
  }
  items.push(item);
}

console.log("construction:", construction);
const apiItems = estimateItemsForApi(items);
const errors = apiItems.map((item, i) => ({ i, err: validateBackendItem(item), item })).filter((x) => x.err);
console.log("items:", apiItems.length, "errors:", errors.length);
for (const e of errors.slice(0, 20)) {
  console.log(e.i, e.err, JSON.stringify(e.item));
}
