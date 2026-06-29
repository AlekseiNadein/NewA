// Quick check for GSN partial display logic (mirrors web/app.js helpers).

function estimateLineIsGsn(item) {
  return String(item?.source || "").toLowerCase() === "gsn";
}

function estimateLineIsStructural(item) {
  return item?.type === "section" || item?.type === "subsection";
}

function estimateLineCalcDone(item) {
  return String(item?.calcStatus || "").trim() === "done";
}

function estimateLineSourceCode(item) {
  return String(item?.sourceCode || item?.recordCode || item?.code || "").trim();
}

function estimateLineShowsGsnPartial(item) {
  if (!estimateLineIsGsn(item) || estimateLineIsStructural(item)) {
    return false;
  }
  if (estimateLineCalcDone(item)) {
    return false;
  }
  const status = String(item?.calcStatus || "").trim();
  if (status) {
    return status !== "done";
  }
  return !String(item?.name || "").trim() && !item.children?.length;
}

function estimateLineGsnOriginalCode(item) {
  const fromCalc = String(item?.calcJson?.record?.originalCode || "").trim();
  if (fromCalc) {
    return fromCalc;
  }
  const fromItem = String(item?.originalCode || "").trim();
  if (fromItem) {
    return fromItem;
  }
  return String(item?.code || "").trim();
}

function estimateLineDisplayCode(item) {
  if (estimateLineShowsGsnPartial(item)) {
    return estimateLineSourceCode(item);
  }
  if (estimateLineIsGsn(item) && estimateLineCalcDone(item)) {
    return estimateLineGsnOriginalCode(item);
  }
  return String(item?.originalCode || item?.code || "").trim();
}

const cases = [
  {
    label: "text parse before calc",
    item: { type: "position", source: "gsn", code: "Е0801-002-02", quantity: 61.475 },
    expectPartial: true,
  },
  {
    label: "queued",
    item: { type: "position", source: "gsn", code: "Е0801-002-02", quantity: 61.475, calcStatus: "queued" },
    expectPartial: true,
  },
  {
    label: "done with enrichment",
    item: {
      type: "position",
      source: "gsn",
      code: "Е0801-002-02",
      originalCode: "Е0801-002-02 (РМ11762РМ6141)",
      name: "Устройство основания",
      unit: "м3",
      quantity: 61.475,
      calcStatus: "done",
    },
    expectPartial: false,
  },
  {
    label: "catalog pick",
    item: {
      type: "position",
      source: "gsn",
      code: "Е0801-002-02",
      name: "Устройство основания",
      unit: "м3",
      quantity: 1,
      children: [{ code: "С1084", name: "Сталь" }],
    },
    expectPartial: false,
  },
  {
    label: "partial shows source cipher not original",
    item: {
      type: "position",
      source: "gsn",
      sourceCode: "Е0624-001-05",
      code: "Е0624-001-05",
      originalCode: "Е0624-001-05",
      quantity: 5.76,
      calcStatus: "queued",
    },
    expectDisplay: "Е0624-001-05",
  },
  {
    label: "done shows GSN original cipher",
    item: {
      type: "position",
      source: "gsn",
      sourceCode: "Е0624-001-05",
      code: "Е0624-001-05",
      originalCode: "06-24-001-05 ГЭСН 81-02-06-2022 Минстрой РФ пр. № 91/пр",
      quantity: 5.76,
      calcStatus: "done",
    },
    expectDisplay: "06-24-001-05 ГЭСН 81-02-06-2022 Минстрой РФ пр. № 91/пр",
  },
];

let failed = 0;
for (const testCase of cases) {
  if (testCase.expectDisplay != null) {
    const actual = estimateLineDisplayCode(testCase.item);
    const ok = actual === testCase.expectDisplay;
    console.log(`${ok ? "OK" : "FAIL"} ${testCase.label}: display=${JSON.stringify(actual)}`);
    if (!ok) {
      failed += 1;
    }
    continue;
  }
  const actual = estimateLineShowsGsnPartial(testCase.item);
  const ok = actual === testCase.expectPartial;
  console.log(`${ok ? "OK" : "FAIL"} ${testCase.label}: partial=${actual}`);
  if (!ok) {
    failed += 1;
  }
}
process.exit(failed ? 1 : 0);
