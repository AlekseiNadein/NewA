const LOGIN_DRAFT_KEY = "nav_login_draft";

function readLoginDraft(storageKey) {
  try {
    const raw = localStorage.getItem(storageKey);
    if (!raw) {
      return null;
    }
    const parsed = JSON.parse(raw);
    if (!parsed || typeof parsed !== "object") {
      return null;
    }
    return {
      companyName: String(parsed.companyName || ""),
      name: String(parsed.name || ""),
    };
  } catch {
    return null;
  }
}

function saveLoginDraft(storageKey, data) {
  localStorage.setItem(
    storageKey,
    JSON.stringify({
      companyName: String(data.companyName || ""),
      name: String(data.name || ""),
    }),
  );
}

function applyLoginDraft(form, storageKey) {
  const draft = readLoginDraft(storageKey);
  const target = form || document.querySelector("#loginForm");
  if (!draft || !target) {
    return;
  }
  const companyInput = target.querySelector('[name="companyName"]');
  const nameInput = target.querySelector('[name="name"]');
  if (companyInput) {
    companyInput.value = draft.companyName;
  }
  if (nameInput) {
    nameInput.value = draft.name;
  }
}

function setupLoginDraftAutosave(form, storageKey) {
  const target = form || document.querySelector("#loginForm");
  if (!target || target.dataset.draftAutosave === "true") {
    return;
  }
  target.dataset.draftAutosave = "true";

  let timer = 0;
  const save = () => {
    window.clearTimeout(timer);
    timer = window.setTimeout(() => {
      saveLoginDraft(storageKey, formData(target));
    }, 250);
  };

  target.addEventListener("input", save);
  target.addEventListener("change", save);
}

function bootstrapLoginForm() {
  setupLoginDraftAutosave(els.loginForm, LOGIN_DRAFT_KEY);
  applyLoginDraft(els.loginForm, LOGIN_DRAFT_KEY);
  window.requestAnimationFrame(() => {
    applyLoginDraft(els.loginForm, LOGIN_DRAFT_KEY);
  });
}

function formData(form) {
  return Object.fromEntries(new FormData(form).entries());
}

function normalizeEstimateId(value) {
  return String(value || "").trim();
}

function sameEstimateId(left, right) {
  const a = normalizeEstimateId(left);
  const b = normalizeEstimateId(right);
  return Boolean(a) && a === b;
}

function estimateCalcTargetKey(estimateId) {
  return normalizeEstimateId(estimateId) || String(estimateId || "");
}

function readSessionJSONArray(key, fallback = []) {
  try {
    const raw = sessionStorage.getItem(key);
    if (!raw) {
      return fallback;
    }
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed : fallback;
  } catch {
    return fallback;
  }
}

function readSessionJSONObject(key, fallback = {}) {
  try {
    const raw = sessionStorage.getItem(key);
    if (!raw) {
      return fallback;
    }
    const parsed = JSON.parse(raw);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      return fallback;
    }
    return parsed;
  } catch {
    return fallback;
  }
}

function hydrateOpenEstimatesFromSession() {
  return readSessionJSONArray("nav_editor_estimates", [])
    .filter((estimate) => estimate && typeof estimate === "object" && !Array.isArray(estimate))
    .map((estimate) => {
      try {
        normalizeEstimateSourceDataFields(estimate);
        hydrateEstimateSourceDataFgisSet(estimate);
        const items = Array.isArray(estimate.items) ? estimate.items : [];
        items.forEach(hydrateEstimateItemFromSourceDataRawText);
      } catch {
        // Повреждённый снимок сметы в sessionStorage не должен ломать старт приложения.
      }
      return estimate;
    });
}

function repairEditorSessionStorage() {
  const entries = [
    ["nav_editor_estimates", []],
    ["nav_editor_buffer", []],
    ["nav_editor_expanded", {}],
  ];
  for (const [key, fallback] of entries) {
    try {
      const raw = sessionStorage.getItem(key);
      if (!raw) {
        continue;
      }
      if (raw.length > 4_000_000) {
        sessionStorage.setItem(key, JSON.stringify(fallback));
        continue;
      }
      const parsed = JSON.parse(raw);
      const valid = Array.isArray(fallback)
        ? Array.isArray(parsed)
        : parsed && typeof parsed === "object" && !Array.isArray(parsed);
      if (!valid) {
        sessionStorage.setItem(key, JSON.stringify(fallback));
      }
    } catch {
      sessionStorage.setItem(key, JSON.stringify(fallback));
    }
  }
}

repairEditorSessionStorage();

const state = {
  me: null,
  authScreen: "login",
  section: "base",
  baseTab: "gsn",
  companies: [],
  constructions: [],
  objects: [],
  estimates: [],
  gsnLoaded: false,
  gsnSupplementsLoaded: false,
  gsnSupplements: [],
  gsnSupplement: localStorage.getItem("nav_gsn_supplement") || "",
  gsnBaseInfoLoaded: false,
  gsnBaseInfo: null,
  gsnSearch: "",
  gsnSearchDraft: "",
  gsnSearchLoading: false,
  gsnRegions: [],
  gsnRegionsLoaded: false,
  fgisSets: [],
  fgisSetsLoaded: false,
  fgisSetId: localStorage.getItem("nav_fgis_set") || "",
  fgisRows: [],
  fgisStats: null,
  fgisFilteredTotal: 0,
  fgisLimit: 100,
  fgisOffset: 0,
  fgisSearch: "",
  fgisSearchDraft: "",
  fgisLoadedSetId: "",
  fgisLoading: false,
  districtPickerRegionCode: "",
  showHierarchyCode: localStorage.getItem("nav_show_hierarchy_code") === "true",
  editorTab: sessionStorage.getItem("nav_editor_tab") || "buffer",
  estimateViewMode: "text",
  estimateTextDrafts: {},
  editorRenderToken: null,
  buffer: readSessionJSONArray("nav_editor_buffer", []),
  openEstimates: hydrateOpenEstimatesFromSession(),
  estimateLinesExpanded: readSessionJSONObject("nav_editor_expanded", {}),
  constructionExpanded: {},
  objectExpanded: {},
  estimateLocks: {},
  userPositions: [],
  addPrimitiveQuantities: {},
  addPrimitiveSources: {},
  addLineTargetEstimateId: null,
  licenseBlocked: [],
  licenseHeld: [],
  appSettings: null,
  appSettingsLoaded: false,
};

let userPositionDialogEditId = "";
let userPositionDialogSaving = false;
const persistEstimateInflight = new Map();
const estimateCalcPollBlockedByPersist = new Set();
const ESTIMATE_LOCK_POLL_MS = 5000;
const ESTIMATE_LOCK_HEARTBEAT_MS = 30000;
const LICENSE_SESSION_HEARTBEAT_MS = 30000;
const ESTIMATE_CALC_STATUS_POLL_MS = 800;
const ESTIMATE_CALC_APPLY_CHUNK = 80;
let estimateLockPollTimer = 0;
let estimateLockHeartbeatTimer = 0;
let licenseSessionHeartbeatTimer = 0;
let estimateCalcStatusPollTimer = 0;
let estimateCalcStatusPollInflight = null;
let pendingEditorRemountEstimateId = null;
let editorOpeningEstimateId = null;
let estimateCalcAwaitingServer = new Set();
const estimateCalcProgressTargets = new Map();
let estimateCalcApplyToken = 0;
let estimateCalcProgressAnimFrame = 0;
let editorTableInteractionEnabled = false;
let editorTableBodyReady = false;
let editorTableBodyRenderToken = 0;
const EDITOR_TABLE_IMMEDIATE_ROWS = 60;
const EDITOR_TABLE_CHUNK_ROWS = 50;
const LARGE_ESTIMATE_CALC_LINE_THRESHOLD = 250;
const LARGE_ESTIMATE_CALC_STATUS_POLL_MS = 2000;
const LARGE_ESTIMATE_CALC_APPLY_CHUNK = 120;
let loadAppRequestId = 0;
let loadAppSuppressMissingSession = true;
let licenseGateVersion = 0;

const money = new Intl.NumberFormat("ru-RU", {
  style: "currency",
  currency: "RUB",
  maximumFractionDigits: 0,
});

const estimateCost = new Intl.NumberFormat("ru-RU", {
  maximumFractionDigits: 0,
});

const estimateCostDetailed = new Intl.NumberFormat("ru-RU", {
  minimumFractionDigits: 0,
  maximumFractionDigits: 2,
});

const els = {
  loginView: document.querySelector("#loginView"),
  registerView: document.querySelector("#registerView"),
  appView: document.querySelector("#appView"),
  logoutButton: document.querySelector("#logoutButton"),
  loginForm: document.querySelector("#loginForm"),
  registerForm: document.querySelector("#registerForm"),
  showRegisterButton: document.querySelector("#showRegisterButton"),
  showLoginButton: document.querySelector("#showLoginButton"),
  currentUser: document.querySelector("#currentUser"),
  currentRole: document.querySelector("#currentRole"),
  navLinks: document.querySelectorAll("[data-section]"),
  baseSubitems: document.querySelector("[data-subitems='base']"),
  baseSubLinks: document.querySelectorAll("[data-base-tab]"),
  baseSection: document.querySelector("#baseSection"),
  constructionsSection: document.querySelector("#constructionsSection"),
  editorSection: document.querySelector("#editorSection"),
  settingsSection: document.querySelector("#settingsSection"),
  editorSubitems: document.querySelector("#editorSubitems"),
  editorEyebrow: document.querySelector("#editorEyebrow"),
  editorEstimateViewModes: document.querySelector("#editorEstimateViewModes"),
  editorTitle: document.querySelector("#editorTitle"),
  editorDescription: document.querySelector("#editorDescription"),
  editorLicenseBlock: document.querySelector("#editorLicenseBlock"),
  editorSectionBody: document.querySelector("#editorSectionBody"),
  editorContent: document.querySelector("#editorContent"),
  estimateLineDialog: document.querySelector("#estimateLineDialog"),
  estimateLineDialogForm: document.querySelector("#estimateLineDialogForm"),
  estimateLineDialogTitle: document.querySelector("#estimateLineDialogTitle"),
  estimateLineDialogSubmit: document.querySelector("#estimateLineDialogSubmit"),
  estimateLineTypeLabel: document.querySelector("#estimateLineTypeLabel"),
  estimateAddLineDialog: document.querySelector("#estimateAddLineDialog"),
  estimateAddLineDialogForm: document.querySelector("#estimateAddLineDialogForm"),
  districtDialog: document.querySelector("#districtDialog"),
  districtDialogForm: document.querySelector("#districtDialogForm"),
  districtRegionFilter: document.querySelector("#districtRegionFilter"),
  districtRegionList: document.querySelector("#districtRegionList"),
  districtZoneSelect: document.querySelector("#districtZoneSelect"),
  baseTitle: document.querySelector("#baseTitle"),
  baseDescription: document.querySelector("#baseDescription"),
  baseLicenseBlock: document.querySelector("#baseLicenseBlock"),
  baseSectionBody: document.querySelector("#baseSectionBody"),
  gsnSupplementBar: document.querySelector("#gsnSupplementBar"),
  gsnSupplementSelect: document.querySelector("#gsnSupplementSelect"),
  gsnSearchForm: document.querySelector("#gsnSearchForm"),
  gsnSearchInput: document.querySelector("#gsnSearchInput"),
  gsnTree: document.querySelector("#gsnTree"),
  fgisPricesPanel: document.querySelector("#fgisPricesPanel"),
  userPositionsPanel: document.querySelector("#userPositionsPanel"),
  userPositionDialog: document.querySelector("#userPositionDialog"),
  userPositionDialogForm: document.querySelector("#userPositionDialogForm"),
  userPositionDialogTitle: document.querySelector("#userPositionDialogTitle"),
  showHierarchyCodeToggle: document.querySelector("#showHierarchyCodeToggle"),
  appSettingsForm: document.querySelector("#appSettingsForm"),
  calcWorkerCountInput: document.querySelector("#calcWorkerCountInput"),
  saveAppSettingsButton: document.querySelector("#saveAppSettingsButton"),
  appSettingsHint: document.querySelector("#appSettingsHint"),
  constructionTree: document.querySelector("#constructionTree"),
  constructionDialog: document.querySelector("#constructionDialog"),
  constructionDialogForm: document.querySelector("#constructionDialogForm"),
  constructionDialogTitle: document.querySelector("#constructionDialogTitle"),
  message: document.querySelector("#message"),
};

els.loginForm?.addEventListener("submit", async (event) => {
  event.preventDefault();
  const credentials = formData(els.loginForm);
  saveLoginDraft(LOGIN_DRAFT_KEY, credentials);

  try {
    const result = await api("/api/auth/login", {
      method: "POST",
      body: credentials,
    });
    if (!result.access?.app) {
      await api("/api/auth/logout", { method: "POST" }).catch(() => {});
      throw new Error("Доступ к системе не открыт. Ожидайте подтверждения администратора");
    }

    loadAppRequestId += 1;
    const requestId = loadAppRequestId;
    state.me = result.user;
    localStorage.removeItem("nav_token");
    localStorage.removeItem("nav_admin_token");
    renderShell();
    showMessage("Вход выполнен", "ok");
    await bootstrapAppData(requestId);
  } catch (error) {
    applyLoginDraft(els.loginForm, LOGIN_DRAFT_KEY);
    showMessage(error.message, "error");
  }
});

bootstrapLoginForm();
void loadApp();

els.registerForm?.addEventListener("submit", async (event) => {
  event.preventDefault();

  const data = formData(els.registerForm);
  if (data.password !== data.passwordConfirm) {
    showMessage("Пароли не совпадают", "error");
    return;
  }

  try {
    const result = await api("/api/auth/register", {
      method: "POST",
      body: data,
    });
    els.registerForm.reset();
    showAuthScreen("login");
    showMessage(result.message || "Заявка отправлена", "ok");
  } catch (error) {
    showMessage(error.message, "error");
  }
});

els.showRegisterButton?.addEventListener("click", () => showAuthScreen("register"));
els.showLoginButton?.addEventListener("click", () => showAuthScreen("login"));

els.logoutButton?.addEventListener("click", async () => {
  await releaseAllEstimateLocks();
  stopEstimateLockSync();
  await releaseLicenseSessions();
  stopLicenseSessionSync();
  try {
    await api("/api/auth/logout", { method: "POST" });
  } catch {
    // ignore logout errors
  }
  state.me = null;
  state.estimateLocks = {};
  state.appSettings = null;
  state.appSettingsLoaded = false;
  localStorage.removeItem("nav_token");
  localStorage.removeItem("nav_admin_token");
  renderShell();
  applyLoginDraft(els.loginForm, LOGIN_DRAFT_KEY);
});

window.addEventListener("beforeunload", () => {
  if (!state.me) {
    return;
  }
  fetch("/api/license-sessions", {
    method: "DELETE",
    credentials: "include",
    keepalive: true,
  });
  for (const estimate of state.openEstimates) {
    if (!isPersistedEstimateId(estimate.id)) {
      continue;
    }
    fetch(`/api/estimates/${estimate.id}/lock`, {
      method: "DELETE",
      credentials: "include",
      keepalive: true,
    });
  }
});

els.navLinks.forEach((button) => {
  button.addEventListener("click", () => {
    state.section = button.dataset.section;
    renderSections();
  });
});

els.baseSubLinks.forEach((button) => {
  button.addEventListener("click", () => {
    state.section = "base";
    state.baseTab = button.dataset.baseTab;
    renderSections();
  });
});

els.fgisPricesPanel.addEventListener("change", (event) => {
  const select = event.target.closest("#fgisSetSelect");
  if (select) {
    changeFGISSet(select.value);
  }
});

els.fgisPricesPanel.addEventListener("submit", (event) => {
  const form = event.target.closest("#fgisSearchForm");
  if (!form) {
    return;
  }
  event.preventDefault();
  const input = form.querySelector("#fgisSearchInput");
  state.fgisSearch = String(input?.value || "").trim();
  state.fgisSearchDraft = state.fgisSearch;
  void loadFGISRows({ resetOffset: true });
});

els.fgisPricesPanel.addEventListener("click", (event) => {
  const resetButton = event.target.closest("[data-fgis-search-reset]");
  if (resetButton) {
    state.fgisSearch = "";
    state.fgisSearchDraft = "";
    void loadFGISRows({ resetOffset: true });
    return;
  }

  const pageButton = event.target.closest("[data-fgis-page]");
  if (pageButton) {
    changeFGISPage(pageButton.dataset.fgisPage);
  }
});

els.constructionTree.addEventListener("click", (event) => {
  const addButton = event.target.closest("[data-add]");
  if (addButton) {
    openConstructionDialog(addButton.dataset.add, addButton.dataset.parentId || "");
    return;
  }

  const editButton = event.target.closest("[data-edit]");
  if (editButton) {
    if (editButton.dataset.edit === "estimate") {
      void openEstimateInEditor(editButton.dataset.editId);
      return;
    }
    openConstructionDialog(editButton.dataset.edit, "", editButton.dataset.editId);
    return;
  }

  const deleteButton = event.target.closest("[data-delete]");
  if (deleteButton) {
    void deleteConstructionEntity(deleteButton.dataset.delete, deleteButton.dataset.deleteId);
    return;
  }

  const constructionToggle = event.target.closest("[data-toggle-construction]");
  if (constructionToggle) {
    const id = constructionToggle.dataset.toggleConstruction;
    state.constructionExpanded[id] = !state.constructionExpanded[id];
    renderConstructionTree();
    return;
  }

  const objectToggle = event.target.closest("[data-toggle-object]");
  if (objectToggle) {
    const id = objectToggle.dataset.toggleObject;
    state.objectExpanded[id] = !state.objectExpanded[id];
    renderConstructionTree();
  }
});

els.constructionDialogForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  const data = formData(els.constructionDialogForm);

  try {
    await saveConstructionDialog(data);
    els.constructionDialog.close();
    els.constructionDialogForm.reset();
    await refreshConstructionData();
    showMessage("Сохранено", "ok");
  } catch (error) {
    showMessage(error.message, "error");
  }
});

els.constructionDialogForm.querySelector("[data-dialog-cancel]").addEventListener("click", () => {
  els.constructionDialog.close();
  els.constructionDialogForm.reset();
});

els.showHierarchyCodeToggle.checked = state.showHierarchyCode;
els.showHierarchyCodeToggle.addEventListener("change", () => {
  state.showHierarchyCode = els.showHierarchyCodeToggle.checked;
  localStorage.setItem("nav_show_hierarchy_code", String(state.showHierarchyCode));
  if (state.gsnSearch) {
    void loadGSNSearch({ force: true });
  } else {
    loadGSNRoot({ force: true });
  }
  renderEditor();
});

els.appSettingsForm?.addEventListener("submit", async (event) => {
  event.preventDefault();
  if (!state.appSettings?.editable) {
    return;
  }
  const calcWorkerCount = Number(els.calcWorkerCountInput?.value || 0);
  try {
    const settings = await api("/api/settings", {
      method: "PUT",
      body: {
        calcWorkerCount,
      },
    });
    state.appSettings = settings;
    state.appSettingsLoaded = true;
    renderSettings();
    showMessage("Настройки сохранены", "ok");
  } catch (error) {
    showMessage(error.message || "Не удалось сохранить настройки", "error");
  }
});

els.gsnSearchForm.addEventListener("submit", (event) => {
  event.preventDefault();
  state.gsnSearch = String(els.gsnSearchInput?.value || "").trim();
  state.gsnSearchDraft = state.gsnSearch;
  renderGSNSearchForm();
  if (state.gsnSearch) {
    void loadGSNSearch({ force: true });
  } else {
    state.gsnLoaded = false;
    void loadGSNRoot({ force: true });
  }
});

els.gsnSupplementBar.addEventListener("click", (event) => {
  const resetButton = event.target.closest("[data-gsn-search-reset]");
  if (!resetButton) {
    return;
  }
  state.gsnSearch = "";
  state.gsnSearchDraft = "";
  if (els.gsnSearchInput) {
    els.gsnSearchInput.value = "";
  }
  renderGSNSearchForm();
  state.gsnLoaded = false;
  void loadGSNRoot({ force: true });
  void applyLicenseGate();
});

els.gsnSupplementSelect.addEventListener("change", () => {
  const nextSupplement = els.gsnSupplementSelect.value;
  if (!nextSupplement || nextSupplement === state.gsnSupplement) {
    return;
  }
  state.gsnSupplement = nextSupplement;
  localStorage.setItem("nav_gsn_supplement", nextSupplement);
  state.gsnLoaded = false;
  state.gsnBaseInfoLoaded = false;
  state.gsnSearch = "";
  state.gsnSearchDraft = "";
  if (els.gsnSearchInput) {
    els.gsnSearchInput.value = "";
  }
  renderGSNSearchForm();
  loadGSNBaseInfo();
  loadGSNRoot({ force: true });
  void applyLicenseGate();
});

els.gsnTree.addEventListener("click", async (event) => {
  if (handleAddPrimitiveClick(event, els.gsnTree, onAddPrimitiveQuantityChange)) {
    return;
  }

  const pdfNode = event.target.closest(".gsn-node-pdf");
  if (pdfNode && !event.target.closest("[data-gsn-toggle]")) {
    await openGSNPdf(pdfNode.dataset.gsnPdfCode, pdfNode.dataset.gsnPdfTitle);
    return;
  }

  const button = event.target.closest("[data-gsn-toggle]");
  if (!button) {
    return;
  }

  const code = button.dataset.gsnToggle;
  const node = button.closest(".gsn-node");
  const children = node.querySelector(":scope > .tree-children");

  if (button.dataset.loaded === "true") {
    const collapsed = children.classList.toggle("hidden");
    button.textContent = collapsed ? "+" : "-";
    return;
  }

  button.disabled = true;
  button.textContent = "...";
  try {
    const nodes = await fetchGSNChildren(code);
    children.innerHTML = nodes.length
      ? nodes.map(renderGSNNode).join("")
      : `<p class="muted">Нет дочерних элементов.</p>`;
    button.dataset.loaded = "true";
    button.textContent = "-";
    children.classList.remove("hidden");
  } catch (error) {
    showMessage(error.message, "error");
    button.textContent = "+";
  } finally {
    button.disabled = false;
  }
});

els.gsnTree.addEventListener("focusout", (event) => {
  handleAddPrimitiveQuantityBlur(event, els.gsnTree, onAddPrimitiveQuantityChange);
});

els.gsnTree.addEventListener("keydown", (event) => {
  handleAddPrimitiveQuantityKeydown(event, els.gsnTree, onAddPrimitiveQuantityChange);
});

els.editorSubitems.addEventListener("click", (event) => {
  if (event.target.closest("[data-editor-create-estimate]")) {
    void createEditorEstimate();
    return;
  }

  const button = event.target.closest("[data-editor-tab]");
  if (!button) {
    return;
  }

  const nextTab = button.dataset.editorTab;
  if (nextTab === state.editorTab) {
    return;
  }

  flushEstimateEditorFields(state.editorTab);
  saveEditorState();
  invalidateEstimateTextDraft(state.editorTab);

  state.editorTab = nextTab;
  if (state.editorTab !== "buffer") {
    state.estimateViewMode = "text";
  }
  sessionStorage.setItem("nav_editor_tab", state.editorTab);
  state.section = "editor";
  renderSections();
});

els.editorEstimateViewModes?.addEventListener("click", (event) => {
  const viewModeButton = event.target.closest("[data-editor-view-mode]");
  if (!viewModeButton) {
    return;
  }
  void setEstimateViewMode(viewModeButton.dataset.editorViewEstimate, viewModeButton.dataset.editorViewMode);
});

els.editorContent.addEventListener("click", (event) => {
  const addLineButton = event.target.closest("[data-editor-add-line]");
  if (addLineButton) {
    openEstimateAddLineDialog(addLineButton.dataset.editorAddLine);
    return;
  }

  const pasteBufferButton = event.target.closest("[data-editor-paste-buffer]");
  if (pasteBufferButton && !pasteBufferButton.disabled) {
    void addBufferToEstimate(pasteBufferButton.dataset.editorPasteBuffer);
    return;
  }

  const exportExcelButton = event.target.closest("[data-editor-export-excel]");
  if (exportExcelButton) {
    void exportEstimateToExcel(exportExcelButton.dataset.editorExportExcel);
    return;
  }

  const closeButton = event.target.closest("[data-editor-close-estimate]");
  if (closeButton) {
    closeEstimateFromEditor(closeButton.dataset.editorCloseEstimate);
    return;
  }

  const deleteEstimateButton = event.target.closest("[data-editor-delete-estimate]");
  if (deleteEstimateButton) {
    void deleteEstimateFromEditor(deleteEstimateButton.dataset.editorDeleteEstimate);
    return;
  }

  const districtButton = event.target.closest("[data-editor-district-open]");
  if (districtButton) {
    void openDistrictDialog(districtButton.dataset.editorDistrictOpen);
    return;
  }

  const expandButton = event.target.closest("[data-editor-line-expand]");
  if (expandButton) {
    void toggleEstimateLineExpand(expandButton.dataset.editorLineExpand, expandButton.dataset.editorLineId);
    return;
  }

  const editLineButton = event.target.closest("[data-editor-line-edit]");
  if (editLineButton) {
    openEstimateLineDialog(editLineButton.dataset.editorLineEdit, editLineButton.dataset.editorLineId);
    return;
  }

  const deleteLineButton = event.target.closest("[data-editor-line-delete]");
  if (deleteLineButton) {
    void deleteEstimateLine(deleteLineButton.dataset.editorLineDelete, deleteLineButton.dataset.editorLineId);
    return;
  }

  const upLineButton = event.target.closest("[data-editor-line-up]");
  if (upLineButton) {
    void moveEstimateLine(upLineButton.dataset.editorLineUp, upLineButton.dataset.editorLineId, -1);
    return;
  }

  const downLineButton = event.target.closest("[data-editor-line-down]");
  if (downLineButton) {
    void moveEstimateLine(downLineButton.dataset.editorLineDown, downLineButton.dataset.editorLineId, 1);
  }
});

els.editorContent.addEventListener("input", (event) => {
  const textarea = event.target.closest("[data-editor-estimate-text]");
  if (textarea) {
    state.estimateTextDrafts[textarea.dataset.editorEstimateText] = textarea.value;
  }
});

els.editorContent.addEventListener("change", (event) => {
  const codeInput = event.target.closest("[data-editor-estimate-code]");
  if (codeInput) {
    void updateEstimateHeaderField(codeInput.dataset.editorEstimateCode, "code", codeInput.value);
    return;
  }

  const titleInput = event.target.closest("[data-editor-estimate-title]");
  if (titleInput) {
    void updateEstimateHeaderField(titleInput.dataset.editorEstimateTitle, "title", titleInput.value);
    return;
  }

  const fgisSetSelect = event.target.closest("[data-editor-fgis-set]");
  if (fgisSetSelect) {
    void updateEstimateFgisSet(fgisSetSelect.dataset.editorFgisSet, fgisSetSelect.value);
  }
});

els.editorContent.addEventListener("focusout", (event) => {
  if (!event.target.closest(".editor-estimate-header")) {
    return;
  }
  window.setTimeout(() => {
    flushPendingEditorRemount();
  }, 0);
});

els.editorContent.addEventListener(
  "wheel",
  (event) => {
    if (event.target.closest(".editor-estimate-table-body-scroll")) {
      enableEditorTableInteraction();
    }
  },
  { passive: true, capture: true },
);

els.editorContent.addEventListener(
  "pointerdown",
  (event) => {
    if (event.target.closest(".editor-estimate-table-body-scroll")) {
      enableEditorTableInteraction();
    }
  },
  { capture: true },
);

els.estimateLineDialogForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  const data = formData(els.estimateLineDialogForm);
  if (data.lineId) {
    await updateEstimateLine(data.estimateId, data.lineId, data);
  } else if (data.lineType === "section" || data.lineType === "subsection") {
    await addEstimateLine(data.estimateId, data.lineType, data.name);
  }
  els.estimateLineDialog.close();
  els.estimateLineDialogForm.reset();
});

els.estimateLineDialogForm.querySelector("[data-dialog-cancel]").addEventListener("click", () => {
  els.estimateLineDialog.close();
  els.estimateLineDialogForm.reset();
});

els.estimateAddLineDialogForm.addEventListener("click", (event) => {
  const choiceButton = event.target.closest("[data-add-line-choice]");
  if (!choiceButton) {
    return;
  }

  const estimateID = els.estimateAddLineDialogForm.elements.estimateId.value;
  const choice = choiceButton.dataset.addLineChoice;
  els.estimateAddLineDialog.close();

  if (choice === "gsn") {
    navigateToBaseForAddLine(estimateID, "gsn");
    return;
  }
  if (choice === "userPosition") {
    navigateToBaseForAddLine(estimateID, "userPositions");
    return;
  }
  if (choice === "section" || choice === "subsection") {
    openEstimateStructuralLineDialog(estimateID, choice);
  }
});

els.estimateAddLineDialogForm.querySelector("[data-dialog-cancel]").addEventListener("click", () => {
  els.estimateAddLineDialog.close();
});

els.districtRegionFilter.addEventListener("input", () => {
  renderDistrictRegionList(els.districtRegionFilter.value);
});

els.districtRegionList.addEventListener("click", (event) => {
  const button = event.target.closest("[data-district-region]");
  if (!button) {
    return;
  }
  state.districtPickerRegionCode = button.dataset.districtRegion;
  els.districtZoneSelect.value = "";
  renderDistrictRegionList(els.districtRegionFilter.value);
  applyDistrictToEstimate(els.districtDialogForm.elements.estimateId.value);
});

els.districtDialogForm.addEventListener("submit", (event) => {
  event.preventDefault();
  void applyDistrictDialog();
});

els.districtDialogForm.querySelector("[data-dialog-cancel]").addEventListener("click", () => {
  els.districtDialog.close();
});

els.userPositionsPanel.addEventListener("click", (event) => {
  if (handleAddPrimitiveClick(event, els.userPositionsPanel, onAddPrimitiveQuantityChange)) {
    return;
  }

  const addButton = event.target.closest("[data-user-position-add]");
  if (addButton) {
    openUserPositionDialog();
    return;
  }

  const editButton = event.target.closest("[data-user-position-edit]");
  if (editButton) {
    openUserPositionDialog(editButton.dataset.userPositionEdit);
    return;
  }

  const deleteButton = event.target.closest("[data-user-position-delete]");
  if (deleteButton) {
    deleteUserPosition(deleteButton.dataset.userPositionDelete);
  }
});

els.userPositionsPanel.addEventListener("focusout", (event) => {
  handleAddPrimitiveQuantityBlur(event, els.userPositionsPanel, onAddPrimitiveQuantityChange);
});

els.userPositionsPanel.addEventListener("keydown", (event) => {
  handleAddPrimitiveQuantityKeydown(event, els.userPositionsPanel, onAddPrimitiveQuantityChange);
});

els.userPositionDialogForm.addEventListener("submit", (event) => {
  event.preventDefault();
  if (userPositionDialogSaving) {
    return;
  }

  userPositionDialogSaving = true;
  try {
    const data = formData(els.userPositionDialogForm);
    if (saveUserPositionDialog(data)) {
      userPositionDialogEditId = "";
      els.userPositionDialog.close();
      els.userPositionDialogForm.reset();
    }
  } finally {
    userPositionDialogSaving = false;
  }
});

els.userPositionDialogForm.querySelector("[data-dialog-cancel]").addEventListener("click", () => {
  userPositionDialogEditId = "";
  els.userPositionDialog.close();
  els.userPositionDialogForm.reset();
});

async function bootstrapAppData(existingRequestId) {
  const requestId = existingRequestId ?? ++loadAppRequestId;
  try {
    await Promise.all([refreshCompanies(), refreshConstructionData()]);
    if (requestId !== loadAppRequestId) {
      return;
    }
    await restoreOpenEstimateLocks();
    if (requestId !== loadAppRequestId) {
      return;
    }
    startEstimateLockSync();
    void loadGSNRegions().catch(() => {});
    void loadFGISSets().catch(() => {});
    loadUserPositions();
    renderShell();
  } catch (error) {
    if (requestId !== loadAppRequestId) {
      return;
    }
    showMessage(error?.message || "Не удалось загрузить данные приложения", "error");
    renderShell();
  }
}

async function loadApp() {
  const requestId = ++loadAppRequestId;
  const suppressMissingSession = loadAppSuppressMissingSession;
  loadAppSuppressMissingSession = false;
  try {
    const me = await api("/api/me");
    if (requestId !== loadAppRequestId) {
      return;
    }
    state.me = me.user;
    if (!me.access?.app && !me.user?.authorized) {
      throw new Error("Нет доступа к системе. Ожидайте подтверждения администратора");
    }

    await bootstrapAppData(requestId);
  } catch (error) {
    if (requestId !== loadAppRequestId) {
      return;
    }
    stopEstimateLockSync();
    const message = error?.message || "Не удалось загрузить приложение";
    const unauthorized = /HTTP 401|HTTP 403|Нет доступа/i.test(message);
    const hadSession = Boolean(state.me);
    if (unauthorized) {
      state.me = null;
      state.estimateLocks = {};
      state.appSettings = null;
      state.appSettingsLoaded = false;
      localStorage.removeItem("nav_token");
      localStorage.removeItem("nav_admin_token");
    }
    renderShell();
    applyLoginDraft(els.loginForm, LOGIN_DRAFT_KEY);
    if (unauthorized && hadSession) {
      showMessage("Сессия истекла, войдите снова", "error");
    } else if (!unauthorized) {
      const hideMissingSession = suppressMissingSession && /missing session/i.test(message);
      if (!hideMissingSession) {
        showMessage(message, "error");
      }
    }
  }
}

async function refreshCompanies() {
  const companies = await api("/api/companies");
  state.companies = Array.isArray(companies) ? companies : [];
}

async function refreshConstructionData() {
  const [constructions, objects, estimates] = await Promise.all([
    api("/api/constructions"),
    api("/api/objects"),
    api("/api/estimates"),
  ]);

  state.constructions = Array.isArray(constructions) ? constructions : [];
  state.objects = Array.isArray(objects) ? objects : [];
  state.estimates = Array.isArray(estimates) ? estimates : [];
  renderConstructionTree();
}

function showAuthScreen(screen) {
  state.authScreen = screen;
  els.loginView.classList.toggle("hidden", screen !== "login");
  els.registerView.classList.toggle("hidden", screen !== "register");
  if (screen === "login") {
    applyLoginDraft(els.loginForm, LOGIN_DRAFT_KEY);
  }
}

function userRoleLabel(user) {
  if (user.isSuperAdministrator) {
    return "суперадминистратор";
  }
  if (user.isAdministrator) {
    return "администратор";
  }
  return user.authorized ? "пользователь" : "ожидает подтверждения";
}

function renderShell() {
  const loggedIn = Boolean(state.me);
  els.loginView.classList.toggle("hidden", loggedIn || state.authScreen !== "login");
  els.registerView.classList.toggle("hidden", loggedIn || state.authScreen !== "register");
  els.appView.classList.toggle("hidden", !loggedIn);
  els.logoutButton.classList.toggle("hidden", !loggedIn);

  if (!loggedIn) {
    if (!state.authScreen) {
      showAuthScreen("login");
    } else {
      applyLoginDraft(els.loginForm, LOGIN_DRAFT_KEY);
    }
    return;
  }

  const companyName =
    state.companies.find((company) => company.id === state.me.companyId)?.name || state.me.companyId;
  els.currentUser.textContent = `${state.me.name} (${companyName})`;
  els.currentRole.textContent = userRoleLabel(state.me);
  renderSections();
  renderConstructionTree();
}

function renderSections() {
  const sectionMap = {
    base: els.baseSection,
    constructions: els.constructionsSection,
    editor: els.editorSection,
    settings: els.settingsSection,
  };

  Object.entries(sectionMap).forEach(([section, element]) => {
    element.classList.toggle("hidden", state.section !== section);
  });

  els.navLinks.forEach((button) => {
    button.classList.toggle("active", button.dataset.section === state.section);
  });

  els.baseSubitems.classList.toggle("hidden", state.section !== "base");
  els.baseSubLinks.forEach((button) => {
    button.classList.toggle("active", button.dataset.baseTab === state.baseTab);
  });
  els.editorSubitems.classList.toggle("hidden", state.section !== "editor");
  renderEditorSubitems();

  void applyLicenseGate();
}

function subsectionIdForGSNSupplement(code) {
  const normalized = String(code || "")
    .trim()
    .toLowerCase()
    .replace(/\s+/g, "");
  if (normalized === "доп.18") {
    return "gsn_supplement_18";
  }
  return null;
}

function subsectionIdForFGISSet(setId) {
  const id = String(setId || "").trim();
  if (id === "alrosa-2026-q2") {
    return "fgis_alrosa_q2_2026";
  }
  if (id === "rzd-2026-q1") {
    return "fgis_rzd_q1_2026";
  }
  const set = state.fgisSets.find((item) => item.id === id);
  if (!set) {
    return null;
  }
  const name = String(set.name || "").toLowerCase();
  if (name.includes("алроса")) {
    return "fgis_alrosa_q2_2026";
  }
  if (name.includes("ржд")) {
    return "fgis_rzd_q1_2026";
  }
  return null;
}

function getEditorTableLicenseSubsectionIdsForFgisSet(fgisSetId) {
  const ids = ["gsn_supplement_18"];
  const fgisSubsectionId = subsectionIdForFGISSet(fgisSetId || "");
  if (fgisSubsectionId && !ids.includes(fgisSubsectionId)) {
    ids.push(fgisSubsectionId);
  }
  return ids;
}

function getEditorTableLicenseSubsectionIds() {
  const estimate = state.openEstimates.find((item) => item.id === state.editorTab);
  return getEditorTableLicenseSubsectionIdsForFgisSet(estimate?.fgisSetId || "");
}

function cloneOpenEstimate(estimate) {
  return JSON.parse(JSON.stringify(estimate));
}

function sectionNeedsLicense() {
  if (state.section === "base") {
    return state.baseTab === "gsn" || state.baseTab === "fgisPrices";
  }
  if (state.section === "editor") {
    return state.estimateViewMode === "table" && state.editorTab !== "buffer";
  }
  return false;
}

function getRequiredLicenseSubsectionIds() {
  if (state.section === "base") {
    if (state.baseTab === "gsn") {
      const id = subsectionIdForGSNSupplement(state.gsnSupplement || "доп.18");
      return id ? [id] : [];
    }
    if (state.baseTab === "fgisPrices") {
      const id = subsectionIdForFGISSet(state.fgisSetId);
      return id ? [id] : [];
    }
    return [];
  }

  if (state.section === "editor" && state.estimateViewMode === "table" && state.editorTab !== "buffer") {
    return getEditorTableLicenseSubsectionIds();
  }

  return [];
}

function licenseSectionElements(section) {
  if (section === "base") {
    return { block: els.baseLicenseBlock, body: els.baseSectionBody };
  }
  if (section === "editor") {
    return { block: els.editorLicenseBlock, body: els.editorSectionBody };
  }
  return null;
}

function setSectionLicenseBlocked(section, blocked) {
  const target = licenseSectionElements(section);
  if (!target?.block || !target?.body) {
    return;
  }
  target.block.classList.toggle("hidden", !blocked);
  target.body.classList.toggle("hidden", blocked);
}

function formatLicenseBlockedMessage(blockedItems) {
  const names = blockedItems.map((item) => item.name).join(", ");
  return `Нет свободных лицензий для ${names}. Обратитесь к администратору системы.`;
}

function renderLicenseBlockMessage(section, blockedItems) {
  const target = licenseSectionElements(section);
  if (!target?.block || !blockedItems.length) {
    return;
  }
  target.block.innerHTML = `<p class="license-block-message">${escapeHTML(formatLicenseBlockedMessage(blockedItems))}</p>`;
}

async function releaseLicenseSessions() {
  if (!state.me) {
    state.licenseBlocked = [];
    state.licenseHeld = [];
    return;
  }

  try {
    await fetch("/api/license-sessions", {
      method: "DELETE",
      credentials: "include",
    });
  } catch {
    // ignore release errors on logout/navigation
  }
  state.licenseBlocked = [];
  state.licenseHeld = [];
}

async function syncLicenseSessions(subsectionIds) {
  if (!state.me) {
    return [];
  }

  if (!subsectionIds.length) {
    await releaseLicenseSessions();
    return [];
  }

  try {
    const response = await fetch("/api/license-sessions", {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      credentials: "include",
      body: JSON.stringify({ subsectionIds }),
    });
    const payload = await response.json().catch(() => ({}));
    if (response.status === 403 || payload?.granted === false) {
      const blocked = Array.isArray(payload?.blocked) ? payload.blocked : [];
      if (blocked.length) {
        state.licenseBlocked = blocked;
        return blocked;
      }
      const fallback = [{ subsectionId: "", name: "выбранных разделов" }];
      state.licenseBlocked = fallback;
      return fallback;
    }
    if (!response.ok) {
      const fallback = [
        {
          subsectionId: "",
          name: String(payload?.error || "лицензий").trim() || "лицензий",
        },
      ];
      state.licenseBlocked = fallback;
      return fallback;
    }
    state.licenseBlocked = [];
    state.licenseHeld = payload.held || [];
    return [];
  } catch {
    const fallback = [{ subsectionId: "", name: "лицензий" }];
    state.licenseBlocked = fallback;
    return fallback;
  }
}

async function syncLicenseSessionsForCurrentSection() {
  return syncLicenseSessions(getRequiredLicenseSubsectionIds());
}

function stopLicenseSessionSync() {
  window.clearInterval(licenseSessionHeartbeatTimer);
  licenseSessionHeartbeatTimer = 0;
}

function startLicenseSessionSync() {
  stopLicenseSessionSync();
  licenseSessionHeartbeatTimer = window.setInterval(() => {
    void syncLicenseSessionsForCurrentSection();
  }, LICENSE_SESSION_HEARTBEAT_MS);
}

async function applyLicenseGate() {
  const version = ++licenseGateVersion;
  const needsLicense = sectionNeedsLicense();

  if (!needsLicense) {
    stopLicenseSessionSync();
    await releaseLicenseSessions();
    if (state.section === "editor") {
      setSectionLicenseBlocked("editor", false);
    }
    renderSectionContent();
    return;
  }

  const blocked = await syncLicenseSessionsForCurrentSection();
  if (version !== licenseGateVersion) {
    return;
  }

  if (blocked.length) {
    stopLicenseSessionSync();
    setSectionLicenseBlocked(state.section, true);
    renderLicenseBlockMessage(state.section, blocked);
    return;
  }

  setSectionLicenseBlocked(state.section, false);
  startLicenseSessionSync();
  renderSectionContent();
}

function renderSectionContent() {
  if (state.section === "base" && state.baseTab === "gsn") {
    els.baseTitle.textContent = "ГСН-2022";
    els.baseDescription.classList.add("hidden");
    els.userPositionsPanel.classList.add("hidden");
    els.fgisPricesPanel.classList.add("hidden");
    renderGSNBaseInfo();
    renderGSNSearchForm();
    els.gsnTree.classList.remove("hidden");
    els.gsnSupplementBar.classList.remove("hidden");
    loadGSNPanel();
  } else if (state.section === "base" && state.baseTab === "fgisPrices") {
    els.baseTitle.textContent = "Сметные цены и индексы";
    els.baseDescription.classList.add("hidden");
    els.gsnTree.classList.add("hidden");
    els.gsnSupplementBar.classList.add("hidden");
    els.userPositionsPanel.classList.add("hidden");
    els.fgisPricesPanel.classList.remove("hidden");
    loadFGISPricesPanel();
  } else if (state.section === "base" && state.baseTab === "userPositions") {
    els.baseTitle.textContent = "Позиции пользователя";
    els.baseDescription.classList.add("hidden");
    els.gsnTree.classList.add("hidden");
    els.gsnSupplementBar.classList.add("hidden");
    els.userPositionsPanel.classList.remove("hidden");
    renderUserPositionsPanel();
  }

  if (state.section === "editor") {
    renderEditor();
  } else if (state.section === "settings") {
    renderSettings();
    void loadAppSettings();
  }
}

async function loadAppSettings({ force = false } = {}) {
  if (!state.me || (state.appSettingsLoaded && !force)) {
    return;
  }
  try {
    const settings = await api("/api/settings");
    state.appSettings = settings;
    state.appSettingsLoaded = true;
    renderSettings();
  } catch (error) {
    if (els.appSettingsHint) {
      els.appSettingsHint.textContent = error.message || "Не удалось загрузить настройки";
    }
  }
}

function renderSettings() {
  const settings = state.appSettings || { calcWorkerCount: 2, editable: false };
  if (els.showHierarchyCodeToggle) {
    els.showHierarchyCodeToggle.checked = state.showHierarchyCode;
  }
  if (els.calcWorkerCountInput) {
    els.calcWorkerCountInput.value = String(settings.calcWorkerCount || 2);
    els.calcWorkerCountInput.disabled = !settings.editable;
  }
  if (els.saveAppSettingsButton) {
    els.saveAppSettingsButton.disabled = !settings.editable;
  }
  if (els.appSettingsHint) {
    els.appSettingsHint.textContent = settings.editable
      ? "Изменение применится сервисом расчета автоматически в течение нескольких секунд."
      : "Изменять количество worker-ов может только администратор.";
  }
}

function getAddLineTargetEstimate() {
  const targetId = state.addLineTargetEstimateId;
  if (targetId) {
    const estimate = state.openEstimates.find((item) => item.id === targetId);
    if (estimate) {
      return estimate;
    }
  }
  if (state.openEstimates.length === 1) {
    return state.openEstimates[0];
  }
  return null;
}

function userPositionBufferKey(positionId) {
  return `buffer:user-position:${positionId}`;
}

function userPositionEstimateKey(positionId) {
  const estimate = getAddLineTargetEstimate();
  if (!estimate) {
    return null;
  }
  return `estimate:${estimate.id}:user-position:${positionId}`;
}

function refreshUserPositionsPanelIfVisible() {
  if (state.section === "base" && state.baseTab === "userPositions") {
    renderUserPositionsPanel();
  }
}

function gsnBufferKey(nodeCode) {
  return `buffer:gsn:${state.gsnSupplement}:${nodeCode}`;
}

function gsnEstimateKey(nodeCode) {
  const estimate = getAddLineTargetEstimate();
  if (!estimate) {
    return null;
  }
  return `estimate:${estimate.id}:gsn:${nodeCode}`;
}

function registerAddPrimitiveSource(key, node) {
  state.addPrimitiveSources[key] = {
    kind: "gsn",
    node: {
      code: node.code,
      originalCode: node.originalCode || "",
      name: node.name || "",
      unit: node.unit || "",
    },
  };
}

function getAddPrimitiveGSNNode(key) {
  return state.addPrimitiveSources[key]?.node || null;
}

function renderGSNLeafPrimitives(node) {
  const bufferKey = gsnBufferKey(node.code);
  registerAddPrimitiveSource(bufferKey, node);
  const bufferQuantity = state.addPrimitiveQuantities[bufferKey] ?? null;

  const estimateKey = gsnEstimateKey(node.code);
  if (estimateKey) {
    registerAddPrimitiveSource(estimateKey, node);
  }
  const estimateQuantity = estimateKey ? (state.addPrimitiveQuantities[estimateKey] ?? null) : null;

  return `
    <div class="gsn-leaf-primitives">
      <div class="gsn-leaf-primitives-row">${renderAddPrimitive(bufferKey, "В буфер", bufferQuantity)}</div>
      ${
        estimateKey
          ? `<div class="gsn-leaf-primitives-row">${renderAddPrimitive(estimateKey, "В смету", estimateQuantity)}</div>`
          : ""
      }
    </div>
  `;
}

function refreshGSNLeafActionsInDOM() {
  if (state.section !== "base" || state.baseTab !== "gsn") {
    return;
  }

  els.gsnTree.querySelectorAll(".gsn-node-leaf[data-gsn-node]").forEach((article) => {
    let node;
    try {
      node = decodeNodeAction(article.dataset.gsnNode);
    } catch {
      return;
    }

    const actions = article.querySelector(".gsn-leaf-actions");
    if (actions) {
      actions.innerHTML = renderGSNLeafPrimitives(node);
    }
  });
}

function renderUserPositionsPanel() {
  const positions = [...state.userPositions].sort(compareByCode);
  const showEstimatePrimitive = Boolean(getAddLineTargetEstimate());
  const rows = positions.map((position, index) => {
    const bufferKey = userPositionBufferKey(position.id);
    const estimateKey = showEstimatePrimitive ? userPositionEstimateKey(position.id) : null;
    const activeQuantity = state.addPrimitiveQuantities[bufferKey] ?? null;
    const estimateActiveQuantity = estimateKey ? (state.addPrimitiveQuantities[estimateKey] ?? null) : null;
    return `
    <tr>
      <td class="user-positions-index-cell">${index + 1}</td>
      <td class="user-positions-code-cell"><span class="user-positions-code-text">${formatBreakableCode(position.code)}</span></td>
      <td class="user-positions-name-cell">${escapeHTML(position.name || "")}</td>
      <td class="user-positions-unit-cell">${escapeHTML(position.unit || "")}</td>
      <td class="user-positions-cost-cell">${money.format(Number(position.cost || 0))}</td>
      <td class="user-positions-actions-cell">
        <div class="user-positions-actions">
          <div class="user-positions-actions-row">
            <button
              class="icon-button secondary"
              data-user-position-edit="${escapeHTML(position.id)}"
              type="button"
              title="Редактировать"
              aria-label="Редактировать"
            >${iconPencil()}</button>
            <button
              class="icon-button secondary icon-button-danger"
              data-user-position-delete="${escapeHTML(position.id)}"
              type="button"
              title="Удалить"
              aria-label="Удалить"
            >${iconTrash()}</button>
          </div>
          <div class="user-positions-actions-row">
            ${renderAddPrimitive(bufferKey, "В буфер", activeQuantity)}
          </div>
          ${
            estimateKey
              ? `<div class="user-positions-actions-row">${renderAddPrimitive(estimateKey, "В смету", estimateActiveQuantity)}</div>`
              : ""
          }
        </div>
      </td>
    </tr>
  `;
  });

  els.userPositionsPanel.innerHTML = `
    <div class="table-wrap">
      <table class="user-positions-table">
        <thead>
          <tr>
            <th class="user-positions-index-cell">№ п/п</th>
            <th class="user-positions-code-cell">Шифр</th>
            <th class="user-positions-name-cell">Наименование</th>
            <th class="user-positions-unit-cell">Единица измерения</th>
            <th class="user-positions-cost-cell">Стоимость</th>
            <th class="user-positions-actions-cell"></th>
          </tr>
        </thead>
        <tbody>
          ${rows.length ? rows.join("") : `<tr><td colspan="6" class="muted">Позиции пользователя отсутствуют</td></tr>`}
        </tbody>
      </table>
    </div>
    <div class="user-positions-footer">
      <button class="user-positions-add-btn" data-user-position-add type="button">
        Добавить позицию пользователя
      </button>
    </div>
  `;
}

function userPositionsStorageKey() {
  const companyId = state.me?.companyId || "default";
  return `nav_user_positions_${companyId}`;
}

function loadUserPositions() {
  const items = readLocalJSON(userPositionsStorageKey(), []);
  const seen = new Set();
  state.userPositions = items.filter((item) => {
    if (!item?.id || seen.has(item.id)) {
      return false;
    }
    seen.add(item.id);
    return true;
  });
  if (seen.size !== items.length) {
    saveUserPositions();
  }
}

function saveUserPositions() {
  localStorage.setItem(userPositionsStorageKey(), JSON.stringify(state.userPositions));
}

function openUserPositionDialog(editId = "") {
  const position = editId ? state.userPositions.find((item) => item.id === editId) : null;
  const isEdit = Boolean(position);

  userPositionDialogEditId = isEdit ? position.id : "";
  els.userPositionDialogForm.reset();
  els.userPositionDialogTitle.textContent = isEdit
    ? "Редактировать позицию пользователя"
    : "Добавить позицию пользователя";
  userPositionField("code").value = isEdit ? position.code || "" : "";
  userPositionField("name").value = isEdit ? position.name || "" : "";
  userPositionField("unit").value = isEdit ? position.unit || "" : "";
  userPositionField("cost").value = isEdit ? String(position.cost ?? 0) : "0";
  els.userPositionDialog.showModal();
  userPositionField("code").focus();
}

function userPositionField(name) {
  return els.userPositionDialogForm.querySelector(`[name="${name}"]`);
}

function newUserPositionId() {
  return `upos_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`;
}

function saveUserPositionDialog(data) {
  const code = String(data.code || "").trim();
  const name = String(data.name || "").trim();
  const unit = String(data.unit || "").trim();
  const cost = Number(data.cost || 0);

  if (!code || !name) {
    showMessage("Укажите шифр и наименование", "error");
    return false;
  }

  if (!Number.isFinite(cost) || cost < 0) {
    showMessage("Укажите корректную стоимость", "error");
    return false;
  }

  if (userPositionDialogEditId) {
    const index = state.userPositions.findIndex((item) => item.id === userPositionDialogEditId);
    if (index < 0) {
      showMessage("Позиция не найдена", "error");
      return false;
    }
    state.userPositions[index] = { ...state.userPositions[index], code, name, unit, cost };
  } else {
    state.userPositions.push({
      id: newUserPositionId(),
      code,
      name,
      unit,
      cost,
    });
  }

  saveUserPositions();
  renderUserPositionsPanel();
  showMessage(userPositionDialogEditId ? "Позиция сохранена" : "Позиция добавлена", "ok");
  return true;
}

function deleteUserPosition(id) {
  const index = state.userPositions.findIndex((item) => item.id === id);
  if (index < 0) {
    showMessage("Позиция не найдена", "error");
    return;
  }

  if (userPositionDialogEditId === id) {
    userPositionDialogEditId = "";
    els.userPositionDialog.close();
    els.userPositionDialogForm.reset();
  }

  state.userPositions.splice(index, 1);
  saveUserPositions();
  renderUserPositionsPanel();
  showMessage("Позиция удалена", "ok");
}

function iconPencil() {
  return `
    <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <path
        d="M4 20h4l10.5-10.5a1.8 1.8 0 0 0 0-2.5l-1.5-1.5a1.8 1.8 0 0 0-2.5 0L4 16v4z"
        fill="none"
        stroke="currentColor"
        stroke-width="1.8"
        stroke-linejoin="round"
      />
      <path d="M13.5 6.5l4 4" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" />
    </svg>
  `;
}

function iconTrash() {
  return `
    <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <path
        d="M5 7h14M9 7V5h6v2M8 7l.7 12h6.6L16 7"
        fill="none"
        stroke="currentColor"
        stroke-width="1.8"
        stroke-linecap="round"
        stroke-linejoin="round"
      />
      <path d="M10 10v6M14 10v6" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" />
    </svg>
  `;
}

function iconPdf() {
  return `
    <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <path
        d="M7 4h7l4 4v12a1 1 0 0 1-1 1H7a1 1 0 0 1-1-1V5a1 1 0 0 1 1-1z"
        fill="none"
        stroke="currentColor"
        stroke-width="1.8"
        stroke-linejoin="round"
      />
      <path d="M14 4v4h4" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round" />
      <path
        d="M8.5 13.5h7M8.5 16.5h5"
        fill="none"
        stroke="currentColor"
        stroke-width="1.6"
        stroke-linecap="round"
      />
    </svg>
  `;
}

function iconArrowUp() {
  return `
    <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <path
        d="M12 7l-5 5M12 7l5 5"
        fill="none"
        stroke="currentColor"
        stroke-width="1.8"
        stroke-linecap="round"
        stroke-linejoin="round"
      />
    </svg>
  `;
}

function iconArrowDown() {
  return `
    <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <path
        d="M12 17l-5-5M12 17l5-5"
        fill="none"
        stroke="currentColor"
        stroke-width="1.8"
        stroke-linecap="round"
        stroke-linejoin="round"
      />
    </svg>
  `;
}

function iconLock() {
  return `
    <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <path
        d="M17 10h-1V7a4 4 0 1 0-8 0v3H7a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2v-8a2 2 0 0 0-2-2Zm-3 0h-4V7a2 2 0 1 1 4 0v3Z"
        fill="currentColor"
      />
    </svg>
  `;
}

function getEstimateLockInfo(estimateId) {
  const id = normalizeEstimateId(estimateId);
  const lock = state.estimateLocks[id];
  if (!lock || lock.userId === state.me?.id) {
    return null;
  }
  return lock;
}

function isEstimateLockedByOther(estimateId) {
  return Boolean(getEstimateLockInfo(estimateId));
}

function startEstimateLockSync() {
  stopEstimateLockSync();
  void refreshEstimateLocks();
  estimateLockPollTimer = window.setInterval(() => {
    void refreshEstimateLocks();
  }, ESTIMATE_LOCK_POLL_MS);
  estimateLockHeartbeatTimer = window.setInterval(() => {
    void refreshOwnedEstimateLocks();
  }, ESTIMATE_LOCK_HEARTBEAT_MS);
}

function stopEstimateLockSync() {
  window.clearInterval(estimateLockPollTimer);
  window.clearInterval(estimateLockHeartbeatTimer);
  estimateLockPollTimer = 0;
  estimateLockHeartbeatTimer = 0;
}

async function refreshEstimateLocks() {
  if (!state.me) {
    return;
  }

  try {
    const locks = await api("/api/estimate-locks");
    const nextLocks = Object.fromEntries(locks.map((lock) => [lock.estimateId, lock]));
    for (const estimate of state.openEstimates) {
      if (!isPersistedEstimateId(estimate.id)) {
        continue;
      }
      const previousLock = state.estimateLocks[estimate.id];
      const nextLock = nextLocks[estimate.id];
      if (previousLock?.userId === state.me?.id && nextLock?.userId !== state.me?.id) {
        void kickEstimateFromEditor(estimate.id);
        return;
      }
    }
    state.estimateLocks = nextLocks;
    renderConstructionTree();
  } catch {
    // ignore transient polling errors
  }
}

async function acquireEstimateLock(estimateId) {
  const response = await fetch(`/api/estimates/${estimateId}/lock`, {
    method: "PUT",
    headers: {
      "Content-Type": "application/json",
    },
    credentials: "include",
  });
  const isJSON = response.headers.get("content-type")?.includes("application/json");
  const payload = isJSON ? await response.json() : null;
  if (response.status === 409) {
    const error = new Error(payload?.error || "смета редактируется другим пользователем");
    error.status = 409;
    error.lock = payload?.lock;
    throw error;
  }
  if (!response.ok) {
    const error = new Error(payload?.error || `HTTP ${response.status}`);
    error.status = response.status;
    throw error;
  }
  if (payload) {
    state.estimateLocks[estimateId] = payload;
  }
  return payload;
}

async function releaseEstimateLock(estimateId) {
  if (!state.me || !isPersistedEstimateId(estimateId)) {
    return;
  }

  try {
    await api(`/api/estimates/${estimateId}/lock`, { method: "DELETE" });
  } catch {
    // ignore release errors on logout/unload
  }
  delete state.estimateLocks[estimateId];
}

async function releaseAllEstimateLocks() {
  await Promise.allSettled(
    state.openEstimates
      .filter((estimate) => isPersistedEstimateId(estimate.id))
      .map((estimate) => releaseEstimateLock(estimate.id)),
  );
}

async function restoreOpenEstimateLocks() {
  for (const estimate of state.openEstimates) {
    if (!isPersistedEstimateId(estimate.id)) {
      continue;
    }
    try {
      await acquireEstimateLock(estimate.id);
    } catch (error) {
      if (error.status === 409 && error.lock) {
        state.estimateLocks[estimate.id] = error.lock;
      }
    }
  }
}

async function refreshOwnedEstimateLocks() {
  if (!state.me) {
    return;
  }

  try {
    const locks = await api("/api/estimate-locks");
    const serverLocks = Object.fromEntries(locks.map((lock) => [lock.estimateId, lock]));

    for (const estimate of [...state.openEstimates]) {
      if (!isPersistedEstimateId(estimate.id)) {
        continue;
      }

      const serverLock = serverLocks[estimate.id];
      if (serverLock?.userId !== state.me?.id) {
        if (state.estimateLocks[estimate.id]?.userId === state.me?.id) {
          void kickEstimateFromEditor(estimate.id);
        }
        continue;
      }

      try {
        await acquireEstimateLock(estimate.id);
      } catch (error) {
        if (error.status === 409 && error.lock) {
          state.estimateLocks[estimate.id] = error.lock;
          renderConstructionTree();
        }
      }
    }

    state.estimateLocks = serverLocks;
    renderConstructionTree();
  } catch {
    // ignore transient heartbeat errors
  }
}

async function kickEstimateFromEditor(estimateId) {
  const index = state.openEstimates.findIndex((item) => item.id === estimateId);
  if (index < 0) {
    return;
  }

  state.openEstimates.splice(index, 1);
  delete state.estimateLocks[estimateId];

  if (state.editorTab === estimateId) {
    state.editorTab =
      state.openEstimates.length > 0
        ? state.openEstimates[Math.min(index, state.openEstimates.length - 1)].id
        : "buffer";
  }

  state.gsnLoaded = false;
  saveEditorState();
  renderEditor();
  renderConstructionTree();
  void refreshEstimateLocks();
  showMessage("Редактирование завершено администратором", "error");
}

function readLocalJSON(key, fallback) {
  try {
    const raw = localStorage.getItem(key);
    return raw ? JSON.parse(raw) : fallback;
  } catch {
    return fallback;
  }
}

function renderEditorSubitems() {
  const estimateButtons = state.openEstimates
    .map(
      (estimate) => `
        <button class="nav-sublink ${sameEstimateId(state.editorTab, estimate.id) ? "active" : ""}" data-editor-tab="${estimate.id}" type="button">
          ${escapeHTML(editorEstimateLabel(estimate))}
        </button>
      `,
    )
    .join("");

  els.editorSubitems.innerHTML = `
    <button class="nav-sublink ${state.editorTab === "buffer" ? "active" : ""}" data-editor-tab="buffer" type="button">Буфер</button>
    ${estimateButtons}
    <button class="nav-sublink nav-sublink-action" data-editor-create-estimate type="button">+ Создать новую смету</button>
  `;
}

async function loadGSNPanel() {
  if (!state.me || state.baseTab !== "gsn") {
    return;
  }

  await loadGSNSupplements();
  if (!state.gsnSupplement) {
    return;
  }

  await loadGSNBaseInfo();
  await loadGSNRoot();
}

async function loadGSNSupplements() {
  if (!state.me || state.gsnSupplementsLoaded) {
    return;
  }

  try {
    const result = await api("/api/gsn/supplements");
    state.gsnSupplements = result.supplements || [];
    state.gsnSupplementsLoaded = true;
    renderGSNSupplementSelect();
  } catch (error) {
    state.gsnSupplementsLoaded = false;
    els.gsnSupplementBar.classList.add("hidden");
    els.gsnTree.innerHTML = `<p class="muted">${escapeHTML(error.message)}</p>`;
  }
}

function gsnSupplementNumber(supplement) {
  if (supplement.ordinal) {
    return supplement.ordinal;
  }
  const match = String(supplement.label || supplement.code).match(/\d+/);
  return match ? Number(match[0]) : null;
}

function gsnSupplementOptionLabel(supplement) {
  const num = gsnSupplementNumber(supplement);
  const buildDate =
    supplement.code === state.gsnSupplement && state.gsnBaseInfo?.version
      ? state.gsnBaseInfo.version
      : supplement.versionDate;
  if (!num) {
    return supplement.label || supplement.code;
  }
  return buildDate ? `Дополнение ${num}, сборка ${buildDate}` : `Дополнение ${num}`;
}

function renderGSNSupplementSelect() {
  if (!state.gsnSupplements.length) {
    els.gsnSupplementBar.classList.add("hidden");
    return;
  }

  const knownCodes = new Set(state.gsnSupplements.map((item) => item.code));
  if (!knownCodes.has(state.gsnSupplement)) {
    state.gsnSupplement = state.gsnSupplements[0].code;
    localStorage.setItem("nav_gsn_supplement", state.gsnSupplement);
  }

  els.gsnSupplementSelect.innerHTML = state.gsnSupplements
    .map(
      (item) => `
        <option value="${escapeHTML(item.code)}" ${item.code === state.gsnSupplement ? "selected" : ""}>
          ${escapeHTML(gsnSupplementOptionLabel(item))}
        </option>
      `,
    )
    .join("");
  els.gsnSupplementSelect.value = state.gsnSupplement;
  els.gsnSupplementBar.classList.remove("hidden");
}

async function loadGSNBaseInfo() {
  if (!state.me || state.gsnBaseInfoLoaded || !state.gsnSupplement) {
    return;
  }

  try {
    const params = new URLSearchParams({ supplement: state.gsnSupplement });
    state.gsnBaseInfo = await api(`/api/gsn/base-info?${params.toString()}`);
    state.gsnBaseInfoLoaded = true;
    renderGSNBaseInfo();
  } catch (error) {
    state.gsnBaseInfoLoaded = false;
    els.baseTitle.textContent = "ГСН-2022";
    els.gsnTree.innerHTML = `<p class="muted">${escapeHTML(error.message)}</p>`;
  }
}

function renderGSNBaseInfo() {
  els.baseTitle.textContent = "ГСН-2022";
  renderGSNSupplementSelect();
}

function renderGSNSearchForm() {
  if (!els.gsnSearchInput) {
    return;
  }
  els.gsnSearchInput.value = state.gsnSearchDraft;
  const resetButton = els.gsnSearchForm?.querySelector("[data-gsn-search-reset]");
  if (resetButton) {
    resetButton.classList.toggle("hidden", !state.gsnSearch);
  }
}

async function loadGSNSearch(options = {}) {
  if (!state.me || state.baseTab !== "gsn" || !state.gsnSupplement || !state.gsnSearch) {
    return;
  }
  if (state.gsnLoaded && !options.force) {
    return;
  }

  state.gsnSearchLoading = true;
  els.gsnTree.innerHTML = `<p class="muted">Поиск...</p>`;

  try {
    const params = new URLSearchParams({
      supplement: state.gsnSupplement,
      q: state.gsnSearch,
      limit: "50",
    });
    const result = await api(`/api/gsn/hierarchy-search?${params.toString()}`);
    const matches = result.matches || [];
    state.gsnLoaded = true;
    if (!matches.length) {
      els.gsnTree.innerHTML = `<p class="muted">Ничего не найдено.</p>`;
      return;
    }
    const tree = buildGSNSearchTree(matches);
    els.gsnTree.innerHTML = renderGSNSearchBranch(tree);
  } catch (error) {
    state.gsnLoaded = false;
    els.gsnTree.innerHTML = `<p class="muted">${escapeHTML(error.message)}</p>`;
  } finally {
    state.gsnSearchLoading = false;
  }
}

function buildGSNSearchTree(matches) {
  const roots = [];

  for (const match of matches) {
    const chain = [...(match.path || []), match.node];
    let level = roots;
    for (const node of chain) {
      let entry = level.find((item) => item.node.code === node.code);
      if (!entry) {
        entry = { node, children: [] };
        level.push(entry);
      }
      level = entry.children;
    }
  }

  return roots;
}

function renderGSNSearchBranch(entries) {
  return entries.map((entry) => renderGSNSearchNode(entry)).join("");
}

function renderGSNSearchNode(entry) {
  const hasTreeChildren = entry.children.length > 0;
  const node = { ...entry.node, hasChildren: hasTreeChildren || entry.node.hasChildren };
  const isPdf = isGSNPdfNode(node);
  const toggle = node.hasChildren
    ? `<button class="tree-toggle" data-gsn-toggle="${escapeHTML(node.code)}" type="button"${
        hasTreeChildren ? ' data-loaded="true"' : ""
      }>${hasTreeChildren ? "-" : "+"}</button>`
    : `<span class="tree-toggle-placeholder"></span>`;
  const encodedNode = encodeNodeAction(node);

  if (!node.hasChildren && !isPdf) {
    const originalCode = gsnLeafOriginalCode(node);
    return `
      <article class="tree-node gsn-node gsn-node-leaf" data-gsn-node="${encodedNode}">
        <header>
          <div class="tree-title">
            ${toggle}
            <div class="gsn-leaf-row">
              <div class="gsn-leaf-cell gsn-leaf-code" title="${escapeHTML(originalCode)}">${escapeHTML(originalCode || "-")}</div>
              <div class="gsn-leaf-cell gsn-leaf-name" title="${escapeHTML(node.name || "")}">${escapeHTML(node.name || "")}</div>
              <div class="gsn-leaf-cell gsn-leaf-unit" title="${escapeHTML(node.unit || "")}">${escapeHTML(node.unit || "-")}</div>
              <div class="gsn-leaf-cell gsn-leaf-actions">${renderGSNLeafPrimitives(node)}</div>
            </div>
          </div>
        </header>
        <div class="tree-children hidden"></div>
      </article>
    `;
  }

  const pdfIcon = isPdf
    ? `<span class="gsn-pdf-icon" title="PDF документ" aria-hidden="true">${iconPdf()}</span>`
    : "";
  const childrenClass = hasTreeChildren ? "tree-children" : "tree-children hidden";
  const childrenHTML = hasTreeChildren ? renderGSNSearchBranch(entry.children) : "";

  return `
    <article class="tree-node gsn-node${isPdf ? " gsn-node-pdf" : ""}"${
      isPdf
        ? ` data-gsn-pdf-code="${escapeHTML(node.code)}" data-gsn-pdf-title="${escapeHTML(gsnNodeTitle(node))}"`
        : ""
    }>
      <header>
        <div class="tree-title">
          ${toggle}
          <div class="gsn-node-label">
            <div class="gsn-node-title-row">
              ${pdfIcon}
              <strong>${escapeHTML(gsnNodeTitle(node))}</strong>
            </div>
          </div>
        </div>
      </header>
      <div class="${childrenClass}">${childrenHTML}</div>
    </article>
  `;
}

async function loadGSNRoot(options = {}) {
  if (!state.me || state.baseTab !== "gsn" || !state.gsnSupplement) {
    return;
  }
  if (state.gsnSearch) {
    return loadGSNSearch(options);
  }
  if (state.gsnLoaded && !options.force) {
    return;
  }

  els.gsnTree.innerHTML = "";

  try {
    const nodes = await fetchGSNChildren("");
    state.gsnLoaded = true;
    els.gsnTree.innerHTML = nodes.length
      ? nodes.map(renderGSNNode).join("")
      : `<p class="muted">Нет данных для отображения.</p>`;
  } catch (error) {
    state.gsnLoaded = false;
    els.gsnTree.innerHTML = `<p class="muted">${escapeHTML(error.message)}</p>`;
  }
}

async function fetchGSNChildren(parentCode) {
  const params = new URLSearchParams({
    supplement: state.gsnSupplement,
    limit: "300",
  });
  if (parentCode) {
    params.set("parent", parentCode);
  }

  const result = await api(`/api/gsn/hierarchy?${params.toString()}`);
  return result.nodes || [];
}

function gsnLeafOriginalCode(node) {
  return String(node?.originalCode || "").trim();
}

function renderGSNNode(node) {
  const isPdf = isGSNPdfNode(node);
  const toggle = node.hasChildren
    ? `<button class="tree-toggle" data-gsn-toggle="${escapeHTML(node.code)}" type="button">+</button>`
    : `<span class="tree-toggle-placeholder"></span>`;
  const encodedNode = encodeNodeAction(node);
  if (!node.hasChildren && !isPdf) {
    const originalCode = gsnLeafOriginalCode(node);
    return `
      <article class="tree-node gsn-node gsn-node-leaf" data-gsn-node="${encodedNode}">
        <header>
          <div class="tree-title">
            ${toggle}
            <div class="gsn-leaf-row">
              <div class="gsn-leaf-cell gsn-leaf-code" title="${escapeHTML(originalCode)}">${escapeHTML(originalCode || "-")}</div>
              <div class="gsn-leaf-cell gsn-leaf-name" title="${escapeHTML(node.name || "")}">${escapeHTML(node.name || "")}</div>
              <div class="gsn-leaf-cell gsn-leaf-unit" title="${escapeHTML(node.unit || "")}">${escapeHTML(node.unit || "-")}</div>
              <div class="gsn-leaf-cell gsn-leaf-actions">${renderGSNLeafPrimitives(node)}</div>
            </div>
          </div>
        </header>
        <div class="tree-children hidden"></div>
      </article>
    `;
  }
  const pdfIcon = isPdf
    ? `<span class="gsn-pdf-icon" title="PDF документ" aria-hidden="true">${iconPdf()}</span>`
    : "";
  return `
    <article class="tree-node gsn-node${isPdf ? " gsn-node-pdf" : ""}"${
      isPdf
        ? ` data-gsn-pdf-code="${escapeHTML(node.code)}" data-gsn-pdf-title="${escapeHTML(gsnNodeTitle(node))}"`
        : ""
    }>
      <header>
        <div class="tree-title">
          ${toggle}
          <div class="gsn-node-label">
            <div class="gsn-node-title-row">
              ${pdfIcon}
              <strong>${escapeHTML(gsnNodeTitle(node))}</strong>
            </div>
          </div>
        </div>
      </header>
      <div class="tree-children hidden"></div>
    </article>
  `;
}

function renderEditor() {
  if (
    state.editorTab !== "buffer" &&
    !state.openEstimates.some((estimate) => sameEstimateId(estimate.id, state.editorTab))
  ) {
    state.editorTab = "buffer";
    sessionStorage.setItem("nav_editor_tab", state.editorTab);
  }

  renderEditorSubitems();
  if (state.editorTab === "buffer") {
    syncEstimateCalcStatusPolling();
    els.editorEyebrow.textContent = "Редактор";
    renderEditorEstimateViewModes(null);
    els.editorTitle.classList.remove("hidden");
    els.editorTitle.textContent = "Буфер";
    els.editorDescription.textContent = "";
    els.editorDescription.classList.add("hidden");
    els.editorContent.innerHTML = state.buffer.length
      ? renderEditorItems(state.buffer, "Буфер пока пуст.")
      : `<p class="muted">Буфер пока пуст.</p>`;
    return;
  }

  const estimate = state.openEstimates.find((item) => sameEstimateId(item.id, state.editorTab));
  if (!estimate) {
    syncEstimateCalcStatusPolling();
    els.editorEyebrow.textContent = "Редактор";
    renderEditorEstimateViewModes(null);
    els.editorTitle.classList.remove("hidden");
    els.editorTitle.textContent = "Редактор";
    els.editorDescription.textContent = "";
    els.editorDescription.classList.add("hidden");
    els.editorContent.innerHTML = `<p class="muted">Выберите смету для редактирования.</p>`;
    return;
  }
  if (editorOpeningEstimateId && sameEstimateId(editorOpeningEstimateId, estimate.id)) {
    els.editorEyebrow.textContent = "Редактор сметы";
    els.editorTitle.classList.add("hidden");
    els.editorDescription.classList.add("hidden");
    renderEditorEstimateViewModes(estimate);
    els.editorContent.innerHTML = `<p class="muted">Открытие сметы…</p>`;
    syncEstimateCalcStatusPolling();
    return;
  }
  els.editorEyebrow.textContent = "Редактор сметы";
  els.editorTitle.classList.add("hidden");
  els.editorDescription.classList.add("hidden");
  renderEditorEstimateViewModes(estimate);
  const renderToken = Symbol("editorRender");
  state.editorRenderToken = renderToken;
  mountEditorContent(estimate);
  if (!state.fgisSetsLoaded) {
    void loadFGISSets()
      .then(() => {
        if (
          state.editorRenderToken !== renderToken ||
          state.editorTab !== estimate.id ||
          state.estimateViewMode !== "table"
        ) {
          return;
        }
        const currentEstimate = state.openEstimates.find((item) => item.id === state.editorTab);
        if (!currentEstimate) {
          return;
        }
        if (refreshEditorFgisSetSelect(currentEstimate)) {
          return;
        }
        mountEditorContent(currentEstimate);
      })
      .catch(() => {});
  }
  syncEstimateCalcStatusPolling();
}

function renderEditorItems(items) {
  return `
    <div class="editor-list">
      ${items
        .map(
          (item) => `
            <article class="editor-item">
              <strong>${escapeHTML(editorItemTitle(item))}</strong>
              ${item.unit ? `<span class="muted">Ед. изм.: ${escapeHTML(item.unit)}</span>` : ""}
              ${
                item.quantity != null
                  ? `<span class="muted">Кол-во: ${formatNumber(item.quantity)}</span>`
                  : ""
              }
            </article>
          `,
        )
        .join("")}
    </div>
  `;
}

function normalizeDistrictRegionCode(code) {
  const trimmed = String(code || "").trim();
  if (!trimmed) {
    return "";
  }
  const numeric = Number.parseInt(trimmed, 10);
  return Number.isFinite(numeric) ? String(numeric) : trimmed;
}

function normalizeDistrictZone(zone) {
  const text = String(zone || "").trim();
  if (!text) {
    return "";
  }
  const numeric = Number.parseInt(text, 10);
  if (!Number.isFinite(numeric) || numeric < 1 || numeric > 11) {
    return "";
  }
  return String(numeric);
}

function parseDistrictValue(district) {
  const text = String(district || "").trim();
  if (!text) {
    return { regionCode: "", zone: "" };
  }

  const dotIndex = text.indexOf(".");
  if (dotIndex < 0) {
    return { regionCode: normalizeDistrictRegionCode(text), zone: "" };
  }

  const regionCode = normalizeDistrictRegionCode(text.slice(0, dotIndex));
  const zonePart = text.slice(dotIndex + 1);
  if (zonePart.includes(".")) {
    return { regionCode, zone: "" };
  }

  return {
    regionCode,
    zone: normalizeDistrictZone(zonePart),
  };
}

function formatDistrictValue(regionCode, zone) {
  const base = normalizeDistrictRegionCode(regionCode);
  if (!base) {
    return "";
  }
  const zoneText = normalizeDistrictZone(zone);
  if (!zoneText) {
    return base;
  }
  return `${base}.${zoneText}`;
}

function findRegionByDistrictCode(regionCode) {
  const normalized = normalizeDistrictRegionCode(regionCode);
  if (!normalized) {
    return null;
  }

  return (
    state.gsnRegions.find((item) => normalizeDistrictRegionCode(item.code) === normalized) ||
    state.gsnRegions.find((item) => item.code === normalized) ||
    state.gsnRegions.find((item) => item.code === normalized.padStart(2, "0")) ||
    null
  );
}

function districtRegionName(district) {
  const { regionCode } = parseDistrictValue(district);
  const region = findRegionByDistrictCode(regionCode);
  return region?.name || "";
}

function districtRegionHint(district) {
  const name = districtRegionName(district);
  if (name) {
    return name;
  }
  return district ? "Регион не найден в классификаторе" : "Нажмите, чтобы выбрать сметный район";
}

async function loadGSNRegions() {
  if (state.gsnRegionsLoaded && state.gsnRegions.length) {
    return state.gsnRegions;
  }

  try {
    const result = await api("/api/gsn/regions");
    state.gsnRegions = result.regions || [];
  } catch (apiError) {
    state.gsnRegions = [];
  }

  if (!state.gsnRegions.length) {
    try {
      const response = await fetch("/regions.json");
      if (response.ok) {
        state.gsnRegions = await response.json();
      }
    } catch {
      // ignore fallback errors
    }
  }

  if (!state.gsnRegions.length) {
    throw new Error("Классификатор регионов недоступен");
  }

  state.gsnRegionsLoaded = true;
  return state.gsnRegions;
}

async function loadFGISSets() {
  if (state.fgisSetsLoaded) {
    return state.fgisSets;
  }

  try {
    const result = await api("/api/gsn/fgis-sets");
    state.fgisSets = result.sets || [];
  } catch {
    state.fgisSets = [];
  }

  state.fgisSetsLoaded = true;
  return state.fgisSets;
}

function renderFgisSetSelectOptions(selectedId) {
  const selected = String(selectedId || "");
  const options = [`<option value="">Не выбраны</option>`];
  for (const set of state.fgisSets) {
    const setId = String(set.id || "");
    options.push(
      `<option value="${escapeHTML(setId)}" ${selected === setId ? "selected" : ""}>${escapeHTML(set.name || setId)}</option>`,
    );
  }
  if (selected && !state.fgisSets.some((set) => set.id === selected)) {
    options.push(`<option value="${escapeHTML(selected)}" selected>${escapeHTML(selected)}</option>`);
  }
  return options.join("");
}

function renderFgisPricesSetSelectOptions(selectedId) {
  const selected = String(selectedId || "");
  if (!state.fgisSets.length) {
    return `<option value="">Наборы не загружены</option>`;
  }
  return state.fgisSets
    .map((set) => {
      const setId = String(set.id || "");
      return `<option value="${escapeHTML(setId)}" ${selected === setId ? "selected" : ""}>${escapeHTML(set.name || setId)}</option>`;
    })
    .join("");
}

function formatFGISCount(value) {
  return new Intl.NumberFormat("ru-RU").format(Number(value || 0));
}

function renderFGISPricesPanelContent() {
  const setSelected = Boolean(state.fgisSetId);
  const stats = state.fgisStats;
  const rows = state.fgisRows;
  const pageStart = state.fgisFilteredTotal ? state.fgisOffset + 1 : 0;
  const pageEnd = state.fgisFilteredTotal
    ? Math.min(state.fgisOffset + rows.length, state.fgisFilteredTotal)
    : 0;
  const canPrev = state.fgisOffset > 0;
  const canNext = state.fgisOffset + state.fgisLimit < state.fgisFilteredTotal;

  const tableRows = rows.map((row, index) => `
    <tr>
      <td class="fgis-prices-index-cell">${state.fgisOffset + index + 1}</td>
      <td class="fgis-prices-original-code-cell"><span class="fgis-prices-code-text">${formatBreakableCode(row.originalCode || row.code)}</span></td>
      <td class="fgis-prices-name-cell">${escapeHTML(row.name || "")}</td>
      <td class="fgis-prices-unit-cell">${escapeHTML(row.unit || "")}</td>
      <td class="fgis-prices-value-cell">${escapeHTML(row.prices || "")}</td>
      <td class="fgis-prices-value-cell">${escapeHTML(row.indexes || "")}</td>
    </tr>
  `);

  els.fgisPricesPanel.innerHTML = `
    <div class="fgis-prices-toolbar">
      <div class="fgis-prices-set-bar">
        <span class="fgis-prices-set-label">Набор:</span>
        <select id="fgisSetSelect" class="fgis-prices-set-select" aria-label="Набор сметных цен и индексов">
          ${renderFgisPricesSetSelectOptions(state.fgisSetId)}
        </select>
      </div>
      ${
        setSelected && stats
          ? `<p class="fgis-prices-stats muted">Сметных цен — ${formatFGISCount(stats.pricesCount)}, индексов — ${formatFGISCount(stats.indexesCount)}.</p>`
          : ""
      }
      ${
        setSelected
          ? `
            <form id="fgisSearchForm" class="fgis-prices-search">
              <input
                id="fgisSearchInput"
                class="fgis-prices-search-input"
                type="search"
                placeholder="Поиск по шифру или наименованию"
                value="${escapeHTML(state.fgisSearchDraft)}"
              />
              <button type="submit">Найти</button>
              ${
                state.fgisSearch
                  ? `<button class="secondary" type="button" data-fgis-search-reset>Очистить</button>`
                  : ""
              }
            </form>
          `
          : ""
      }
    </div>
    ${
      !setSelected
        ? `<p class="muted">Выберите набор сметных цен и индексов.</p>`
        : state.fgisLoading
          ? `<p class="muted">Загрузка записей...</p>`
          : `
            <div class="table-wrap">
              <table class="fgis-prices-table">
                <thead>
                  <tr>
                    <th class="fgis-prices-index-cell">№ п/п</th>
                    <th class="fgis-prices-original-code-cell">Оригинальный шифр</th>
                    <th class="fgis-prices-name-cell">Наименование</th>
                    <th class="fgis-prices-unit-cell">Ед. изм.</th>
                    <th class="fgis-prices-value-cell">Сметные цены</th>
                    <th class="fgis-prices-value-cell">Индексы</th>
                  </tr>
                </thead>
                <tbody>
                  ${
                    tableRows.length
                      ? tableRows.join("")
                      : `<tr><td colspan="6" class="muted">${
                          state.fgisSearch ? "Ничего не найдено" : "Записи отсутствуют"
                        }</td></tr>`
                  }
                </tbody>
              </table>
            </div>
            <div class="fgis-prices-pagination">
              <button class="secondary" type="button" data-fgis-page="prev" ${canPrev ? "" : "disabled"}>Назад</button>
              <span class="muted">${pageStart}–${pageEnd} из ${formatFGISCount(state.fgisFilteredTotal)}</span>
              <button class="secondary" type="button" data-fgis-page="next" ${canNext ? "" : "disabled"}>Вперёд</button>
            </div>
          `
    }
  `;
}

async function loadFGISRows(options = {}) {
  if (!state.me || !state.fgisSetId) {
    state.fgisRows = [];
    state.fgisStats = null;
    state.fgisFilteredTotal = 0;
    state.fgisLoadedSetId = "";
    renderFGISPricesPanelContent();
    return;
  }

  if (options.resetOffset) {
    state.fgisOffset = 0;
  }

  state.fgisLoading = true;
  if (!options.silent) {
    renderFGISPricesPanelContent();
  }

  try {
    const params = new URLSearchParams({
      set: state.fgisSetId,
      limit: String(state.fgisLimit),
      offset: String(state.fgisOffset),
    });
    if (state.fgisSearch) {
      params.set("q", state.fgisSearch);
    }
    const result = await api(`/api/gsn/fgis-rows?${params.toString()}`);
    state.fgisRows = result.rows || [];
    state.fgisStats = result.stats || null;
    state.fgisFilteredTotal = Number(result.filteredTotal || 0);
    state.fgisLoadedSetId = state.fgisSetId;
  } catch (error) {
    state.fgisRows = [];
    state.fgisStats = null;
    state.fgisFilteredTotal = 0;
    state.fgisLoadedSetId = "";
    els.fgisPricesPanel.innerHTML = `<p class="muted">${escapeHTML(error.message)}</p>`;
    return;
  } finally {
    state.fgisLoading = false;
  }

  renderFGISPricesPanelContent();
}

async function loadFGISPricesPanel(force = false) {
  if (!state.me) {
    els.fgisPricesPanel.innerHTML = `<p class="muted">Войдите в систему для просмотра сметных цен и индексов.</p>`;
    return;
  }

  try {
    await loadFGISSets();
  } catch (error) {
    els.fgisPricesPanel.innerHTML = `<p class="muted">${escapeHTML(error.message)}</p>`;
    return;
  }

  if (!state.fgisSets.length) {
    els.fgisPricesPanel.innerHTML = `<p class="muted">Наборы сметных цен и индексов не найдены.</p>`;
    return;
  }

  const knownIds = new Set(state.fgisSets.map((set) => set.id));
  if (!knownIds.has(state.fgisSetId)) {
    state.fgisSetId = state.fgisSets[0].id;
    localStorage.setItem("nav_fgis_set", state.fgisSetId);
    state.fgisOffset = 0;
    state.fgisSearch = "";
    state.fgisSearchDraft = "";
    force = true;
  }

  if (
    force ||
    state.fgisLoadedSetId !== state.fgisSetId ||
    (state.section === "base" && state.baseTab === "fgisPrices" && !state.fgisLoading && !state.fgisRows.length)
  ) {
    await loadFGISRows({ silent: state.fgisRows.length > 0 && state.fgisLoadedSetId === state.fgisSetId });
    return;
  }

  renderFGISPricesPanelContent();
}

function changeFGISSet(nextSetId) {
  if (!nextSetId || nextSetId === state.fgisSetId) {
    return;
  }
  state.fgisSetId = nextSetId;
  localStorage.setItem("nav_fgis_set", state.fgisSetId);
  state.fgisOffset = 0;
  state.fgisSearch = "";
  state.fgisSearchDraft = "";
  void loadFGISRows();
  void applyLicenseGate();
}

function changeFGISPage(direction) {
  if (direction === "prev" && state.fgisOffset > 0) {
    state.fgisOffset = Math.max(0, state.fgisOffset - state.fgisLimit);
    void loadFGISRows({ silent: true });
    return;
  }
  if (direction === "next" && state.fgisOffset + state.fgisLimit < state.fgisFilteredTotal) {
    state.fgisOffset += state.fgisLimit;
    void loadFGISRows({ silent: true });
  }
}

function renderDistrictRegionList(filterText = "") {
  const query = String(filterText || "").trim().toLowerCase();
  const regions = state.gsnRegions.filter((region) => {
    if (!query) {
      return true;
    }
    const code = normalizeDistrictRegionCode(region.code);
    return (
      code.includes(query) ||
      String(region.code).toLowerCase().includes(query) ||
      String(region.name || "").toLowerCase().includes(query)
    );
  });

  if (!regions.length) {
    els.districtRegionList.innerHTML = `<p class="muted district-region-empty">${
      state.gsnRegions.length ? "Ничего не найдено" : "Классификатор регионов не загружен"
    }</p>`;
    return;
  }

  els.districtRegionList.innerHTML = regions
    .map((region) => {
      const selected = state.districtPickerRegionCode === region.code;
      return `
        <button
          class="district-region-item ${selected ? "active" : ""}"
          data-district-region="${escapeHTML(region.code)}"
          type="button"
        >
          <span class="district-region-code">${escapeHTML(normalizeDistrictRegionCode(region.code))}</span>
          <span class="district-region-name">${escapeHTML(region.name || "")}</span>
        </button>
      `;
    })
    .join("");
}

function scrollDistrictRegionSelectionIntoView() {
  const activeItem = els.districtRegionList.querySelector(".district-region-item.active");
  if (activeItem) {
    activeItem.scrollIntoView({ block: "nearest" });
  }
}

async function openDistrictDialog(estimateID) {
  const estimate = state.openEstimates.find((item) => item.id === estimateID);
  if (!estimate) {
    return;
  }

  try {
    await loadGSNRegions();
  } catch (error) {
    showMessage(error.message, "error");
    return;
  }

  const parsed = parseDistrictValue(estimate.district);
  const region = findRegionByDistrictCode(parsed.regionCode);
  state.districtPickerRegionCode = region?.code || "";

  els.districtDialogForm.elements.estimateId.value = estimateID;
  els.districtRegionFilter.value = "";
  els.districtZoneSelect.value = parsed.zone || "";
  renderDistrictRegionList();

  els.districtDialog.showModal();
  requestAnimationFrame(() => {
    scrollDistrictRegionSelectionIntoView();
  });
  els.districtRegionFilter.focus();
}

function applyDistrictToEstimate(estimateID, options = {}) {
  const estimate = state.openEstimates.find((item) => item.id === estimateID);
  if (!estimate || !state.districtPickerRegionCode) {
    return false;
  }

  estimate.district = formatDistrictValue(state.districtPickerRegionCode, els.districtZoneSelect.value);
  invalidateEstimateTextDraft(estimateID);
  saveEditorState();
  renderEditor();
  if (options.persist !== false) {
    void persistOpenEstimate(estimateId).catch(() => {});
  }
  return true;
}

async function applyDistrictDialog() {
  const estimateID = els.districtDialogForm.elements.estimateId.value;
  const estimate = state.openEstimates.find((item) => item.id === estimateID);
  if (!estimate) {
    els.districtDialog.close();
    return;
  }

  if (!state.districtPickerRegionCode) {
    showMessage("Выберите регион", "error");
    return;
  }

  const previousDistrict = estimate.district || "";
  applyDistrictToEstimate(estimateID, { persist: false });
  els.districtDialog.close();

  if (estimate.district !== previousDistrict) {
    (estimate.items || []).forEach(clearGsnLineCalcEnrichment);
    showMessage("Сметный район обновлён", "ok");
  }

  saveEditorState();
  renderEditor();
  try {
    await persistOpenEstimate(estimateID);
  } catch {
    return;
  }
}

function parseLocalizedNumber(value) {
  const text = String(value ?? "").trim().replace(/\s/g, "").replace(",", ".");
  if (!text) {
    return 0;
  }
  const number = Number(text);
  return Number.isFinite(number) ? number : 0;
}

function normalizeResourcePricing(item) {
  if (!item || item.unitPriceIndex) {
    return item;
  }
  const text = String(item.unitPriceText || "").trim();
  const starIndex = text.indexOf("*");
  if (starIndex < 0) {
    return item;
  }
  return {
    ...item,
    unitPriceText: text.slice(0, starIndex).trim(),
    unitPriceIndex: text.slice(starIndex + 1).trim(),
  };
}

function estimateResourceUnitPriceValue(item) {
  const normalized = normalizeResourcePricing(item);
  const base = parseLocalizedNumber(normalized.unitPriceText);
  const index = parseLocalizedNumber(normalized.unitPriceIndex);
  if (index) {
    return base * index;
  }
  if (base) {
    return base;
  }
  return Number(normalized.unitPrice || 0);
}

function estimateResourceLineTotal(child, parentQuantity) {
  const unitPrice = estimateResourceUnitPriceValue(child);
  const consumption = Number(child?.quantity || 0);
  const positionQuantity = Number(parentQuantity || 0);
  return unitPrice * consumption * positionQuantity;
}

function estimatePositionTotalFromResources(item) {
  const positionQuantity = resolveEstimateItemQuantity(item);
  return (item?.children || []).reduce(
    (sum, child) => sum + estimateResourceLineTotal(child, positionQuantity),
    0,
  );
}

function parseEstimateCalcJsonRecord(item) {
  const raw = item?.calcJson;
  if (!raw) {
    return null;
  }
  let parsed = raw;
  if (typeof raw === "string") {
    try {
      parsed = JSON.parse(raw);
    } catch {
      return null;
    }
  }
  if (parsed?.record) {
    return parsed.record;
  }
  if (parsed?.code) {
    return parsed;
  }
  return null;
}

function estimateLineTotalFromCalcJson(item) {
  const record = parseEstimateCalcJsonRecord(item);
  if (!record) {
    return null;
  }
  const quantity = resolveEstimateItemQuantity(item);
  if (record.isWork === false) {
    return (
      estimateResourceUnitPriceValue({
        unitPriceText: record.unitPriceText || "",
        unitPriceIndex: record.unitPriceIndex || "",
        unitPrice: record.unitPrice || 0,
      }) * quantity
    );
  }
  if (!Array.isArray(record.resources) || !record.resources.length) {
    return null;
  }
  const children = record.resources.map((resource, index) =>
    estimateChildFromResource(resource, index, item?.id || "line"),
  );
  return estimatePositionTotalFromResources({ quantity, children });
}

function readEstimateCalcPricingFromJson(calcJson) {
  if (!calcJson) {
    return null;
  }
  let parsed = calcJson;
  if (typeof calcJson === "string") {
    try {
      parsed = JSON.parse(calcJson);
    } catch {
      return null;
    }
  }
  const quantity = parsed?.quantity != null ? Number(parsed.quantity) : null;
  const unitPrice = parsed?.unitPrice != null ? Number(parsed.unitPrice) : null;
  const total = parsed?.total != null ? Number(parsed.total) : null;
  if (
    (quantity != null && !Number.isFinite(quantity)) ||
    (unitPrice != null && !Number.isFinite(unitPrice)) ||
    (total != null && !Number.isFinite(total))
  ) {
    return null;
  }
  return { quantity, unitPrice, total };
}

function resolveEstimateLineServerTotal(item) {
  const direct = Number(item?.total);
  if (Number.isFinite(direct) && direct !== 0) {
    return direct;
  }
  const fromJson = readEstimateCalcPricingFromJson(item?.calcJson);
  if (fromJson?.total != null && Number.isFinite(fromJson.total) && fromJson.total !== 0) {
    return fromJson.total;
  }
  return Number.isFinite(direct) ? direct : 0;
}

function estimateLineTotal(item, parentItem = null) {
  if (item?.type === "resource" && parentItem) {
    return estimateResourceLineTotal(item, parentItem.quantity);
  }
  if (estimateLineIsStructural(item)) {
    return 0;
  }
  if (estimateLineNeedsGsnCalc(item)) {
    return estimateLineCalcDone(item) ? resolveEstimateLineServerTotal(item) : 0;
  }
  return Number(item?.total ?? resolveEstimateItemQuantity(item) * Number(item?.unitPrice || 0));
}

function estimateGrandTotal(items) {
  return (items || []).reduce((sum, item) => sum + estimateLineTotal(item), 0);
}

function estimateLineNeedsGsnCalc(item) {
  if (item?.type !== "position" || !estimateLineIsGsn(item) || estimateLineIsStructural(item)) {
    return false;
  }
  return Boolean(estimateRecordLookupCode(item));
}

function estimateLineCountsTowardGrandTotal(item) {
  if (estimateLineIsStructural(item)) {
    return false;
  }
  if (estimateLineNeedsGsnCalc(item)) {
    return estimateLineCalcPricingReady(item);
  }
  return true;
}

function estimateLineHasAppliedPricing(item) {
  if (estimateLineIsWorkPosition(item)) {
    return Boolean(item?.children?.length);
  }
  if (estimateLineIsResourcePosition(item)) {
    return Boolean(String(item?.unitPriceText || "").trim() || Number(item?.unitPrice || 0));
  }
  return Boolean(Number(item?.unitPrice || 0) || Number(item?.total || 0));
}

function estimateGsnLineCalcGrandTotal(item) {
  return estimateLineCalcDone(item) ? resolveEstimateLineServerTotal(item) : 0;
}

function estimateLineCalcPricingReady(item) {
  return estimateLineCalcDone(item);
}

function applyEstimateCalcPricingFromJson(item, calcJson) {
  const pricing = readEstimateCalcPricingFromJson(calcJson);
  if (!item || !pricing) {
    return;
  }
  if (pricing.quantity != null) {
    item.quantity = pricing.quantity;
  }
  if (pricing.unitPrice != null) {
    item.unitPrice = pricing.unitPrice;
  }
  if (pricing.total != null) {
    item.total = pricing.total;
  }
}

function applyEstimateCalcLineResult(item, status) {
  if (!item || !status) {
    return;
  }
  const nextStatus = String(status.status || "").trim();
  if (nextStatus) {
    item.calcStatus = nextStatus;
  }
  if (status.error !== undefined) {
    item.calcError = status.error || "";
  }
  if (status.code) {
    item.code = status.code;
    item.recordCode = status.code;
  }
  if (status.originalCode) {
    item.originalCode = status.originalCode;
  }
  if (status.name) {
    item.name = status.name;
  }
  if (status.unit) {
    item.unit = status.unit;
  }
  if (status.quantity != null && Number.isFinite(Number(status.quantity))) {
    item.quantity = Number(status.quantity);
  }
  if (status.unitPrice != null && Number.isFinite(Number(status.unitPrice))) {
    item.unitPrice = Number(status.unitPrice);
  }
  if (status.total != null && Number.isFinite(Number(status.total))) {
    item.total = Number(status.total);
  }
}

function estimateCalcTerminalStatus(status) {
  const value = String(status || "").trim();
  return value === "done" || value === "failed" || value === "dead";
}

function estimateCalcProgressDisplayErrors(target, displayed) {
  const totalErrors = target?.errors ?? 0;
  const processed = target?.processed ?? 0;
  if (processed <= 0 || displayed >= processed) {
    return totalErrors;
  }
  return Math.min(totalErrors, Math.round((totalErrors * displayed) / processed));
}

function isLargeEstimateForCalc(estimate) {
  const key = estimateCalcTargetKey(estimate?.id || "");
  const target = estimateCalcProgressTargets.get(key);
  if (target?.largeEstimate !== undefined) {
    return target.largeEstimate;
  }
  return estimateCalcProgressTotal(estimate?.items) > LARGE_ESTIMATE_CALC_LINE_THRESHOLD;
}

function estimateCalcApplyChunkSize(estimate) {
  return isLargeEstimateForCalc(estimate) ? LARGE_ESTIMATE_CALC_APPLY_CHUNK : ESTIMATE_CALC_APPLY_CHUNK;
}

function estimateCalcStatusPollIntervalMs(estimate) {
  return isLargeEstimateForCalc(estimate) ? LARGE_ESTIMATE_CALC_STATUS_POLL_MS : ESTIMATE_CALC_STATUS_POLL_MS;
}

function initCalcProgressGrandTotalTarget() {
  return {
    runningGrandTotal: 0,
    countedDoneLineIds: new Set(),
  };
}

function buildEstimateItemsCalcLookup(items) {
  const byId = new Map();
  const poolsByCode = new Map();
  (items || []).forEach((item) => {
    if (item?.id) {
      byId.set(item.id, item);
    }
    if (!estimateLineNeedsGsnCalc(item)) {
      return;
    }
    const code = estimateRecordLookupCode(item);
    if (!code) {
      return;
    }
    const pool = poolsByCode.get(code);
    if (pool) {
      pool.push(item);
    } else {
      poolsByCode.set(code, [item]);
    }
  });
  return { byId, poolsByCode };
}

function createEstimateCalcStatusResolver(items) {
  const lookup = buildEstimateItemsCalcLookup(items);
  const poolsByCode = new Map();
  lookup.poolsByCode.forEach((pool, code) => {
    poolsByCode.set(code, pool.slice());
  });
  return (status) => {
    if (!status) {
      return null;
    }
    let item = lookup.byId.get(status.lineId) || null;
    if (item) {
      return item;
    }
    const code = String(status.code || "").trim();
    if (!code) {
      return null;
    }
    const pool = poolsByCode.get(code);
    if (!pool?.length) {
      return null;
    }
    return pool.shift() || null;
  };
}

function resolveEstimateItemForCalcStatus(lookup, status) {
  if (!status) {
    return null;
  }
  const byId = lookup?.byId || lookup;
  let item = byId?.get(status.lineId) || null;
  if (item) {
    return item;
  }
  const code = String(status.code || "").trim();
  if (!code || !lookup?.poolsByCode) {
    return null;
  }
  const pool = lookup.poolsByCode.get(code);
  return pool?.[0] || null;
}

function estimateCalcInProgress(estimateId, items, target = null) {
  const key = estimateCalcTargetKey(estimateId);
  if (key && estimateCalcAwaitingServer.has(key)) {
    return true;
  }
  const resolvedTarget = target || estimateCalcProgressTargets.get(key);
  if (resolvedTarget?.total > 0 && (resolvedTarget.processed ?? 0) < resolvedTarget.total) {
    return true;
  }
  return estimateHasInFlightGsnCalc(items);
}

function getCalcProgressGrandTotalForDisplay(target, items, estimateId = "") {
  if (estimateCalcInProgress(estimateId, items, target) && target?.countedDoneLineIds) {
    return target.runningGrandTotal ?? 0;
  }
  return estimateGrandTotalForDisplay(items);
}

function isGsnCalcStatusEntry(item, status) {
  if (item) {
    return estimateLineNeedsGsnCalc(item);
  }
  return Boolean(String(status?.code || "").trim());
}

function resetGsnLinesForTableCalculation(items) {
  (items || []).forEach((item) => {
    if (!estimateLineNeedsGsnCalc(item)) {
      return;
    }
    item.calcStatus = "queued";
    item.calcError = "";
    item.calcJson = null;
    item.calcJsonKey = JSON.stringify(null);
    delete item.calcRecordAppliedKey;
    item.unitPrice = 0;
    item.unitPriceText = "";
    item.unitPriceIndex = "";
    item.total = 0;
    item.hasResources = false;
    item.children = undefined;
    delete item.isWork;
  });
}

function prepareEstimateTableCalculationState(estimateId, estimate = null) {
  const resolved = estimate || state.openEstimates.find((item) => sameEstimateId(item.id, estimateId));
  if (!resolved) {
    return;
  }
  resetGsnLinesForTableCalculation(resolved.items);
  restartEstimateTableCalculation(estimateId, resolved);
  markEstimateCalcAwaitingServer(estimateId);
  estimateCalcPollBlockedByPersist.add(estimateId);
}

function accumulateCalcGrandTotalFromItems(estimateId, items) {
  const key = estimateCalcTargetKey(estimateId);
  const target = estimateCalcProgressTargets.get(key);
  if (!target) {
    return;
  }
  if (!target.countedDoneLineIds) {
    Object.assign(target, initCalcProgressGrandTotalTarget());
  }
  let added = 0;
  (items || []).forEach((item) => {
    if (!item || !estimateLineNeedsGsnCalc(item) || item.calcStatus !== "done") {
      return;
    }
    const lineKey = item.id;
    if (target.countedDoneLineIds.has(lineKey)) {
      return;
    }
    if (estimateCalcRecordNeedsApply(item)) {
      applyEstimateCalcRecord(item, item.calcJson);
    }
    target.countedDoneLineIds.add(lineKey);
    added += estimateGsnLineCalcGrandTotal(item);
  });
  if (added) {
    target.runningGrandTotal = (target.runningGrandTotal ?? 0) + added;
  }
}

function applyEstimateCalcBatch(estimate, statuses) {
  const resolveItem = createEstimateCalcStatusResolver(estimate?.items);
  applyEstimateCalcStatusFieldsOnly(estimate, statuses, resolveItem);
  applyEstimateCalcRecordsBatch(estimate, statuses, resolveItem);
}

function estimateGrandTotalForDisplay(items) {
  return (items || []).reduce((sum, item) => {
    if (!estimateLineCountsTowardGrandTotal(item)) {
      return sum;
    }
    return sum + estimateLineTotal(item);
  }, 0);
}

function estimateCalcProgressBaseGrandTotal(items) {
  return (items || []).reduce((sum, item) => {
    if (estimateLineIsStructural(item) || estimateLineNeedsGsnCalc(item)) {
      return sum;
    }
    return sum + estimateLineTotal(item);
  }, 0);
}

function estimateCalcProgressGrandTotalForDisplay(target, items, key, done) {
  return estimateGrandTotalForDisplay(items);
}

function estimateDisplayedGrandTotal(estimate) {
  return estimateGrandTotalForDisplay(estimate?.items || []);
}

function markEstimateCalcAwaitingServer(estimateId) {
  const key = estimateCalcTargetKey(estimateId);
  if (key) {
    estimateCalcAwaitingServer.add(key);
  }
}

function clearEstimateCalcAwaitingServer(estimateId) {
  const key = estimateCalcTargetKey(estimateId);
  if (key) {
    estimateCalcAwaitingServer.delete(key);
  }
}

function estimateHasInFlightGsnCalc(items) {
  return (items || []).some((item) => {
    if (!estimateLineNeedsGsnCalc(item)) {
      return false;
    }
    const status = String(item.calcStatus || "").trim();
    return !status || status === "queued" || status === "leased";
  });
}

function ensureEstimateCalcProgressTarget(estimateId, estimate = null) {
  const key = estimateCalcTargetKey(estimateId);
  const resolved = estimate || state.openEstimates.find((item) => sameEstimateId(item.id, estimateId));
  if (!resolved) {
    return estimateCalcProgressTargets.get(key) || null;
  }
  const itemTotal = estimateCalcProgressTotal(resolved.items);
  const previous = estimateCalcProgressTargets.get(key);
  if (!itemTotal) {
    return previous || null;
  }
  const fromItems = estimateCalcProgress(resolved.items, key);

  if (!previous) {
    estimateCalcProgressTargets.set(key, {
      total: itemTotal,
      processed: fromItems.processed,
      errors: fromItems.errors,
      displayed: 0,
      largeEstimate: itemTotal > LARGE_ESTIMATE_CALC_LINE_THRESHOLD,
      ...initCalcProgressGrandTotalTarget(),
    });
    return estimateCalcProgressTargets.get(key);
  }

  estimateCalcProgressTargets.set(key, {
    ...previous,
    total: Math.max(itemTotal, previous.total ?? 0),
    processed: Math.max(previous.processed ?? 0, fromItems.processed),
    errors: Math.max(previous.errors ?? 0, fromItems.errors),
    countedDoneLineIds: previous.countedDoneLineIds ?? new Set(),
    runningGrandTotal: previous.runningGrandTotal ?? 0,
  });
  return estimateCalcProgressTargets.get(key);
}

function estimateCalcAwaitingBlocksProgress(estimateId, items) {
  const key = estimateCalcTargetKey(estimateId);
  if (!key || !estimateCalcAwaitingServer.has(key)) {
    return false;
  }
  return !(items || []).some(
    (item) =>
      estimateLineNeedsGsnCalc(item) && estimateCalcTerminalStatus(String(item.calcStatus || "").trim()),
  );
}

function restartEstimateTableCalculation(estimateId, estimate = null) {
  const key = estimateCalcTargetKey(estimateId);
  const resolved = estimate || state.openEstimates.find((item) => sameEstimateId(item.id, estimateId));
  if (!resolved) {
    return;
  }
  const calcTotal = estimateCalcProgressTotal(resolved.items);
  if (calcTotal > 0) {
    const grand = initCalcProgressGrandTotalTarget();
    estimateCalcProgressTargets.set(key, {
      total: calcTotal,
      processed: 0,
      errors: 0,
      displayed: 0,
      largeEstimate: calcTotal > LARGE_ESTIMATE_CALC_LINE_THRESHOLD,
      ...grand,
    });
    markEstimateCalcAwaitingServer(estimateId);
  } else {
    estimateCalcProgressTargets.delete(key);
    clearEstimateCalcAwaitingServer(estimateId);
  }
}

function estimateCalcProgress(items, estimateId = "") {
  const calcItems = (items || []).filter(estimateLineNeedsGsnCalc);
  let processed = 0;
  let errors = 0;
  calcItems.forEach((item) => {
    const status = String(item.calcStatus || "").trim();
    if (estimateCalcTerminalStatus(status)) {
      processed += 1;
    }
    if (status === "failed" || status === "dead") {
      errors += 1;
    }
  });
  return { total: calcItems.length, processed, errors };
}

function estimateCalcProgressView(items, estimateId = "") {
  const { total, processed, errors } = estimateCalcProgress(items, estimateId);
  if (!total) {
    return null;
  }
  return {
    total,
    processed,
    errors,
    done: processed >= total,
  };
}

function renderEstimateCalcProgressHTML(items, estimateId = "", displayOverride = null) {
  const view =
    displayOverride || estimateCalcProgressView(items, estimateCalcTargetKey(estimateId));
  if (!view) {
    return "";
  }
  return `<span class="editor-estimate-calc-progress">
    <span class="editor-estimate-calc-progress-main ${
      view.done ? "editor-estimate-calc-progress-main-ok" : "editor-estimate-calc-progress-main-danger"
    }">Обработано позиций: ${view.processed}/${view.total}</span>${
      view.errors > 0
        ? `<span class="editor-estimate-calc-progress-errors">, ошибок: ${view.errors}</span>`
        : ""
    }
  </span>`;
}

function estimateCalcProgressTotal(items) {
  return (items || []).filter(estimateLineNeedsGsnCalc).length;
}

function calcProgressForUpdate(estimate, statuses, estimateId = "") {
  const key = estimateCalcTargetKey(estimateId || estimate?.id || "");
  if (key && estimateCalcAwaitingServer.has(key)) {
    return estimateCalcProgress(estimate?.items || [], estimateId || estimate?.id || "");
  }
  return calcProgressFromStatusPayload(
    estimate,
    statuses,
    createEstimateCalcStatusResolver(estimate?.items),
  );
}
function calcProgressFromStatusPayload(estimate, statuses, resolveItem) {
  const items = estimate?.items || [];
  let total = estimateCalcProgressTotal(items);
  if (!total) {
    return { total: 0, processed: 0, errors: 0 };
  }
  if (typeof resolveItem !== "function") {
    return estimateCalcProgress(items, estimate?.id || "");
  }
  let processed = 0;
  let errors = 0;
  const matched = new Set();

  (statuses || []).forEach((status) => {
    const item = resolveItem(status);
    if (!item || !estimateLineNeedsGsnCalc(item) || matched.has(item.id)) {
      return;
    }
    matched.add(item.id);
    const nextStatus = String(status.status || "").trim();
    if (estimateCalcTerminalStatus(nextStatus)) {
      processed += 1;
    }
    if (nextStatus === "failed" || nextStatus === "dead") {
      errors += 1;
    }
  });

  return { total, processed, errors };
}

function getEstimateCalcProgressDisplay(estimateId, items) {
  const key = estimateCalcTargetKey(estimateId);
  const target = estimateCalcProgressTargets.get(key);
  const grandTotal = getCalcProgressGrandTotalForDisplay(target, items, estimateId);
  if (target && target.total > 0) {
    const processed = target.processed ?? 0;
    const done = processed >= target.total;
    return {
      total: target.total,
      processed,
      errors: estimateCalcProgressDisplayErrors(target, processed),
      grandTotal,
      done,
    };
  }
  const view = estimateCalcProgressView(items, key);
  if (!view) {
    return null;
  }
  return {
    ...view,
    grandTotal,
  };
}

function updateCalcProgressTarget(estimateId, progress, estimate = null) {
  const key = estimateCalcTargetKey(estimateId);
  const previous = estimateCalcProgressTargets.get(key);
  const itemTotal = estimate ? estimateCalcProgressTotal(estimate.items) : 0;
  const awaiting = key && estimateCalcAwaitingServer.has(key);
  let total = itemTotal || progress.total || previous?.total || 0;
  if (previous?.total > total && (previous.processed ?? 0) < previous.total) {
    total = previous.total;
  }
  const fromItems = estimate ? estimateCalcProgress(estimate.items, key) : progress;
  let processed = fromItems.processed ?? 0;
  let errors = fromItems.errors ?? 0;
  if (!awaiting) {
    processed = Math.max(progress.processed ?? 0, processed, previous?.processed ?? 0);
    errors = Math.max(progress.errors ?? 0, errors, previous?.errors ?? 0);
  }
  if (!total) {
    return;
  }
  const done = processed >= total;
  estimateCalcProgressTargets.set(key, {
    total,
    processed,
    errors,
    displayed: processed,
    largeEstimate: previous?.largeEstimate,
    runningGrandTotal:
      done && estimate
        ? estimateGrandTotalForDisplay(estimate.items)
        : awaiting
          ? (previous?.runningGrandTotal ?? 0)
          : (previous?.runningGrandTotal ?? 0),
    countedDoneLineIds: previous?.countedDoneLineIds ?? new Set(),
  });
  if (done) {
    clearEstimateCalcAwaitingServer(estimateId);
  }
}

function stopCalcProgressAnimation() {
  if (estimateCalcProgressAnimFrame) {
    cancelAnimationFrame(estimateCalcProgressAnimFrame);
    estimateCalcProgressAnimFrame = 0;
  }
}

function startCalcProgressAnimation(estimateId) {
  stopCalcProgressAnimation();
  const key = estimateCalcTargetKey(estimateId);
  const tick = () => {
    estimateCalcProgressAnimFrame = 0;
    const estimate = state.openEstimates.find((item) => sameEstimateId(item.id, estimateId));
    const target = estimateCalcProgressTargets.get(key);
    if (
      !estimate ||
      !target ||
      state.section !== "editor" ||
      !sameEstimateId(state.editorTab, estimateId) ||
      state.estimateViewMode !== "table"
    ) {
      return;
    }

    const step = Math.max(1, Math.ceil(target.total / 160));
    if (target.displayed < target.processed) {
      target.displayed = Math.min(target.processed, target.displayed + step);
      updateEditorCalcProgressDom(estimate);
      estimateCalcProgressAnimFrame = requestAnimationFrame(tick);
      return;
    }

    target.displayed = target.processed;
    updateEditorCalcProgressDom(estimate);
  };
  estimateCalcProgressAnimFrame = requestAnimationFrame(tick);
}

function resetEditorTableInteractionState() {
  editorTableInteractionEnabled = false;
  editorTableBodyReady = false;
  syncEditorTablePassiveState();
}

function enableEditorTableInteraction() {
  if (editorTableInteractionEnabled) {
    return;
  }
  editorTableInteractionEnabled = true;
  syncEditorTablePassiveState();
}

function syncEditorTablePassiveState() {
  const tableBodyScroll = els.editorContent?.querySelector(".editor-estimate-table-body-scroll");
  if (!tableBodyScroll) {
    return;
  }
  const passive = !editorTableBodyReady;
  tableBodyScroll.classList.toggle("editor-table-passive", passive);
  tableBodyScroll.classList.toggle("editor-table-interactive", !passive);
}

function markEditorTableBodyReady() {
  editorTableBodyReady = true;
  enableEditorTableInteraction();
  syncEditorTablePassiveState();
}

function updateEditorCalcProgressDom(estimate) {
  ensureEstimateCalcProgressTarget(estimate.id, estimate);
  const key = estimateCalcTargetKey(estimate.id);
  let view = getEstimateCalcProgressDisplay(estimate.id, estimate.items || []);
  if (!view) {
    view = estimateCalcProgressView(estimate.items || [], key);
  }
  if (!view) {
    const progressRoot = els.editorContent?.querySelector(".editor-estimate-calc-progress");
    if (progressRoot && estimateHasInFlightGsnCalc(estimate.items)) {
      return;
    }
    if (progressRoot) {
      progressRoot.hidden = true;
    }
    return;
  }
  let progressRoot = els.editorContent?.querySelector(".editor-estimate-calc-progress");
  if (!progressRoot) {
    const totalRow = els.editorContent?.querySelector(".editor-estimate-total-row");
    if (totalRow) {
      totalRow.insertAdjacentHTML("beforeend", renderEstimateCalcProgressHTML(estimate.items, estimate.id, view));
      progressRoot = els.editorContent?.querySelector(".editor-estimate-calc-progress");
    }
    if (!progressRoot) {
      const totalOutput = els.editorContent?.querySelector(".editor-estimate-total-value");
      if (totalOutput && view.grandTotal !== undefined) {
        totalOutput.textContent = money.format(view.grandTotal);
      }
      return;
    }
  }
  progressRoot.hidden = false;
  const progressMain = progressRoot.querySelector(".editor-estimate-calc-progress-main");
  if (progressMain) {
    progressMain.textContent = `Обработано позиций: ${view.processed}/${view.total}`;
    progressMain.classList.toggle("editor-estimate-calc-progress-main-ok", view.done);
    progressMain.classList.toggle("editor-estimate-calc-progress-main-danger", !view.done);
  }
  let errorsNode = progressRoot.querySelector(".editor-estimate-calc-progress-errors");
  if (view.errors > 0) {
    const errorsText = `, ошибок: ${view.errors}`;
    if (errorsNode) {
      errorsNode.textContent = errorsText;
    } else {
      progressRoot.insertAdjacentHTML("beforeend", `<span class="editor-estimate-calc-progress-errors">${errorsText}</span>`);
    }
  } else if (errorsNode) {
    errorsNode.remove();
  }
  const totalOutput = els.editorContent?.querySelector(".editor-estimate-total-value");
  if (totalOutput && view.grandTotal !== undefined) {
    totalOutput.textContent = money.format(view.grandTotal);
  }
}

function syncOpenEstimateCalcJsonFromSource(estimateId) {
  const estimate = state.openEstimates.find((item) => sameEstimateId(item.id, estimateId));
  const source = findCachedSourceEstimate(estimateId);
  if (!estimate?.items?.length || !source?.items?.length) {
    return false;
  }
  const sourceById = new Map(source.items.map((item) => [item.id, item]));
  let changed = false;
  estimate.items.forEach((item) => {
    const saved = sourceById.get(item.id);
    if (!saved) {
      return;
    }
    if (saved.calcJson && !item.calcJson) {
      item.calcJson = saved.calcJson;
      item.calcJsonKey = JSON.stringify(saved.calcJson || null);
      changed = true;
    }
    if (saved.calcStatus && !item.calcStatus) {
      item.calcStatus = saved.calcStatus;
      item.calcError = saved.calcError || "";
      changed = true;
    }
    if (estimateLineCalcDone(saved)) {
      if (saved.total != null && item.total !== saved.total) {
        item.total = saved.total;
        changed = true;
      }
      if (saved.unitPrice != null && item.unitPrice !== saved.unitPrice) {
        item.unitPrice = saved.unitPrice;
        changed = true;
      }
      if (saved.quantity != null && item.quantity !== saved.quantity) {
        item.quantity = saved.quantity;
        changed = true;
      }
    }
  });
  return changed;
}

function countDoneGsnLinesMissingCalcJson(items) {
  return (items || []).filter(
    (item) =>
      estimateLineNeedsGsnCalc(item) &&
      estimateLineCalcDone(item) &&
      !parseEstimateCalcJsonRecord(item),
  ).length;
}

let estimateCalcCacheRefreshInflight = null;

function maybeRefreshEstimateCalcCache(estimateId) {
  const estimate = state.openEstimates.find((item) => sameEstimateId(item.id, estimateId));
  if (!estimate || countDoneGsnLinesMissingCalcJson(estimate.items) === 0) {
    return;
  }
  if (syncOpenEstimateCalcJsonFromSource(estimateId)) {
    updateEditorTableTotals(estimate, { skipCacheRefresh: true });
    if (countDoneGsnLinesMissingCalcJson(estimate.items) === 0) {
      return;
    }
  }
  if (estimateCalcCacheRefreshInflight === estimateId) {
    return;
  }
  estimateCalcCacheRefreshInflight = estimateId;
  void refreshConstructionData()
    .then(() => {
      if (!state.openEstimates.some((item) => sameEstimateId(item.id, estimateId))) {
        return;
      }
      syncOpenEstimateCalcJsonFromSource(estimateId);
      const current = state.openEstimates.find((item) => sameEstimateId(item.id, estimateId));
      if (current) {
        updateEditorTableTotals(current, { skipCacheRefresh: true });
      }
    })
    .catch(() => {})
    .finally(() => {
      if (estimateCalcCacheRefreshInflight === estimateId) {
        estimateCalcCacheRefreshInflight = null;
      }
    });
}

function updateEditorTableTotals(estimate, { skipCacheRefresh = false, lite = false } = {}) {
  const key = estimateCalcTargetKey(estimate.id);
  const awaiting = key && estimateCalcAwaitingServer.has(key);
  const target = estimateCalcProgressTargets.get(key);
  const items = estimate.items || [];
  const grandTotal = getCalcProgressGrandTotalForDisplay(target, items, estimate.id);
  const largeCalcActive =
    lite || (isLargeEstimateForCalc(estimate) && estimateHasInFlightGsnCalc(items));
  if (largeCalcActive) {
    updateEditorCalcProgressDom(estimate);
    const totalOutput = els.editorContent?.querySelector(".editor-estimate-total-value");
    if (totalOutput) {
      totalOutput.textContent = money.format(grandTotal);
    }
    return;
  }
  if (!awaiting) {
    syncOpenEstimateCalcJsonFromSource(estimate.id);
  }
  enrichEditorEstimateItems(items);
  updateCalcProgressTarget(estimate.id, estimateCalcProgress(items, key), estimate);
  const totalOutput = els.editorContent?.querySelector(".editor-estimate-total-value");
  if (totalOutput) {
    totalOutput.textContent = money.format(
      getCalcProgressGrandTotalForDisplay(estimateCalcProgressTargets.get(key), items, estimate.id),
    );
  }
  updateEditorCalcProgressDom(estimate);
  if (!skipCacheRefresh && !awaiting) {
    maybeRefreshEstimateCalcCache(estimate.id);
  }
}

function formatEstimateMoney(value) {
  const number = Number(value || 0);
  if (!Number.isFinite(number) || number === 0) {
    return "";
  }
  return estimateCostDetailed.format(number);
}

const SOURCE_DATA_POSITION_CIPHER_SEPARATORS = ["(", " ", "#"];

function extractSourceDataPositionCipher(firstField) {
  const raw = String(firstField || "").trim();
  if (!raw) {
    return "";
  }
  let cutAt = raw.length;
  for (const separator of SOURCE_DATA_POSITION_CIPHER_SEPARATORS) {
    const index = raw.indexOf(separator);
    if (index >= 0 && index < cutAt) {
      cutAt = index;
    }
  }
  return raw.slice(0, cutAt).trim();
}

function extractSourceDataPositionOriginalCode(cipherField) {
  const raw = String(cipherField || "").trim();
  const open = raw.indexOf("(");
  if (open < 0) {
    return "";
  }
  const close = raw.indexOf(")", open + 1);
  if (close < 0) {
    return "";
  }
  return raw.slice(open + 1, close).trim();
}

function parseSourceDataLocalizedNumber(raw) {
  const normalized = String(raw ?? "")
    .trim()
    .replace(/\s/g, "")
    .replace(",", ".");
  if (!normalized) {
    return 0;
  }
  const value = Number(normalized);
  if (!Number.isFinite(value)) {
    throw new Error(`invalid number ${JSON.stringify(raw)}`);
  }
  return value;
}

function isSourceDataQuantityExpression(expr) {
  return expr.includes("(") || /[+\-*/.:]/.test(expr);
}

function roundSourceDataQuantity(value, decimals) {
  const factor = 10 ** decimals;
  return Math.round(value * factor) / factor;
}

function tokenizeSourceDataQuantityExpression(expr) {
  const normalized = String(expr || "").trim().replace(/\s/g, "");
  if (!normalized) {
    throw new Error("empty expression");
  }
  const tokens = [];
  for (let index = 0; index < normalized.length; ) {
    const ch = normalized[index];
    if (ch === "+") {
      tokens.push({ kind: "plus" });
      index += 1;
      continue;
    }
    if (ch === "-") {
      tokens.push({ kind: "minus" });
      index += 1;
      continue;
    }
    if (ch === "*" || ch === ".") {
      tokens.push({ kind: "mul" });
      index += 1;
      continue;
    }
    if (ch === "/" || ch === ":") {
      tokens.push({ kind: "div" });
      index += 1;
      continue;
    }
    if (ch === "(") {
      tokens.push({ kind: "lparen" });
      index += 1;
      continue;
    }
    if (ch === ")") {
      tokens.push({ kind: "rparen" });
      index += 1;
      continue;
    }
    if (/\d|,/.test(ch)) {
      let end = index;
      while (end < normalized.length && /\d|,/.test(normalized[end])) {
        end += 1;
      }
      tokens.push({ kind: "number", value: parseSourceDataLocalizedNumber(normalized.slice(index, end)) });
      index = end;
      continue;
    }
    throw new Error(`invalid character ${JSON.stringify(ch)} in expression ${JSON.stringify(expr)}`);
  }
  return tokens;
}

function evaluateSourceDataQuantityExpression(expr) {
  const tokens = tokenizeSourceDataQuantityExpression(expr);
  let position = 0;

  function parseExpression() {
    let value = parseTerm();
    while (position < tokens.length) {
      const token = tokens[position];
      if (token.kind === "plus") {
        position += 1;
        value += parseTerm();
        continue;
      }
      if (token.kind === "minus") {
        position += 1;
        value -= parseTerm();
        continue;
      }
      break;
    }
    return value;
  }

  function parseTerm() {
    let value = parseFactor();
    while (position < tokens.length) {
      const token = tokens[position];
      if (token.kind === "mul") {
        position += 1;
        value *= parseFactor();
        continue;
      }
      if (token.kind === "div") {
        position += 1;
        const rhs = parseFactor();
        if (rhs === 0) {
          throw new Error("division by zero");
        }
        value /= rhs;
        continue;
      }
      break;
    }
    return value;
  }

  function parseFactor() {
    if (position >= tokens.length) {
      throw new Error("unexpected end of expression");
    }
    const token = tokens[position];
    if (token.kind === "plus") {
      position += 1;
      return parseFactor();
    }
    if (token.kind === "minus") {
      position += 1;
      return -parseFactor();
    }
    if (token.kind === "lparen") {
      position += 1;
      const value = parseExpression();
      if (position >= tokens.length || tokens[position].kind !== "rparen") {
        throw new Error("missing closing parenthesis");
      }
      position += 1;
      return value;
    }
    if (token.kind === "number") {
      position += 1;
      return token.value;
    }
    throw new Error("unexpected token in expression");
  }

  const value = parseExpression();
  if (position !== tokens.length) {
    throw new Error(`unexpected trailing input in ${JSON.stringify(expr)}`);
  }
  return value;
}

function parseSourceDataQuantity(raw) {
  const trimmed = String(raw || "").trim();
  if (!trimmed) {
    return 0;
  }

  let expr = trimmed;
  let decimals = -1;
  const openBracket = expr.lastIndexOf("[");
  if (openBracket >= 0 && expr.endsWith("]")) {
    const inside = expr.slice(openBracket + 1, -1).trim();
    if (!inside) {
      throw new Error(`empty quantity precision in ${JSON.stringify(raw)}`);
    }
    const parsedDecimals = Number(inside);
    if (!Number.isInteger(parsedDecimals) || parsedDecimals < 0) {
      throw new Error(`invalid quantity precision ${JSON.stringify(inside)} in ${JSON.stringify(raw)}`);
    }
    decimals = parsedDecimals;
    expr = expr.slice(0, openBracket).trim();
  }

  let value;
  if (isSourceDataQuantityExpression(expr)) {
    value = evaluateSourceDataQuantityExpression(expr);
  } else {
    value = parseSourceDataLocalizedNumber(expr);
  }
  if (!Number.isFinite(value)) {
    throw new Error(`quantity is not finite in ${JSON.stringify(raw)}`);
  }
  if (decimals >= 0) {
    value = roundSourceDataQuantity(value, decimals);
  }
  return value;
}

function resolveEstimateItemQuantity(item) {
  const stored = Number(item?.quantity || 0);
  if (stored) {
    return stored;
  }
  const rawText = String(item?.rawText || "").trim();
  if (!rawText || rawText.startsWith("Р") || rawText.startsWith("ПР")) {
    return stored;
  }
  const fields = splitEstimateTextLine(rawText).map(unescapeEstimateTextField);
  const quantityRaw = fields.length > 1 ? String(fields[1] ?? "").trim() : "";
  if (!quantityRaw) {
    return stored;
  }
  try {
    return parseSourceDataQuantity(quantityRaw);
  } catch {
    return stored;
  }
}

function parseSourceDataPositionLineFields(fields) {
  const cipherRaw = String(fields[0] || "").trim();
  const quantityRaw = fields.length > 1 ? String(fields[1] ?? "").trim() : "";
  const totalRaw = fields.length > 2 ? String(fields[2] ?? "").trim() : "";
  const nameRaw = fields.length > 3 ? String(fields[3] ?? "").trim() : "";
  const unitRaw = fields.length > 4 ? String(fields[4] ?? "").trim() : "";
  const sourceCode = extractSourceDataPositionCipher(cipherRaw);
  const originalCode = extractSourceDataPositionOriginalCode(cipherRaw);
  const hasTotal = totalRaw !== "";
  const total = hasTotal ? parseEstimateTextNumber(totalRaw) : 0;
  const hasName = nameRaw !== "";
  const hasUnit = unitRaw !== "";
  const hasQuantity = quantityRaw !== "";
  let quantity = 0;
  if (hasQuantity) {
    try {
      quantity = parseSourceDataQuantity(quantityRaw);
    } catch (error) {
      throw new Error(`Объём: ${error.message || "не удалось разобрать выражение"}`);
    }
  }

  return {
    cipherRaw,
    sourceCode,
    originalCode,
    hasOriginalCode: originalCode !== "",
    quantityRaw,
    quantity,
    hasQuantity,
    hasTotal,
    total,
    hasName,
    name: nameRaw,
    hasUnit,
    unit: unitRaw,
    unitPrice: 0,
  };
}

function parseSourceDataPositionLineFieldsFromRawText(rawText) {
  const line = String(rawText || "").trim();
  if (!line || line.startsWith("Р") || line.startsWith("ПР")) {
    return null;
  }
  const fields = splitEstimateTextLine(line).map(unescapeEstimateTextField);
  if (fields.length < 2) {
    return null;
  }
  try {
    return parseSourceDataPositionLineFields(fields);
  } catch {
    return null;
  }
}

function buildGsnPositionItemFromSourceDataLine({ existing, fields, rawText, lineIndex }) {
  const parsed = parseSourceDataPositionLineFields(fields);
  const rawTextUnchanged = existing && String(existing.rawText || "") === String(rawText || "");
  const calcDone = rawTextUnchanged && estimateLineCalcDone(existing);

  const name = parsed.hasName ? parsed.name : calcDone ? existing.name || "" : "";
  const unit = parsed.hasUnit ? parsed.unit : calcDone ? existing.unit || "" : "";
  const originalCode = calcDone ? existing.originalCode || "" : "";
  const total = parsed.hasTotal ? parsed.total : calcDone ? Number(existing.total || 0) : 0;
  const unitPrice = calcDone ? Number(existing.unitPrice || 0) : 0;
  const quantity = rawTextUnchanged
    ? Number(existing?.quantity || 0)
    : parsed.hasQuantity
      ? parsed.quantity
      : 0;

  return {
    id: existing?.id || `line_${lineIndex}`,
    type: "position",
    source: "gsn",
    sourceId: parsed.sourceCode,
    sourceCode: parsed.cipherRaw || parsed.sourceCode,
    code: parsed.sourceCode,
    recordCode: parsed.sourceCode,
    originalCode,
    name,
    unit,
    quantity,
    unitPrice,
    total,
    hasResources: calcDone ? Boolean(existing.hasResources) : false,
    children: calcDone ? existing.children : undefined,
    calcStatus: rawTextUnchanged ? existing?.calcStatus || "" : "",
    calcError: rawTextUnchanged ? existing?.calcError || "" : "",
    calcJson: rawTextUnchanged ? existing?.calcJson || null : null,
    rawText,
  };
}

function hydrateEstimateItemFromSourceDataRawText(item) {
  if (!item?.rawText || estimateLineIsStructural(item)) {
    return item;
  }
  const parsed = parseSourceDataPositionLineFieldsFromRawText(item.rawText);
  if (!parsed) {
    return item;
  }
  item.sourceCode = parsed.cipherRaw || parsed.sourceCode;
  item.code = parsed.sourceCode;
  item.recordCode = parsed.sourceCode;
  if (parsed.hasName) {
    item.name = parsed.name;
  }
  if (parsed.hasUnit) {
    item.unit = parsed.unit;
  }
  if (parsed.hasTotal) {
    item.total = parsed.total;
  }
  if (!String(item.source || "").trim() && item.type === "position") {
    item.source = "gsn";
  }
  return item;
}

function estimateLineSourceCode(item) {
  return String(item?.sourceCode || item?.recordCode || item?.code || "").trim();
}

function estimateLineNormalizedSourceCode(item) {
  return extractSourceDataPositionCipher(estimateLineSourceCode(item));
}

function estimateLineSourceCodeFromRawText(rawText) {
  const line = String(rawText || "").trim();
  if (!line || line.startsWith("Р") || line.startsWith("ПР")) {
    return "";
  }
  const fields = splitEstimateTextLine(line).map(unescapeEstimateTextField);
  return String(fields[0] || "").trim();
}

function estimateSourceDataPositionCipherRaw(item) {
  const rawText = String(item?.rawText || "").trim();
  if (rawText && !rawText.startsWith("Р") && !rawText.startsWith("ПР")) {
    const fields = splitEstimateTextLine(rawText).map(unescapeEstimateTextField);
    if (fields[0]) {
      return String(fields[0]).trim();
    }
  }
  return estimateSourceDataPositionCode(item);
}

function estimateSourceDataQuantityRaw(item) {
  const rawText = String(item?.rawText || "").trim();
  if (rawText && !rawText.startsWith("Р") && !rawText.startsWith("ПР")) {
    const fields = splitEstimateTextLine(rawText).map(unescapeEstimateTextField);
    if (fields[1] != null && String(fields[1]).trim() !== "") {
      return String(fields[1]).trim();
    }
  }
  return String(item?.quantity ?? 0);
}

function estimateLineGsnDisplayCodeAfterCalc(item) {
  const originalCode = String(item?.originalCode || "").trim();
  if (originalCode) {
    return originalCode;
  }
  const record = parseEstimateCalcJsonRecord(item);
  if (record) {
    const recordOriginal = String(record.originalCode || "").trim();
    if (recordOriginal) {
      return recordOriginal;
    }
    return String(record.code || item?.code || "").trim();
  }
  return String(item?.code || "").trim();
}

function estimateLineDisplayCode(item) {
  if (estimateLineShowsGsnPartial(item)) {
    return extractSourceDataPositionCipher(estimateLineSourceCode(item)) || estimateLineSourceCode(item);
  }
  if (estimateLineIsGsn(item) && estimateLineCalcDone(item)) {
    return estimateLineGsnDisplayCodeAfterCalc(item);
  }
  return String(item?.code || "").trim();
}

function estimateRecordFetchCode(item) {
  return String(item?.recordCode || item?.sourceCode || item?.code || "").trim();
}

function estimateRecordLookupCode(item) {
  return estimateRecordFetchCode(item) || estimateLineDisplayCode(item);
}

function applyRecordDetailToEstimateItem(item, record) {
  if (!item || !record) {
    return;
  }
  const sourceCode = estimateLineSourceCode(item);
  const sourceFields = parseSourceDataPositionLineFieldsFromRawText(item.rawText);
  if (record.code) {
    item.recordCode = record.code;
    item.code = record.code;
  }
  item.originalCode = String(record.originalCode || "").trim();
  if (sourceCode) {
    item.sourceCode = sourceCode;
  }
  if (record.name && !sourceFields?.hasName) {
    item.name = record.name;
  }
  if (record.unit && !sourceFields?.hasUnit) {
    item.unit = record.unit;
  }
  item.isWork = record.isWork !== false;
  if (!item.isWork) {
    item.unitPriceText = record.unitPriceText || "";
    item.unitPriceIndex = record.unitPriceIndex || "";
    item.hasResources = false;
    item.children = undefined;
  }
}

function estimateLineIsStructural(item) {
  return item?.type === "section" || item?.type === "subsection";
}

function estimateLineStructuralLabel(item) {
  if (item?.type === "section") {
    return "Раздел";
  }
  if (item?.type === "subsection") {
    return "Подраздел";
  }
  return "";
}

function estimateLineHasResources(item) {
  return Boolean(item?.hasResources || item?.children?.length);
}

function estimateLineCanExpand(item) {
  if (estimateLineShowsGsnPartial(item)) {
    return false;
  }
  if (estimateLineIsResourcePosition(item)) {
    return false;
  }
  if (item?.source === "user_position") {
    return false;
  }
  if (!estimateLineIsWorkPosition(item)) {
    return false;
  }
  if (estimateLineHasResources(item)) {
    return true;
  }
  return Boolean(estimateRecordLookupCode(item));
}

function estimateLineIsResourcePosition(item) {
  if (item?.isWork === false) {
    return true;
  }
  if (item?.isWork === true) {
    return false;
  }
  const code = estimateRecordFetchCode(item);
  return /^[СCМMТT]\d/.test(code);
}

function estimateLineIsWorkPosition(item) {
  if (item?.type !== "position" || item?.source === "user_position") {
    return false;
  }
  if (estimateLineIsResourcePosition(item)) {
    return false;
  }
  if (item.isWork === true) {
    return true;
  }
  const code = estimateRecordFetchCode(item);
  return /^[ЕУEУЦ]/.test(code);
}

function estimateLineKindLabel(line) {
  if (!line) {
    return "";
  }
  if (line.type === "section") {
    return "Раздел";
  }
  if (line.type === "subsection") {
    return "Подраздел";
  }
  if (estimateLineIsWorkPosition(line)) {
    return "Позиция-работа";
  }
  if (line.type === "position") {
    return "Позиция";
  }
  return line.type || "";
}

function formatEstimateLineQuantityInput(value) {
  const number = Number(value);
  if (!Number.isFinite(number)) {
    return "";
  }
  if (number === 0) {
    return "0";
  }
  return quantityInputFormat.format(number);
}

function formatEstimateLinePriceInput(line) {
  const value = Number(line?.unitPrice || 0);
  if (!Number.isFinite(value) || value === 0) {
    return "";
  }
  return estimateCostDetailed.format(value);
}

function configureEstimateLineDialogForm(options) {
  const { mode, line } = options;
  const form = els.estimateLineDialogForm;
  const typeReadonly = form.querySelector("[data-estimate-line-type-readonly]");
  const positionFields = form.querySelectorAll("[data-estimate-line-field]");

  if (mode === "edit") {
    typeReadonly.classList.remove("hidden");
    els.estimateLineTypeLabel.textContent = estimateLineKindLabel(line);

    const isPosition = line?.type === "position";
    positionFields.forEach((field) => {
      const fieldName = field.dataset.estimateLineField;
      const show = isPosition && (fieldName !== "unitPrice" || !estimateLineIsWorkPosition(line));
      field.classList.toggle("hidden", !show);
    });
    els.estimateLineDialogSubmit.textContent = "Сохранить";
    return;
  }

  typeReadonly.classList.add("hidden");
  positionFields.forEach((field) => {
    field.classList.add("hidden");
  });
  els.estimateLineDialogSubmit.textContent = "Добавить";
}

function getExpandedLineIds(estimateId) {
  if (!state.estimateLinesExpanded[estimateId]) {
    state.estimateLinesExpanded[estimateId] = [];
  }
  return new Set(state.estimateLinesExpanded[estimateId]);
}

function saveEstimateLinesExpanded() {
  sessionStorage.setItem("nav_editor_expanded", JSON.stringify(state.estimateLinesExpanded));
}

function parseResourceQuantityText(value) {
  const text = String(value || "").trim().replace(/\s/g, "").replace(",", ".");
  if (!text || text.toUpperCase() === "П") {
    return 0;
  }
  const number = Number(text);
  return Number.isFinite(number) ? number : 0;
}

function formatEstimateUnitPrice(item) {
  const normalized = normalizeResourcePricing(item);
  const base = String(normalized?.unitPriceText || "").trim();
  const index = String(normalized?.unitPriceIndex || "").trim();
  if (index) {
    const basePart = base ? escapeHTML(base) : "";
    const indexPart = `<span class="editor-estimate-price-index">*${escapeHTML(index)}</span>`;
    return basePart ? `${basePart}<br>${indexPart}` : indexPart;
  }
  if (base) {
    return escapeHTML(base);
  }
  const value = Number(normalized?.unitPrice || 0);
  return value ? estimateCostDetailed.format(value) : "";
}

function estimateChildFromResource(resource, index, parentId) {
  return {
    id: `${parentId}_res_${index}`,
    type: "resource",
    code: resource.code || "",
    originalCode: resource.originalCode || "",
    name: resource.name || "",
    unit: resource.unit || "",
    quantity: parseResourceQuantityText(resource.quantityText),
    unitPrice: 0,
    unitPriceText: resource.unitPriceText || "",
    unitPriceIndex: resource.unitPriceIndex || "",
    total: 0,
  };
}

function estimateLineNeedsDistrictPricing(item) {
  return (
    item?.type === "position" &&
    item?.source !== "user_position" &&
    !estimateLineShowsGsnPartial(item) &&
    Boolean(estimateRecordLookupCode(item))
  );
}

function refreshEstimateItemChildrenPricing(item, resources) {
  const existingChildren = item.children || [];
  const existingByCode = new Map(
    existingChildren.map((child) => [String(child.code || child.originalCode || ""), child]),
  );

  item.children = resources.map((resource, index) => {
    const next = estimateChildFromResource(resource, index, item.id);
    const codeKey = String(next.code || next.originalCode || "");
    const existing = existingByCode.get(codeKey) || existingChildren[index];
    if (existing?.id) {
      next.id = existing.id;
    }
    return next;
  });
  item.hasResources = resources.length > 0;
}

async function recalculateEstimatePricing(estimate) {
  const items = estimate?.items || [];
  if (!items.length) {
    return;
  }

  const district = estimate.district || "";
  const fgisSetId = estimate.fgisSetId || "";
  const tasks = items
    .filter((item) => estimateLineNeedsDistrictPricing(item))
    .map(async (item) => {
      try {
        const code = estimateRecordLookupCode(item);
        if (!code) {
          return;
        }
        const record = await fetchGSNRecordDetail(code, fgisSetId, district);
        if (!record) {
          return;
        }
        applyRecordDetailToEstimateItem(item, record);
        if (record.isWork === false) {
          return;
        }
        if (record.resources?.length) {
          refreshEstimateItemChildrenPricing(item, record.resources);
        }
      } catch {
        return;
      }
    });

  await Promise.all(tasks);
}

function applyEstimateCalcRecord(item, calcJson) {
  const record = parseEstimateCalcJsonRecord({ calcJson });
  if (!record) {
    applyEstimateCalcPricingFromJson(item, calcJson);
    return Boolean(calcJson);
  }
  applyRecordDetailToEstimateItem(item, record);
  if (record.isWork !== false && record.resources?.length) {
    refreshEstimateItemChildrenPricing(item, record.resources);
  }
  applyEstimateCalcPricingFromJson(item, calcJson);
  item.calcRecordAppliedKey = JSON.stringify(calcJson || null);
  return true;
}

function ensureEstimateLineCalcPricingApplied(item) {
  if (!estimateCalcRecordNeedsApply(item)) {
    return false;
  }
  return applyEstimateCalcRecord(item, item.calcJson);
}

function estimateCalcRecordNeedsApply(item) {
  if (!estimateLineCalcDone(item) || !item?.calcJson) {
    return false;
  }
  const calcKey = item.calcJsonKey || JSON.stringify(item.calcJson || null);
  if (item.calcRecordAppliedKey !== calcKey) {
    return true;
  }
  return !estimateLineHasAppliedPricing(item);
}

function applyEstimateCalcRecordsBatch(estimate, statuses, resolveItem = null) {
  const resolve = resolveItem || createEstimateCalcStatusResolver(estimate?.items);
  let changed = false;

  (statuses || []).forEach((status) => {
    if (status.status !== "done" || !status.calcJson) {
      return;
    }
    const item = resolve(status);
    if (!item) {
      return;
    }
    if (applyEstimateCalcRecord(item, status.calcJson)) {
      changed = true;
    }
  });

  return changed;
}

function shouldPreserveLocalGsnCalcOnMerge(local, saved) {
  if (!local || !estimateLineIsGsn(local) || !estimateLineCalcDone(local)) {
    return false;
  }
  const savedStatus = String(saved?.calcStatus || "").trim();
  if (savedStatus === "done" && local.calcJson && !saved.calcJson) {
    return true;
  }
  return false;
}

function applyEstimateCalcStatusMetaOnly(estimate, statuses) {
  const itemsById = new Map((estimate?.items || []).map((item) => [item.id, item]));
  let changed = false;

  (statuses || []).forEach((status) => {
    const item = itemsById.get(status.lineId);
    if (!item) {
      return;
    }
    const nextStatus = status.status || "";
    const nextError = status.error || "";
    const nextCalcKey = JSON.stringify(status.calcJson || null);
    if (item.calcStatus !== nextStatus || item.calcError !== nextError || item.calcJsonKey !== nextCalcKey) {
      item.calcStatus = nextStatus;
      item.calcError = nextError;
      item.calcJson = status.calcJson || null;
      item.calcJsonKey = nextCalcKey;
      changed = true;
    }
    if (nextStatus === "done") {
      applyEstimateCalcLineResult(item, status);
      applyEstimateCalcPricingFromJson(item, status.calcJson);
    }
  });

  return changed;
}

function applyEstimateCalcStatusFieldsOnly(estimate, statuses, resolveItem = null) {
  const resolve = resolveItem || createEstimateCalcStatusResolver(estimate?.items);
  let changed = false;

  (statuses || []).forEach((status) => {
    const item = resolve(status);
    if (!item) {
      return;
    }
    const nextStatus = status.status || "";
    const nextError = status.error || "";
    const nextCalcKey = JSON.stringify(status.calcJson || null);
    if (item.calcStatus !== nextStatus || item.calcError !== nextError || item.calcJsonKey !== nextCalcKey) {
      item.calcStatus = nextStatus;
      item.calcError = nextError;
      item.calcJson = status.calcJson || null;
      item.calcJsonKey = nextCalcKey;
      changed = true;
    }
    applyEstimateCalcLineResult(item, status);
    if (nextStatus === "done") {
      if (status.calcJson) {
        if (applyEstimateCalcRecord(item, status.calcJson)) {
          changed = true;
        }
      } else {
        applyEstimateCalcPricingFromJson(item, status.calcJson);
      }
    }
  });

  return changed;
}

function calcStatusNeedsApply(item, status) {
  if (!item || !status) {
    return false;
  }
  const nextStatus = String(status.status || "").trim();
  const nextError = String(status.error || "").trim();
  const nextCalcKey = JSON.stringify(status.calcJson || null);
  if (item.calcStatus !== nextStatus || item.calcError !== nextError || item.calcJsonKey !== nextCalcKey) {
    return true;
  }
  return nextStatus === "done" && status.calcJson && estimateCalcRecordNeedsApply(item);
}

function matchCalcStatusesToItems(items, statuses) {
  const gsnItems = (items || []).filter(estimateLineNeedsGsnCalc);
  const gsnStatuses = (statuses || []).filter((status) => String(status?.code || "").trim());
  const statusByLineId = new Map();
  gsnStatuses.forEach((status) => {
    if (status?.lineId) {
      statusByLineId.set(status.lineId, status);
    }
  });

  const pairs = [];
  const matchedStatusIds = new Set();
  const unmatchedItems = [];

  gsnItems.forEach((item) => {
    const status = statusByLineId.get(item.id);
    if (status) {
      pairs.push({ item, status });
      matchedStatusIds.add(status.lineId);
      return;
    }
    unmatchedItems.push(item);
  });

  const unmatchedStatuses = gsnStatuses.filter((status) => !matchedStatusIds.has(status.lineId));
  const zipCount = Math.min(unmatchedItems.length, unmatchedStatuses.length);
  for (let index = 0; index < zipCount; index += 1) {
    pairs.push({ item: unmatchedItems[index], status: unmatchedStatuses[index] });
  }
  return pairs;
}

function applyEstimateCalcStatusPairs(estimate, pairs, { skipRecords = false } = {}) {
  let changed = false;
  (pairs || []).forEach(({ status, item }) => {
    if (!calcStatusNeedsApply(item, status)) {
      return;
    }
    const nextStatus = status.status || "";
    const nextError = status.error || "";
    const nextCalcKey = JSON.stringify(status.calcJson || null);
    if (item.calcStatus !== nextStatus || item.calcError !== nextError || item.calcJsonKey !== nextCalcKey) {
      item.calcStatus = nextStatus;
      item.calcError = nextError;
      item.calcJson = status.calcJson || null;
      item.calcJsonKey = nextCalcKey;
      changed = true;
    }
    applyEstimateCalcLineResult(item, status);
    if (nextStatus === "done") {
      if (!skipRecords && status.calcJson) {
        if (applyEstimateCalcRecord(item, status.calcJson)) {
          changed = true;
        }
      } else {
        applyEstimateCalcPricingFromJson(item, status.calcJson);
      }
    }
  });
  return changed;
}

function refreshEstimateTableRowFromItem(row, item) {
  if (!row || !item) {
    return;
  }
  const codeEl = row.querySelector(".user-positions-code-text");
  if (codeEl) {
    codeEl.innerHTML = formatBreakableCode(estimateLineDisplayCode(item));
  }
  const isPartialGsn = estimateLineShowsGsnPartial(item);
  const nameCell = row.querySelector(".editor-estimate-name-cell");
  if (nameCell) {
    nameCell.innerHTML = isPartialGsn
      ? renderEstimateCalcStatusBadge(item)
      : `${escapeHTML(item.name || "")}${renderEstimateCalcStatusBadge(item)}`;
  }
  const unitCell = row.querySelector(".editor-estimate-unit-cell");
  if (unitCell) {
    unitCell.textContent = isPartialGsn ? "" : item.unit || "";
  }
  const quantityCell = row.querySelector(".editor-estimate-quantity-cell");
  if (quantityCell) {
    quantityCell.textContent = estimateLineIsStructural(item) ? "" : formatNumber(item.quantity);
  }
  const priceCell = row.querySelector(".editor-estimate-price-cell");
  if (priceCell) {
    priceCell.textContent = isPartialGsn ? "" : formatEstimateUnitPrice(item);
  }
  const totalCell = row.querySelector(".editor-estimate-total-cell");
  if (totalCell) {
    totalCell.textContent = isPartialGsn ? "" : formatEstimateMoney(estimateLineTotal(item));
  }
}

function refreshAllRenderedEstimateTableRows(estimate) {
  if (
    state.section !== "editor" ||
    state.editorTab !== estimate.id ||
    state.estimateViewMode !== "table" ||
    !isEditorContentMountedForEstimate(estimate.id)
  ) {
    return 0;
  }
  const tbody = els.editorContent?.querySelector(".editor-estimate-table tbody");
  if (!tbody) {
    return 0;
  }
  const itemsById = new Map((estimate.items || []).map((item) => [item.id, item]));
  let refreshed = 0;
  tbody.querySelectorAll(`[data-editor-line-edit="${CSS.escape(estimate.id)}"]`).forEach((button) => {
    const item = itemsById.get(button.dataset.editorLineId);
    const row = button.closest("tr");
    if (!item || !row) {
      return;
    }
    refreshEstimateTableRowFromItem(row, item);
    refreshed += 1;
  });
  return refreshed;
}

function syncLargeEstimateTableRows(estimate) {
  if (
    state.section !== "editor" ||
    state.editorTab !== estimate.id ||
    state.estimateViewMode !== "table" ||
    !isEditorContentMountedForEstimate(estimate.id)
  ) {
    return;
  }
  updateEditorTableContent(estimate, { refreshBody: true });
}

function scheduleLargeEstimateTableRowRefresh(estimate) {
  let frames = 0;
  const tick = () => {
    frames += 1;
    refreshAllRenderedEstimateTableRows(estimate);
    const tbody = els.editorContent?.querySelector(".editor-estimate-table tbody");
    const rendered =
      tbody?.querySelectorAll(`[data-editor-line-edit="${CSS.escape(estimate.id)}"]`).length || 0;
    const total = (estimate.items || []).length;
    if (frames < 240 && rendered < total) {
      requestAnimationFrame(tick);
    }
  };
  requestAnimationFrame(tick);
}

async function applyIncrementalEstimateCalcUpdates(estimate, statuses) {
  const estimateId = estimate.id;
  const large = isLargeEstimateForCalc(estimate);
  enrichEditorEstimateItems(estimate.items);
  const pairs = matchCalcStatusesToItems(estimate.items, statuses).filter(({ item, status }) =>
    calcStatusNeedsApply(item, status),
  );
  if (!pairs.length) {
    return { applied: 0, remaining: 0 };
  }

  if (large) {
    const yieldEvery = 500;
    for (let index = 0; index < pairs.length; index += yieldEvery) {
      const batch = pairs.slice(index, index + yieldEvery);
      applyEstimateCalcStatusPairs(estimate, batch, { skipRecords: true });
      if (index + yieldEvery < pairs.length) {
        await new Promise((resolve) => setTimeout(resolve, 0));
      }
    }
    enrichEditorEstimateItems(estimate.items);
    ensureEstimateCalcProgressTarget(estimateId, estimate);
    accumulateCalcGrandTotalFromItems(estimateId, estimate.items);
    updateCalcProgressTarget(estimateId, estimateCalcProgress(estimate.items, estimateId), estimate);
    if (
      state.section === "editor" &&
      sameEstimateId(state.editorTab, estimateId) &&
      state.estimateViewMode === "table"
    ) {
      updateEditorCalcProgressDom(estimate);
      updateEditorTableTotals(estimate, { skipCacheRefresh: true, lite: true });
      refreshAllRenderedEstimateTableRows(estimate);
      scheduleLargeEstimateTableRowRefresh(estimate);
      const progress = estimateCalcProgress(estimate.items, estimateId);
      if (progress.total > 0 && progress.processed >= progress.total) {
        syncLargeEstimateTableRows(estimate);
      }
    }
    return { applied: pairs.length, remaining: 0 };
  }

  const chunkSize = 15;
  let applied = 0;

  for (let index = 0; index < pairs.length; index += chunkSize) {
    const batch = pairs.slice(index, index + chunkSize);
    applyEstimateCalcStatusPairs(estimate, batch);
    enrichEditorEstimateItems(estimate.items);
    ensureEstimateCalcProgressTarget(estimateId, estimate);
    accumulateCalcGrandTotalFromItems(estimateId, estimate.items);
    updateCalcProgressTarget(estimateId, estimateCalcProgress(estimate.items, estimateId), estimate);
    applied += batch.length;

    if (
      state.section === "editor" &&
      sameEstimateId(state.editorTab, estimateId) &&
      state.estimateViewMode === "table"
    ) {
      startCalcProgressAnimation(estimateId);
      updateEditorCalcProgressDom(estimate);
      updateEditorTableTotals(estimate, { skipCacheRefresh: true, lite: false });
      refreshEstimateTableCodeCells(
        estimate,
        batch.map(({ item }) => item.id),
      );
    }

    if (index + chunkSize < pairs.length) {
      await new Promise((resolve) => requestAnimationFrame(resolve));
    }
  }

  return { applied, remaining: 0 };
}

function applyEstimateCalcStatuses(estimate, statuses) {
  return applyEstimateCalcStatusFieldsOnly(estimate, statuses);
}

async function finalizeEstimateCalcIfComplete(estimate, statuses) {
  const estimateId = estimate.id;
  const progress = estimateCalcProgress(estimate.items, estimateId);
  if (!progress.total || progress.processed < progress.total) {
    return;
  }
  const large = isLargeEstimateForCalc(estimate);
  const donePairs = matchCalcStatusesToItems(estimate.items, statuses).filter(
    ({ status }) => status.status === "done" && status.calcJson,
  );
  const chunkSize = estimateCalcApplyChunkSize(estimate);
  for (let index = 0; index < donePairs.length; index += chunkSize) {
    await new Promise((resolve) => requestAnimationFrame(resolve));
    const batch = donePairs.slice(index, index + chunkSize);
    applyEstimateCalcStatusPairs(estimate, batch);
    if (
      state.section === "editor" &&
      sameEstimateId(state.editorTab, estimateId) &&
      state.estimateViewMode === "table" &&
      !large
    ) {
      updateEditorTableTotals(estimate, { skipCacheRefresh: true });
    }
  }
  saveEditorState();
  if (
    state.section === "editor" &&
    sameEstimateId(state.editorTab, estimateId) &&
    state.estimateViewMode === "table" &&
    isEditorContentMountedForEstimate(estimateId)
  ) {
    if (large) {
      refreshEstimateTableCalcVisuals(estimate);
      syncLargeEstimateTableRows(estimate);
    } else {
      updateEditorTableContent(estimate, { refreshBody: true });
    }
  }
}

async function pollEstimateCalcStatus(estimateId) {
  if (!state.me || !isPersistedEstimateId(estimateId) || estimateCalcStatusPollInflight === estimateId) {
    return;
  }
  const estimate = state.openEstimates.find((item) => sameEstimateId(item.id, estimateId));
  if (!estimate) {
    return;
  }

  const key = estimateCalcTargetKey(estimateId);
  const calcInFlight = estimateHasInFlightGsnCalc(estimate.items);

  estimateCalcStatusPollInflight = estimateId;
  try {
    const large = isLargeEstimateForCalc(estimate);
    if (!large && !calcInFlight) {
      syncOpenEstimateCalcJsonFromSource(estimateId);
    }
    const response = await api(
      `/api/estimates/${estimateId}/calc-status${large ? "?lite=1" : ""}`,
    );
    const statuses = response.items || [];
    if (estimateCalcStatusPollInflight === estimateId) {
      estimateCalcStatusPollInflight = null;
    }
    enrichEditorEstimateItems(estimate.items);
    await applyIncrementalEstimateCalcUpdates(estimate, statuses);

    ensureEstimateCalcProgressTarget(estimateId, estimate);
    updateCalcProgressTarget(estimateId, estimateCalcProgress(estimate.items, estimateId), estimate);

    const progress = estimateCalcProgress(estimate.items, estimateId);
    if (
      large &&
      progress.processed > 0 &&
      state.section === "editor" &&
      sameEstimateId(state.editorTab, estimateId) &&
      state.estimateViewMode === "table"
    ) {
      refreshAllRenderedEstimateTableRows(estimate);
      if (progress.total > 0 && progress.processed >= progress.total) {
        syncLargeEstimateTableRows(estimate);
      }
    }

    const hasCalcActivity =
      progress.total > 0 ||
      estimateCalcProgressTargets.get(key)?.total > 0 ||
      calcInFlight ||
      progress.processed > 0;

    if (
      hasCalcActivity &&
      state.section === "editor" &&
      sameEstimateId(state.editorTab, estimateId) &&
      state.estimateViewMode === "table"
    ) {
      startCalcProgressAnimation(estimateId);
      updateEditorCalcProgressDom(estimate);
      updateEditorTableTotals(estimate, { skipCacheRefresh: true, lite: large });
    }

    if (progress.total > 0 && progress.processed >= progress.total) {
      await finalizeEstimateCalcIfComplete(estimate, statuses);
    } else if (progress.processed > 0) {
      saveEditorState();
    }
  } catch {
    const failedEstimate = state.openEstimates.find((item) => sameEstimateId(item.id, estimateId));
    if (
      failedEstimate &&
      state.section === "editor" &&
      sameEstimateId(state.editorTab, estimateId) &&
      state.estimateViewMode === "table"
    ) {
      ensureEstimateCalcProgressTarget(estimateId, failedEstimate);
      updateEditorCalcProgressDom(failedEstimate);
    }
    return;
  } finally {
    if (estimateCalcStatusPollInflight === estimateId) {
      estimateCalcStatusPollInflight = null;
    }
  }
}

function refreshEstimateTableCodeCells(estimate, itemIds = null) {
  if (
    state.section !== "editor" ||
    state.editorTab !== estimate.id ||
    state.estimateViewMode !== "table" ||
    !isEditorContentMountedForEstimate(estimate.id)
  ) {
    return;
  }
  const filter = itemIds ? new Set(itemIds) : null;
  const rowByLineId = new Map();
  const editButtons = els.editorContent?.querySelectorAll(
    `[data-editor-line-edit="${CSS.escape(estimate.id)}"]`,
  );
  if (editButtons) {
    for (const button of editButtons) {
      const lineId = button.dataset.editorLineId;
      if (lineId) {
        rowByLineId.set(lineId, button.closest("tr"));
      }
    }
  }
  for (const item of estimate.items || []) {
    if (filter && !filter.has(item.id)) {
      continue;
    }
    const row = rowByLineId.get(item.id);
    if (!row) {
      continue;
    }
    refreshEstimateTableRowFromItem(row, item);
  }
}

function collectEstimateCalcVisualLineIds(statuses, resolveOrLookup, { doneOnly = false } = {}) {
  const resolveItem =
    typeof resolveOrLookup === "function"
      ? resolveOrLookup
      : (status) => resolveEstimateItemForCalcStatus(resolveOrLookup, status);
  const lineIds = [];
  (statuses || []).forEach((status) => {
    const item = resolveItem(status);
    if (!item || !estimateLineNeedsGsnCalc(item)) {
      return;
    }
    const nextStatus = String(status.status || "").trim();
    if (doneOnly && nextStatus !== "done") {
      return;
    }
    if (!doneOnly && !estimateCalcTerminalStatus(nextStatus)) {
      return;
    }
    lineIds.push(item.id);
  });
  return lineIds;
}

function refreshEstimateTableCalcVisuals(estimate, { lineIds = null } = {}) {
  if (
    state.section !== "editor" ||
    state.editorTab !== estimate.id ||
    state.estimateViewMode !== "table" ||
    !isEditorContentMountedForEstimate(estimate.id)
  ) {
    return;
  }
  const large = isLargeEstimateForCalc(estimate);
  updateEditorTableTotals(estimate, { skipCacheRefresh: true, lite: large });
  refreshAllRenderedEstimateTableRows(estimate);
  if (!large) {
    refreshEstimateTableCodeCells(estimate, lineIds);
  }
}

function stopEstimateCalcStatusPolling() {
  if (estimateCalcStatusPollTimer) {
    window.clearInterval(estimateCalcStatusPollTimer);
    estimateCalcStatusPollTimer = 0;
  }
  stopCalcProgressAnimation();
  estimateCalcApplyToken += 1;
}

function restartEstimateCalcStatusPolling() {
  if (estimateCalcStatusPollTimer) {
    window.clearInterval(estimateCalcStatusPollTimer);
    estimateCalcStatusPollTimer = 0;
  }
  stopCalcProgressAnimation();
  const estimateId = state.section === "editor" && state.estimateViewMode === "table" ? state.editorTab : "";
  if (!estimateId || estimateId === "buffer" || !isPersistedEstimateId(estimateId)) {
    return;
  }
  if (persistEstimateInflight.has(estimateId) || estimateCalcPollBlockedByPersist.has(estimateId)) {
    return;
  }
  const estimate = state.openEstimates.find((item) => sameEstimateId(item.id, estimateId));
  if (estimate) {
    ensureEstimateCalcProgressTarget(estimateId, estimate);
    updateEditorCalcProgressDom(estimate);
  }
  void pollEstimateCalcStatus(estimateId);
  const pollMs = estimate ? estimateCalcStatusPollIntervalMs(estimate) : ESTIMATE_CALC_STATUS_POLL_MS;
  estimateCalcStatusPollTimer = window.setInterval(() => {
    if (
      state.section !== "editor" ||
      state.estimateViewMode !== "table" ||
      !sameEstimateId(state.editorTab, estimateId)
    ) {
      stopEstimateCalcStatusPolling();
      return;
    }
    void pollEstimateCalcStatus(estimateId);
  }, pollMs);
}

function syncEstimateCalcStatusPolling() {
  restartEstimateCalcStatusPolling();
}

function estimateLineFromGSNRecord(record, node, lineId = `line_${Date.now()}`) {
  const isWork = record.isWork !== false;
  const children = isWork
    ? (record.resources || []).map((resource, index) => estimateChildFromResource(resource, index, lineId))
    : undefined;
  return {
    id: lineId,
    type: "position",
    code: record.code || "",
    recordCode: record.code || "",
    originalCode: record.originalCode || "",
    name: record.name || node.name || "",
    unit: record.unit || node.unit || "",
    quantity: 1,
    unitPrice: 0,
    unitPriceText: isWork ? "" : record.unitPriceText || "",
    unitPriceIndex: isWork ? "" : record.unitPriceIndex || "",
    total: 0,
    isWork,
    hasResources: isWork && Boolean(record.hasResources || children?.length),
    children: children?.length ? children : undefined,
  };
}

async function fetchGSNHierarchyRecords(hierarchyCode, fgisSetId = "", district = "") {
  const params = new URLSearchParams({
    supplement: state.gsnSupplement,
    hierarchy: hierarchyCode,
  });
  if (fgisSetId) {
    params.set("fgisSet", fgisSetId);
  }
  if (district) {
    params.set("district", district);
  }
  const result = await api(`/api/gsn/hierarchy-records?${params.toString()}`);
  return result.records || [];
}

async function fetchGSNRecordDetail(code, fgisSetId = "", district = "") {
  const params = new URLSearchParams({ code });
  if (fgisSetId) {
    params.set("fgisSet", fgisSetId);
  }
  if (district) {
    params.set("district", district);
  }
  const result = await api(`/api/gsn/record?${params.toString()}`);
  return result.record || null;
}

function renderEstimateLineActions(estimateId, item, index, total, options = {}) {
  const encodedEstimateId = escapeHTML(estimateId);
  const encodedLineId = escapeHTML(item.id);

  if (options.rowKind === "child") {
    return `<td class="editor-estimate-actions-cell"></td>`;
  }

  return `
    <td class="editor-estimate-actions-cell">
      <div class="editor-estimate-line-actions">
        <button
          class="icon-button secondary"
          data-editor-line-edit="${encodedEstimateId}"
          data-editor-line-id="${encodedLineId}"
          type="button"
          title="Редактировать"
          aria-label="Редактировать"
        >${iconPencil()}</button>
        <button
          class="icon-button secondary icon-button-danger"
          data-editor-line-delete="${encodedEstimateId}"
          data-editor-line-id="${encodedLineId}"
          type="button"
          title="Удалить"
          aria-label="Удалить"
        >${iconTrash()}</button>
        <button
          class="icon-button secondary"
          data-editor-line-up="${encodedEstimateId}"
          data-editor-line-id="${encodedLineId}"
          type="button"
          title="Переместить вверх"
          aria-label="Переместить вверх"
          ${index === 0 ? "disabled" : ""}
        >${iconArrowUp()}</button>
        <button
          class="icon-button secondary"
          data-editor-line-down="${encodedEstimateId}"
          data-editor-line-id="${encodedLineId}"
          type="button"
          title="Переместить вниз"
          aria-label="Переместить вниз"
          ${index >= total - 1 ? "disabled" : ""}
        >${iconArrowDown()}</button>
      </div>
    </td>
  `;
}

function renderEstimateCodeCell(estimateId, item, options = {}) {
  const structuralLabel = estimateLineStructuralLabel(item);
  const displayCode = estimateLineDisplayCode(item);
  const encodedEstimateId = escapeHTML(estimateId);
  const encodedLineId = escapeHTML(item.id);
  const expandButton =
    options.rowKind === "main" && options.hasResources
      ? `<button
          class="editor-line-expand-btn"
          data-editor-line-expand="${encodedEstimateId}"
          data-editor-line-id="${encodedLineId}"
          type="button"
          title="${options.isExpanded ? "Свернуть ресурсы" : "Развернуть ресурсы"}"
          aria-label="${options.isExpanded ? "Свернуть ресурсы" : "Развернуть ресурсы"}"
        >${options.isExpanded ? "−" : "+"}</button>`
      : `<span class="editor-line-expand-placeholder"></span>`;

  const codeContent = structuralLabel
    ? `<span class="editor-estimate-structural-label">${escapeHTML(structuralLabel)}</span>`
    : `<span class="user-positions-code-text">${formatBreakableCode(displayCode)}</span>`;

  return `
    <td class="editor-estimate-code-cell">
      <div class="editor-estimate-code-cell-inner">
        ${expandButton}
        ${codeContent}
      </div>
    </td>
  `;
}

function estimateTableRowClass(item, rowKind) {
  const classes = [rowKind === "child" ? "editor-estimate-row-child" : "editor-estimate-row-main"];
  if (item?.type === "section") {
    classes.push("editor-estimate-row-section");
  } else if (item?.type === "subsection") {
    classes.push("editor-estimate-row-subsection");
  }
  return classes.join(" ");
}

function renderEstimateCalcStatusBadge(item) {
  const status = String(item?.calcStatus || "").trim();
  if (!status || estimateLineIsStructural(item)) {
    return "";
  }
  const labels = {
    queued: "В очереди",
    leased: "Считается",
    done: "Рассчитано",
    failed: "Повтор",
    dead: "Ошибка",
  };
  const label = labels[status] || status;
  const title = item?.calcError ? ` title="${escapeHTML(item.calcError)}"` : "";
  return `<span class="editor-estimate-calc-status editor-estimate-calc-status-${escapeHTML(status)}"${title}>${escapeHTML(label)}</span>`;
}

function renderEstimateTableRow(options) {
  const { estimateId, item, indexLabel, rowKind, hasResources, isExpanded, lineIndex, lineCount, parentItem } =
    options;
  const isChild = rowKind === "child";
  const isStructural = estimateLineIsStructural(item);
  const isPartialGsn = estimateLineShowsGsnPartial(item);
  const lineTotal = estimateLineTotal(item, parentItem);
  const quantityCell = isStructural ? "" : formatNumber(item.quantity);
  return `
    <tr class="${estimateTableRowClass(item, rowKind)}">
      <td class="editor-estimate-index-cell">${escapeHTML(indexLabel)}</td>
      ${renderEstimateCodeCell(estimateId, item, { rowKind, hasResources, isExpanded })}
      <td class="editor-estimate-name-cell${isChild ? " editor-estimate-name-cell-child" : ""}">${isPartialGsn ? renderEstimateCalcStatusBadge(item) : `${escapeHTML(item.name || "")}${renderEstimateCalcStatusBadge(item)}`}</td>
      <td class="editor-estimate-unit-cell">${isPartialGsn ? "" : escapeHTML(item.unit || "")}</td>
      <td class="editor-estimate-quantity-cell">${quantityCell}</td>
      <td class="editor-estimate-price-cell">${isPartialGsn ? "" : formatEstimateUnitPrice(item)}</td>
      <td class="editor-estimate-total-cell">${isPartialGsn ? "" : formatEstimateMoney(lineTotal)}</td>
      ${renderEstimateLineActions(estimateId, item, lineIndex, lineCount, {
        rowKind,
        hasResources,
        isExpanded,
      })}
    </tr>
  `;
}

function buildEstimateTableRows(estimate, { start = 0, end, positionNumberStart = 0 } = {}) {
  const items = estimate.items || [];
  const endIndex = end ?? items.length;
  const expanded = getExpandedLineIds(estimate.id);
  const rows = [];
  let positionNumber = positionNumberStart;

  for (let index = start; index < endIndex; index += 1) {
    const item = items[index];
    const isStructural = estimateLineIsStructural(item);
    if (!isStructural) {
      positionNumber += 1;
    }
    const indexLabel = isStructural ? "" : String(positionNumber);
    const hasResources = estimateLineCanExpand(item);
    const isExpanded = expanded.has(item.id);
    rows.push(
      renderEstimateTableRow({
        estimateId: estimate.id,
        item,
        indexLabel,
        rowKind: "main",
        hasResources,
        isExpanded,
        lineIndex: index,
        lineCount: items.length,
      }),
    );
    if (hasResources && isExpanded) {
      (item.children || []).forEach((child, childIndex) => {
        rows.push(
          renderEstimateTableRow({
            estimateId: estimate.id,
            item: child,
            indexLabel: `${positionNumber}.${childIndex + 1}`,
            rowKind: "child",
            lineIndex: index,
            lineCount: items.length,
            parentItem: item,
          }),
        );
      });
    }
  }

  return rows;
}

function countEstimateTablePositionNumberBefore(items, index) {
  let positionNumber = 0;
  for (let i = 0; i < index; i += 1) {
    if (!estimateLineIsStructural(items[i])) {
      positionNumber += 1;
    }
  }
  return positionNumber;
}

async function toggleEstimateLineExpand(estimateId, lineId) {
  const estimate = state.openEstimates.find((item) => item.id === estimateId);
  const item = estimate?.items?.find((line) => line.id === lineId);
  if (!estimate || !item) {
    return;
  }

  const expanded = getExpandedLineIds(estimateId);
  if (expanded.has(lineId)) {
    expanded.delete(lineId);
    state.estimateLinesExpanded[estimateId] = [...expanded];
    saveEstimateLinesExpanded();
    renderEditor();
    return;
  }

  if (!item.children?.length) {
    const fetchCode = estimateRecordLookupCode(item);
    if (fetchCode) {
      try {
        const record = await fetchGSNRecordDetail(fetchCode, estimate.fgisSetId || "", estimate.district || "");
        if (!record) {
          showMessage("Запись не найдена", "error");
          return;
        }
        applyRecordDetailToEstimateItem(item, record);
        if (record.isWork === false) {
          saveEditorState();
          renderEditor();
          return;
        }
        if (record.resources?.length) {
          refreshEstimateItemChildrenPricing(item, record.resources);
          item.hasResources = true;
          saveEditorState();
          updateEditorTableTotals(estimate);
        } else {
          item.hasResources = false;
          showMessage("У позиции нет ресурсов", "ok");
          renderEditor();
          return;
        }
      } catch (error) {
        showMessage(error.message || "Не удалось загрузить ресурсы", "error");
        return;
      }
    }
  }

  if (!item.children?.length) {
    showMessage("У позиции нет ресурсов", "ok");
    return;
  }

  expanded.add(lineId);
  state.estimateLinesExpanded[estimateId] = [...expanded];
  saveEstimateLinesExpanded();
  renderEditor();
}

function syncEstimateViewModeTabState() {
  if (!els.editorEstimateViewModes) {
    return;
  }
  els.editorEstimateViewModes.querySelectorAll("[data-editor-view-mode]").forEach((button) => {
    button.classList.toggle("active", state.estimateViewMode === button.dataset.editorViewMode);
  });
}

function renderEditorEstimateViewModes(estimate) {
  if (!estimate || !els.editorEstimateViewModes) {
    els.editorEstimateViewModes?.classList.add("hidden");
    if (els.editorEstimateViewModes) {
      els.editorEstimateViewModes.innerHTML = "";
    }
    return;
  }

  els.editorEstimateViewModes.classList.remove("hidden");
  const existing = els.editorEstimateViewModes.querySelector("[data-editor-view-estimate]");
  if (existing && existing.dataset.editorViewEstimate === estimate.id) {
    syncEstimateViewModeTabState();
    return;
  }
  els.editorEstimateViewModes.innerHTML = renderEstimateViewModeTabs(estimate);
}

function cancelEditorTableBodyRender() {
  editorTableBodyRenderToken += 1;
}

function mountEditorTableBody(estimate, tbody, { incremental = false } = {}) {
  const items = estimate.items || [];
  const emptyRow = `<tr><td colspan="8" class="muted">Строки сметы отсутствуют</td></tr>`;
  if (!items.length) {
    tbody.innerHTML = emptyRow;
    markEditorTableBodyReady();
    return;
  }

  if (!incremental || items.length <= EDITOR_TABLE_IMMEDIATE_ROWS) {
    tbody.innerHTML = buildEstimateTableRows(estimate).join("");
    markEditorTableBodyReady();
    return;
  }

  const token = ++editorTableBodyRenderToken;
  const firstEnd = Math.min(EDITOR_TABLE_IMMEDIATE_ROWS, items.length);
  tbody.innerHTML = buildEstimateTableRows(estimate, {
    start: 0,
    end: firstEnd,
    positionNumberStart: 0,
  }).join("");
  markEditorTableBodyReady();
  let index = firstEnd;

  const appendChunk = () => {
    if (editorTableBodyRenderToken !== token) {
      return;
    }
    if (state.editorTab !== estimate.id || state.estimateViewMode !== "table") {
      return;
    }
    const liveTbody = els.editorContent?.querySelector(".editor-estimate-table tbody");
    if (!liveTbody || liveTbody !== tbody) {
      return;
    }
    const end = Math.min(index + EDITOR_TABLE_CHUNK_ROWS, items.length);
    const template = document.createElement("template");
    template.innerHTML = buildEstimateTableRows(estimate, {
      start: index,
      end,
      positionNumberStart: countEstimateTablePositionNumberBefore(items, index),
    }).join("");
    liveTbody.append(...template.content.children);
    index = end;
    if (index < items.length) {
      requestAnimationFrame(appendChunk);
    }
  };

  if (index < items.length) {
    requestAnimationFrame(appendChunk);
  }
}

function renderEstimateViewModeTabs(estimate) {
  const textActive = state.estimateViewMode === "text" ? " active" : "";
  const tableActive = state.estimateViewMode === "table" ? " active" : "";
  return `
    <button
      class="editor-view-mode-tab${textActive}"
      data-editor-view-mode="text"
      data-editor-view-estimate="${escapeHTML(estimate.id)}"
      type="button"
    >Текстовый</button>
    <button
      class="editor-view-mode-tab${tableActive}"
      data-editor-view-mode="table"
      data-editor-view-estimate="${escapeHTML(estimate.id)}"
      type="button"
    >Табличный</button>
  `;
}

function renderEstimateEditorHeader(estimate) {
  const items = estimate.items || [];
  if (state.estimateViewMode === "table") {
    ensureEstimateCalcProgressTarget(estimate.id, estimate);
  }
  const progressView = getEstimateCalcProgressDisplay(estimate.id, items);
  const calcProgress = renderEstimateCalcProgressHTML(items, estimate.id, progressView);
  const headerGrandTotal = progressView?.grandTotal ?? estimateGrandTotalForDisplay(items);
  return `
    <div class="editor-estimate-header">
      <label class="editor-estimate-header-field">
        <span class="muted">Шифр</span>
        <input
          data-editor-estimate-code="${escapeHTML(estimate.id)}"
          value="${escapeHTML(estimate.code || "")}"
        />
      </label>
      <label class="editor-estimate-header-field editor-estimate-header-field-grow">
        <span class="muted">Наименование</span>
        <input
          data-editor-estimate-title="${escapeHTML(estimate.id)}"
          value="${escapeHTML(estimate.title || "")}"
        />
      </label>
      <label class="editor-estimate-header-field">
        <span class="muted">Сметный район</span>
        <button
          class="editor-district-field"
          data-editor-district-open="${escapeHTML(estimate.id)}"
          type="button"
          title="${escapeHTML(districtRegionHint(estimate.district))}"
        >${escapeHTML(estimate.district || "—")}</button>
      </label>
      <label class="editor-estimate-header-field editor-estimate-header-field-fgis-set">
        <span class="muted">Сметные цены и индексы</span>
        <select
          class="editor-fgis-set-select"
          data-editor-fgis-set="${escapeHTML(estimate.id)}"
        >${renderFgisSetSelectOptions(estimate.fgisSetId)}</select>
      </label>
      <div class="editor-estimate-header-field editor-estimate-header-field-total">
        <div class="editor-estimate-total-row">
          <span class="muted">Сметная стоимость</span>
          <output class="editor-estimate-total-value">${money.format(headerGrandTotal)}</output>
        </div>
        ${calcProgress}
      </div>
    </div>
    <div class="editor-estimate-meta">
      <span class="editor-meta-inline">
        <span class="muted">Стройка:</span>
        <strong>${escapeHTML(estimate.constructionLabel || "-")}</strong>
      </span>
      <span class="editor-meta-inline">
        <span class="muted">Объект:</span>
        <strong>${escapeHTML(estimate.objectLabel || "-")}</strong>
      </span>
    </div>
  `;
}

function renderEstimateEditorActions(estimate, options = {}) {
  const { tableMode = false } = options;
  return `
    <div class="editor-estimate-actions">
      ${
        tableMode
          ? `
      <button
        class="secondary construction-add-btn"
        data-editor-add-line="${escapeHTML(estimate.id)}"
        type="button"
      >+ Добавить строку сметы</button>
      <button
        class="secondary construction-add-btn"
        data-editor-paste-buffer="${escapeHTML(estimate.id)}"
        type="button"
        ${state.buffer.length ? "" : "disabled"}
      >Добавить позиции из буфера</button>
      <button
        class="secondary construction-add-btn"
        data-editor-export-excel="${escapeHTML(estimate.id)}"
        type="button"
      >Вывести в Excel</button>
      `
          : ""
      }
      <button
        class="secondary construction-add-btn"
        data-editor-close-estimate="${escapeHTML(estimate.id)}"
        type="button"
      >Закрыть смету</button>
      <button
        class="icon-button secondary icon-button-danger"
        data-editor-delete-estimate="${escapeHTML(estimate.id)}"
        type="button"
        title="Удалить смету"
        aria-label="Удалить смету"
      >${iconTrash()}</button>
    </div>
  `;
}

function renderEstimateTextEditor(estimate) {
  return `
    <textarea
      class="editor-estimate-text"
      data-editor-estimate-text="${escapeHTML(estimate.id)}"
      spellcheck="false"
      rows="24"
    ></textarea>
    ${renderEstimateEditorActions(estimate)}
  `;
}

function isEditorHeaderFieldActive() {
  const active = document.activeElement;
  if (!active || !els.editorContent?.contains(active)) {
    return false;
  }
  const header = active.closest(".editor-estimate-header");
  if (!header || header.offsetParent === null) {
    return false;
  }
  return true;
}

function isEditorContentMountedForEstimate(estimateId) {
  if (!estimateId || !els.editorContent) {
    return false;
  }
  return Boolean(
    els.editorContent.querySelector(
      `[data-editor-fgis-set="${estimateId}"], [data-editor-estimate-text="${estimateId}"]`,
    ),
  );
}

function refreshEditorFgisSetSelect(estimate) {
  const select = els.editorContent?.querySelector(`[data-editor-fgis-set="${estimate.id}"]`);
  if (!select) {
    return false;
  }
  const selectedId = String(estimate.fgisSetId || "");
  select.innerHTML = renderFgisSetSelectOptions(selectedId);
  select.value = selectedId;
  return true;
}

function updateEditorTableContent(estimate, options = {}) {
  if (state.estimateViewMode !== "table") {
    return;
  }
  const { refreshBody = false } = options;
  if (refreshBody) {
    const tbody = els.editorContent?.querySelector(".editor-estimate-table tbody");
    if (!tbody) {
      return;
    }
    cancelEditorTableBodyRender();
    editorTableBodyReady = false;
    syncEditorTablePassiveState();
    const itemCount = (estimate.items || []).length;
    mountEditorTableBody(estimate, tbody, {
      incremental: itemCount > EDITOR_TABLE_IMMEDIATE_ROWS,
    });
  }
  updateEditorTableTotals(estimate);
}

function flushPendingEditorRemount() {
  if (!pendingEditorRemountEstimateId || isEditorHeaderFieldActive()) {
    return;
  }
  const estimateId = pendingEditorRemountEstimateId;
  pendingEditorRemountEstimateId = null;
  if (state.section !== "editor" || state.editorTab !== estimateId) {
    return;
  }
  const estimate = state.openEstimates.find((item) => item.id === estimateId);
  if (!estimate) {
    return;
  }
  mountEditorContent(estimate, { force: true });
}

function mountEditorContent(estimate, { force = false } = {}) {
  const editorMounted = isEditorContentMountedForEstimate(estimate.id);
  if (!force && editorMounted && isEditorHeaderFieldActive()) {
    pendingEditorRemountEstimateId = estimate.id;
    return;
  }
  pendingEditorRemountEstimateId = null;
  cancelEditorTableBodyRender();
  if (state.estimateViewMode === "table") {
    resetEditorTableInteractionState();
  }
  els.editorContent.innerHTML = renderEstimateEditor(estimate);
  els.editorContent.classList.toggle("editor-content--table", state.estimateViewMode === "table");
  if (state.estimateViewMode === "text") {
    const textarea = els.editorContent.querySelector(`[data-editor-estimate-text="${estimate.id}"]`);
    if (textarea) {
      const draft = state.estimateTextDrafts[estimate.id];
      if (draft === undefined) {
        textarea.value = "";
        textarea.placeholder = "Загрузка текста сметы…";
        textarea.disabled = true;
        scheduleEstimateTextDraftBuild(estimate);
      } else {
        textarea.disabled = false;
        textarea.placeholder = "";
        textarea.value = draft;
      }
    }
    return;
  }
  const tbody = els.editorContent.querySelector(".editor-estimate-table tbody");
  if (tbody) {
    const itemCount = (estimate.items || []).length;
    mountEditorTableBody(estimate, tbody, {
      incremental: itemCount > EDITOR_TABLE_IMMEDIATE_ROWS,
    });
  }
}

function renderEstimateTableEditor(estimate) {
  return `
    <div class="table-wrap editor-estimate-table-body-scroll">
      <table class="editor-estimate-table">
        <thead>
          <tr>
            <th class="editor-estimate-index-cell">№ п/п</th>
            <th class="editor-estimate-code-cell">Шифр</th>
            <th class="editor-estimate-name-cell">Наименование</th>
            <th class="editor-estimate-unit-cell">Ед. изм.</th>
            <th class="editor-estimate-quantity-cell">Объем<br><span class="editor-estimate-header-sub">/ расход</span></th>
            <th class="editor-estimate-price-cell">Стоимость ед.</th>
            <th class="editor-estimate-total-cell">Стоимость на объем</th>
            <th class="editor-estimate-actions-cell">Действия</th>
          </tr>
        </thead>
        <tbody></tbody>
      </table>
    </div>
    ${renderEstimateEditorActions(estimate, { tableMode: true })}
  `;
}

function renderEstimateEditor(estimate) {
  if (state.estimateViewMode === "text") {
    return renderEstimateTextEditor(estimate);
  }

  return `
    <div class="editor-estimate-table-layout">
      <div class="editor-estimate-chrome">
        ${renderEstimateEditorHeader(estimate)}
      </div>
      ${renderEstimateTableEditor(estimate)}
    </div>
  `;
}

function fgisSetDisplayName(setId) {
  const selected = String(setId || "").trim();
  if (!selected) {
    return "Не выбраны";
  }
  const set = state.fgisSets.find((item) => item.id === selected);
  return set?.name || selected;
}

function formatEstimateUnitPricePlain(item) {
  const normalized = normalizeResourcePricing(item);
  const base = String(normalized?.unitPriceText || "").trim();
  const index = String(normalized?.unitPriceIndex || "").trim();
  if (index) {
    return base ? `${base}\n*${index}` : `*${index}`;
  }
  if (base) {
    return base;
  }
  const value = Number(normalized?.unitPrice || 0);
  return value ? estimateCostDetailed.format(value) : "";
}

function estimateLineCodeExportText(item) {
  const structuralLabel = estimateLineStructuralLabel(item);
  return structuralLabel || estimateLineDisplayCode(item);
}

const ESTIMATE_TEXT_DELIMITER = "'";

function escapeEstimateTextField(value) {
  return String(value ?? "")
    .replace(/\\/g, "\\\\")
    .replace(/\r\n/g, "\\n")
    .replace(/\n/g, "\\n")
    .replace(/\r/g, "\\n")
    .replace(/'/g, "''");
}

function unescapeEstimateTextField(value) {
  return String(value ?? "")
    .replace(/\\n/g, "\n")
    .replace(/\\\\/g, "\\");
}

function estimateLineIsGsn(item) {
  const source = String(item?.source || "").toLowerCase();
  if (source === "gsn") {
    return true;
  }
  if (source === "user_position" || estimateLineIsStructural(item) || item?.type !== "position") {
    return false;
  }
  return Boolean(String(item?.rawText || "").trim()) && Boolean(estimateRecordFetchCode(item));
}

function estimateLineCalcDone(item) {
  return String(item?.calcStatus || "").trim() === "done";
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
  if (String(item?.name || "").trim() || String(item?.unit || "").trim()) {
    return false;
  }
  if (Number(item?.total || 0) || Number(item?.unitPrice || 0)) {
    return false;
  }
  return !item.children?.length;
}

function clearGsnLineCalcEnrichment(item) {
  if (!estimateLineIsGsn(item) || estimateLineIsStructural(item)) {
    return;
  }
  const sourceFields = parseSourceDataPositionLineFieldsFromRawText(item.rawText);
  item.calcStatus = "";
  item.calcError = "";
  item.calcJson = null;
  item.calcJsonKey = JSON.stringify(null);
  delete item.calcRecordAppliedKey;
  item.hasResources = false;
  item.children = undefined;
  delete item.isWork;
  item.originalCode = "";
  if (sourceFields?.hasName) {
    item.name = sourceFields.name;
  } else {
    item.name = "";
  }
  if (sourceFields?.hasUnit) {
    item.unit = sourceFields.unit;
  } else {
    item.unit = "";
  }
  if (sourceFields?.hasTotal) {
    item.total = sourceFields.total;
    item.unitPrice = sourceFields.unitPrice;
  } else {
    item.unitPrice = 0;
    item.unitPriceText = "";
    item.unitPriceIndex = "";
    item.total = 0;
  }
}

function enrichEditorEstimateItems(items) {
  (items || []).forEach((item) => {
    if (estimateCalcRecordNeedsApply(item)) {
      applyEstimateCalcRecord(item, item.calcJson);
    }
  });
}

function estimateLineShouldSerializeAsGsn(item) {
  if (estimateLineIsStructural(item)) {
    return false;
  }
  if (String(item?.source || "").toLowerCase() === "user_position") {
    return false;
  }
  if (estimateLineIsGsn(item)) {
    return true;
  }
  if (item?.type !== "position") {
    return false;
  }
  return estimateLineIsWorkPosition(item) || estimateLineIsResourcePosition(item);
}

function normalizeEstimateItemTextFields(fields) {
  if (fields.length === 10) {
    return fields.slice(1);
  }
  if (fields.length === 9) {
    return fields;
  }
  if (fields.length === 8) {
    return [...fields, "0"];
  }
  if (fields.length === 7) {
    return [...fields, "0", "0"];
  }
  if (fields.length === 6) {
    const [type, source, code, name, quantity, unit] = fields;
    return [type, source, code, "", name, quantity, unit, "0", "0"];
  }
  return null;
}

function splitEstimateTextLine(line) {
  return String(line || "").split(ESTIMATE_TEXT_DELIMITER);
}

function parseEstimateTextNumber(value) {
  const normalized = String(value ?? "")
    .trim()
    .replace(/\s/g, "")
    .replace(",", ".");
  if (!normalized) {
    return 0;
  }
  const number = Number(normalized);
  return Number.isFinite(number) ? number : 0;
}

const SOURCE_DATA_HEADER_SEP_4 = "''''";
const SOURCE_DATA_HEADER_SEP_5 = "'''''";
const SOURCE_DATA_FORMAT_CODE = 49;

function deriveEstimateSourceDataNumericId(estimate) {
  if (estimate.sourceDataNumericId != null && Number.isFinite(Number(estimate.sourceDataNumericId))) {
    return Number(estimate.sourceDataNumericId);
  }
  const sessionMatch = String(estimate.id || "").match(/^session_est_(\d+)$/);
  if (sessionMatch) {
    return Math.max(1, Math.floor(Number(sessionMatch[1]) / 1000));
  }
  const digits = String(estimate.id || "").replace(/\D/g, "");
  if (digits) {
    return Math.max(1, Number(digits.slice(-9)) || 1);
  }
  return 1;
}

function getEstimateConstructionObjectFields(estimate) {
  if (
    estimate.constructionName != null ||
    estimate.constructionCode != null ||
    estimate.objectName != null ||
    estimate.objectCode != null
  ) {
    return {
      constructionName: estimate.constructionName || "",
      constructionCode: estimate.constructionCode || "",
      objectName: estimate.objectName || "",
      objectCode: estimate.objectCode || "",
    };
  }

  const object = state.objects.find((item) => item.id === estimate.objectId);
  const construction = object
    ? state.constructions.find((item) => item.id === object.constructionId)
    : null;

  return {
    constructionName: construction?.name || "",
    constructionCode: construction?.code || "",
    objectName: object?.name || "",
    objectCode: object?.code || "",
  };
}

function estimateSourceDataPositionCode(item) {
  return String(item?.code || item?.recordCode || "").trim();
}

function readEstimateHeaderFieldsFromDom(estimateId) {
  const estimate = state.openEstimates.find((item) => item.id === estimateId);
  if (!estimate) {
    return;
  }
  const codeInput = els.editorContent?.querySelector(`[data-editor-estimate-code="${estimateId}"]`);
  const titleInput = els.editorContent?.querySelector(`[data-editor-estimate-title="${estimateId}"]`);
  if (codeInput) {
    estimate.code = String(codeInput.value || "").trim();
  }
  if (titleInput) {
    estimate.title = String(titleInput.value || "").trim();
  }
}

function flushEstimateEditorFields(estimateId) {
  if (!estimateId || estimateId === "buffer") {
    return;
  }
  const estimate = state.openEstimates.find((item) => item.id === estimateId);
  if (!estimate) {
    return;
  }
  if (state.estimateViewMode === "text" && state.editorTab === estimateId) {
    const draft = readEstimateTextDraft(estimateId);
    if (draft != null) {
      try {
        applyEstimateTextToEstimate(estimate, draft);
      } catch {
        // Не блокируем переключение вкладки при ошибке разбора черновика.
      }
    }
    return;
  }
  if (state.estimateViewMode === "table" && state.editorTab === estimateId) {
    readEstimateHeaderFieldsFromDom(estimateId);
  }
}

function normalizeEstimateSourceDataFields(estimate) {
  if (!estimate) {
    return;
  }
  if (estimate.sourceDataDocumentSet == null && estimate.sourceDataHeaderCode1 != null) {
    estimate.sourceDataDocumentSet = estimate.sourceDataHeaderCode1;
  }
  if (estimate.sourceDataCalculationFlags == null && estimate.sourceDataHeaderCode2 != null) {
    estimate.sourceDataCalculationFlags = estimate.sourceDataHeaderCode2;
  }
  if (estimate.sourceDataSummaryChapterNo == null && estimate.sourceDataOrdinal != null) {
    estimate.sourceDataSummaryChapterNo = estimate.sourceDataOrdinal;
  }
}

function serializeSourceDataEstimateHeaderLine(estimate) {
  normalizeEstimateSourceDataFields(estimate);
  const numericId = deriveEstimateSourceDataNumericId(estimate);
  const fields = [
    String(numericId * 10),
    estimate.sourceDataDocumentSet || "",
    estimate.sourceDataCalculationFlags || "",
    "",
    estimate.district || "",
    "",
    "",
    "",
    "",
  ];
  return `Э${fields.join(ESTIMATE_TEXT_DELIMITER)}*`;
}

function serializeSourceDataConstructionLine(estimate) {
  const { constructionName, constructionCode, objectName, objectCode } =
    getEstimateConstructionObjectFields(estimate);
  const fields = [
    "",
    "",
    constructionName || "",
    constructionCode || "",
    objectCode || "",
    objectName || "",
    "",
    estimate.sourceDataSummaryChapterNo || "",
    estimate.code || "",
    estimate.title || "",
    "",
    "",
    "",
  ];
  return `Ю${fields.join(ESTIMATE_TEXT_DELIMITER)}*`;
}

function serializeSourceDataConfigLine(estimate) {
  const raw = String(estimate.sourceDataConfigLineRaw || "");
  if (raw.startsWith("F(")) {
    return `${upsertSourceDataF49ConfigLine(raw, estimate.fgisSetId || "")}*`;
  }
  const fgisSetId = estimate.fgisSetId || "";
  if (fgisSetId) {
    return `F(${SOURCE_DATA_FORMAT_CODE})${ESTIMATE_TEXT_DELIMITER}наборФГИС=${escapeEstimateTextField(fgisSetId)}*`;
  }
  return `F(${SOURCE_DATA_FORMAT_CODE})*`;
}

function serializeSourceDataClosingLine(estimate) {
  if (estimate.sourceDataBimConfig) {
    return `К${ESTIMATE_TEXT_DELIMITER}${escapeEstimateTextField(estimate.sourceDataBimConfig)}*`;
  }
  return "К*";
}

function estimateSourceDataPositionSerializedFields(item) {
  const parsed = parseSourceDataPositionLineFieldsFromRawText(item.rawText);
  const name = String(item.name || "").trim() || (parsed?.hasName ? parsed.name : "");
  const unit = String(item.unit || "").trim() || (parsed?.hasUnit ? parsed.unit : "");
  let total = Number(item.total || 0);
  if (!total && parsed?.hasTotal) {
    total = parsed.total;
  }
  return { name, unit, total };
}

function serializeSourceDataPositionLine(item) {
  const code = estimateSourceDataPositionCipherRaw(item);
  const quantity = estimateSourceDataQuantityRaw(item);
  const { name, unit, total } = estimateSourceDataPositionSerializedFields(item);
  const hasExtraFields = Boolean(name || unit || total);

  if (!hasExtraFields) {
    return `${escapeEstimateTextField(code)}${ESTIMATE_TEXT_DELIMITER}${escapeEstimateTextField(quantity)}*`;
  }

  const totalText = total ? String(total) : "";
  return `${escapeEstimateTextField(code)}${ESTIMATE_TEXT_DELIMITER}${escapeEstimateTextField(quantity)}${ESTIMATE_TEXT_DELIMITER}${escapeEstimateTextField(totalText)}${ESTIMATE_TEXT_DELIMITER}${escapeEstimateTextField(name)}${ESTIMATE_TEXT_DELIMITER}${escapeEstimateTextField(unit)}*`;
}

function serializeSourceDataItemLine(item) {
  if (item?.type === "section") {
    return `Р${escapeEstimateTextField(item.name || "")}*`;
  }
  if (item?.type === "subsection") {
    return `ПР${escapeEstimateTextField(item.name || "")}*`;
  }
  return serializeSourceDataPositionLine(item);
}

function serializedSourceDataLineBody(line) {
  return line.endsWith("*") ? line.slice(0, -1) : line;
}

function hydrateEstimateSourceDataFgisSet(estimate) {
  if (!estimate) {
    return "";
  }
  const fromRaw = parseSourceDataFgisSetFromLine(String(estimate.sourceDataConfigLineRaw || ""));
  const fgisSetId = String(estimate.fgisSetId || fromRaw || "").trim();
  if (fgisSetId) {
    estimate.fgisSetId = fgisSetId;
    applyFgisSetToEstimateSourceData(estimate, fgisSetId);
  }
  return fgisSetId;
}

function syncEstimateSourceDataFromFgisSet(estimate) {
  hydrateEstimateSourceDataFgisSet(estimate);
}

function scheduleEstimateTextDraftBuild(estimate) {
  const estimateId = estimate.id;
  if (state.estimateTextDrafts[estimateId] !== undefined) {
    return;
  }
  window.setTimeout(() => {
    if (state.estimateTextDrafts[estimateId] !== undefined) {
      return;
    }
    state.estimateTextDrafts[estimateId] = serializeEstimateToText(estimate);
    if (
      state.section !== "editor" ||
      !sameEstimateId(state.editorTab, estimateId) ||
      state.estimateViewMode !== "text"
    ) {
      return;
    }
    const textarea = els.editorContent?.querySelector(`[data-editor-estimate-text="${estimateId}"]`);
    if (!textarea) {
      return;
    }
    textarea.disabled = false;
    textarea.placeholder = "";
    textarea.value = state.estimateTextDrafts[estimateId];
  }, 0);
}

function writeEstimateTextDraftFromState(estimateId) {
  const estimate = state.openEstimates.find((item) => item.id === estimateId);
  if (!estimate) {
    return;
  }
  syncEstimateSourceDataFromFgisSet(estimate);
  state.estimateTextDrafts[estimateId] = serializeEstimateToText(estimate);
}

function serializeEstimateToText(estimate) {
  syncEstimateSourceDataFromFgisSet(estimate);
  const lines = [
    serializeSourceDataEstimateHeaderLine(estimate),
    serializeSourceDataConstructionLine(estimate),
    serializeSourceDataConfigLine(estimate),
  ];
  const items = estimate.items || [];
  for (const item of items) {
    const line = serializeSourceDataItemLine(item);
    lines.push(line);
    if (item && (item.type === "position" || item.type === "section" || item.type === "subsection")) {
      item.rawText = serializedSourceDataLineBody(line);
    }
  }
  lines.push(serializeSourceDataClosingLine(estimate));
  return lines.join("\n");
}

function parseSourceDataParamMap(fields) {
  const params = {};
  for (const field of fields) {
    if (!field) {
      continue;
    }
    const eqIndex = field.indexOf("=");
    if (eqIndex < 0) {
      continue;
    }
    const key = field.slice(0, eqIndex).trim();
    const value = unescapeEstimateTextField(field.slice(eqIndex + 1));
    if (key) {
      params[key] = value;
    }
  }
  return params;
}

function parseSourceDataEstimateHeaderLine(line) {
  if (!line.startsWith("Э")) {
    throw new Error("Первая строка должна начинаться с Э");
  }
  const body = line.slice(1);
  const legacyHeaderMatch = body.match(/^(\d+)''''(.*)'''''$/);
  if (legacyHeaderMatch) {
    const idTimes10 = legacyHeaderMatch[1].trim();
    const district = legacyHeaderMatch[2];
    const numericId = Math.floor(parseEstimateTextNumber(idTimes10) / 10);
    return {
      sourceDataNumericId: Number.isFinite(numericId) && numericId > 0 ? numericId : 1,
      sourceDataDocumentSet: "",
      sourceDataCalculationFlags: "",
      district,
    };
  }

  const fields = splitSourceDataStrictFields(body);
  const idTimes10 = fields[0] || "";
  const numericId = Math.floor(parseEstimateTextNumber(idTimes10) / 10);
  return {
    sourceDataNumericId: Number.isFinite(numericId) && numericId > 0 ? numericId : 1,
    sourceDataDocumentSet: fields[1] || "",
    sourceDataCalculationFlags: fields[2] || "",
    district: fields[4] || "",
  };
}

function splitSourceDataStrictFields(line) {
  return String(line || "").split(ESTIMATE_TEXT_DELIMITER);
}

function parseSourceDataConstructionLine(line) {
  if (!line.startsWith("Ю")) {
    throw new Error("Вторая строка должна начинаться с Ю");
  }
  const fields = splitSourceDataStrictFields(line.slice(1));
  return {
    constructionName: fields[2] || "",
    constructionCode: fields[3] || "",
    objectCode: fields[4] || "",
    objectName: fields[5] || "",
    sourceDataSummaryChapterNo: fields[7] || "",
    code: fields[8] || "",
    title: fields[9] || "",
  };
}

function sourceDataFgisSetParamKey(field) {
  if (!field) {
    return "";
  }
  const eqIndex = field.indexOf("=");
  if (eqIndex < 0) {
    return "";
  }
  return field.slice(0, eqIndex).trim();
}

function upsertSourceDataF49ConfigLine(configLineBody, fgisSetId) {
  if (!configLineBody.startsWith("F(")) {
    return configLineBody;
  }
  const match = configLineBody.match(/^F\((\d+)\)(.*)$/);
  if (!match) {
    return configLineBody;
  }
  const formatCode = match[1];
  const fields = splitEstimateTextLine(match[2] || "").map(unescapeEstimateTextField);
  const otherFields = fields.filter((field) => {
    const key = sourceDataFgisSetParamKey(field);
    return key && key !== "наборФГИС" && key !== "fgisSetId";
  });
  const nextFields = [];
  const fgisToUse = fgisSetId || parseSourceDataFgisSetFromLine(configLineBody);
  if (fgisToUse) {
    nextFields.push(`наборФГИС=${fgisToUse}`);
  }
  nextFields.push(...otherFields);
  if (!nextFields.length) {
    return `F(${formatCode})`;
  }
  return `F(${formatCode})${ESTIMATE_TEXT_DELIMITER}${nextFields.map(escapeEstimateTextField).join(ESTIMATE_TEXT_DELIMITER)}`;
}

function applyFgisSetToEstimateSourceData(estimate, fgisSetId) {
  if (!estimate) {
    return;
  }
  const nextFgisSetId = String(fgisSetId || "");
  const raw = String(estimate.sourceDataConfigLineRaw || "");
  if (raw.startsWith("F(")) {
    estimate.sourceDataConfigLineRaw = upsertSourceDataF49ConfigLine(raw, nextFgisSetId);
    return;
  }
  if (nextFgisSetId) {
    estimate.sourceDataConfigLineRaw = `F(${SOURCE_DATA_FORMAT_CODE})${ESTIMATE_TEXT_DELIMITER}наборФГИС=${nextFgisSetId}`;
    return;
  }
  if (!raw) {
    estimate.sourceDataConfigLineRaw = `F(${SOURCE_DATA_FORMAT_CODE})`;
  }
}

function applyParsedSourceDataFgisSet(estimate, parsedEstimate, fgisSetIdFromText = "") {
  if (!estimate || !parsedEstimate) {
    return "";
  }
  const fgisSetId = String(fgisSetIdFromText || parsedEstimate.fgisSetId || "").trim();
  if (parsedEstimate.sourceDataConfigLineRaw) {
    estimate.sourceDataConfigLineRaw = parsedEstimate.sourceDataConfigLineRaw;
  }
  estimate.fgisSetId = fgisSetId;
  applyFgisSetToEstimateSourceData(estimate, fgisSetId);
  return fgisSetId;
}

function extractSourceDataBimConfigFromLines(lines) {
  for (let lineIndex = lines.length - 1; lineIndex >= 0; lineIndex -= 1) {
    const trimmed = String(lines[lineIndex] || "").trimEnd();
    if (!trimmed.endsWith("*")) {
      continue;
    }
    const line = trimmed.slice(0, -1);
    if (line === "К") {
      return "";
    }
    if (line.startsWith("К")) {
      const fields = splitSourceDataStrictFields(line.slice(1));
      return fields[1] || fields[0] || "";
    }
  }
  return "";
}

function parseSourceDataFgisSetFromLine(line) {
  const normalizedLine = String(line || "").trim();
  if (!normalizedLine.startsWith("F(")) {
    return "";
  }
  const match = normalizedLine.match(/^F\(\d+\)(.*)$/);
  if (!match) {
    return "";
  }
  const fields = splitEstimateTextLine(match[2] || "").map(unescapeEstimateTextField);
  const params = parseSourceDataParamMap(fields);
  const mapValue = params.наборФГИС || params.fgisSetId;
  if (mapValue) {
    return String(mapValue).trim();
  }
  const body = String(match[1] || "");
  const fallbackMatch = body.match(/(?:^|')\s*(?:наборФГИС|fgisSetId)\s*=\s*([^'*]+)/iu);
  return fallbackMatch ? String(fallbackMatch[1] || "").trim() : "";
}

function extractFgisSetIdFromEstimateText(text) {
  const lines = String(text || "")
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter((line) => line.length > 0);
  let fgisSetId = "";
  for (const rawLine of lines) {
    if (!rawLine.endsWith("*")) {
      continue;
    }
    const parsed = parseSourceDataFgisSetFromLine(rawLine.slice(0, -1));
    if (parsed) {
      fgisSetId = parsed;
    }
  }
  return fgisSetId;
}

function parseSourceDataConfigLine(line) {
  if (!line.startsWith("F(")) {
    throw new Error("Третья строка должна начинаться с F(49)");
  }
  const match = line.match(/^F\((\d+)\)(.*)$/);
  if (!match) {
    throw new Error("Строка F: неверный формат");
  }
  return {
    fgisSetId: parseSourceDataFgisSetFromLine(line),
  };
}

function isSourceDataSkippedLine(line) {
  if (line === "К") {
    return true;
  }
  if (line.startsWith("К'")) {
    return true;
  }
  return line.startsWith("F(");
}

function isSourceDataTextFormat(lines) {
  const firstLine = String(lines[0] || "").trim();
  return firstLine.startsWith("Э");
}

function applyLegacyEstimateTextToEstimate(estimate, lines) {
  const headerFields = splitEstimateTextLine(lines[0]).map(unescapeEstimateTextField);
  if (headerFields.length < 2) {
    throw new Error("Первая строка должна содержать титульные данные сметы");
  }

  const [code, title, description = "", district = "", fgisSetId = ""] = headerFields;
  const previousDistrict = estimate.district || "";
  estimate.code = code;
  estimate.title = title;
  estimate.description = description;
  estimate.district = headerFields.length > 3 ? district : previousDistrict;
  estimate.fgisSetId = headerFields.length > 4 ? fgisSetId : "";

  const existingItems = estimate.items || [];
  const existingById = new Map(existingItems.map((item) => [item.id, item]));
  const newItems = [];
  const gsnCodeOccurrence = new Map();

  for (let lineIndex = 1; lineIndex < lines.length; lineIndex += 1) {
    const line = lines[lineIndex];
    const fields = splitEstimateTextLine(line).map(unescapeEstimateTextField);
    if (fields.length === 2) {
      const [first, second] = fields;
      const lineType = String(first || "").toLowerCase();
      if (lineType === "section" || lineType === "subsection") {
        const existing = [...existingById.values()].find(
          (item) => item.type === lineType && String(item.name || "") === second,
        );
        newItems.push({
          id: existing?.id || `line_${Date.now()}_${newItems.length}`,
          type: lineType,
          code: "",
          name: second,
          unit: "",
          quantity: 0,
          unitPrice: 0,
          total: 0,
          rawText: line,
        });
        continue;
      }

      const gsnCode = extractSourceDataPositionCipher(first);
      const quantityValue = 0;
      const codeOccurrence = gsnCodeOccurrence.get(gsnCode) || 0;
      const existing =
        findExistingGsnItemByCodeOccurrence(existingItems, gsnCode, codeOccurrence) ||
        existingItems.find((item) => {
          const itemCode = estimateLineNormalizedSourceCode(item) || estimateRecordFetchCode(item);
          return String(itemCode).trim() === gsnCode;
        }) ||
        null;
      gsnCodeOccurrence.set(gsnCode, codeOccurrence + 1);
      newItems.push({
        id: existing?.id || `line_${Date.now()}_${newItems.length}`,
        type: "position",
        source: "gsn",
        sourceId: gsnCode,
        sourceCode: String(first || "").trim() || gsnCode,
        code: gsnCode,
        recordCode: gsnCode,
        originalCode: extractSourceDataPositionOriginalCode(first),
        name: "",
        unit: "",
        quantity: quantityValue,
        unitPrice: 0,
        total: 0,
        hasResources: false,
        children: undefined,
        calcStatus: "",
        calcError: "",
        calcJson: null,
        rawText: line,
      });
      continue;
    }

    const normalizedFields = normalizeEstimateItemTextFields(fields);
    if (!normalizedFields) {
      throw new Error(`Строка ${lineIndex + 1}: неверный формат (${fields.length} полей)`);
    }

    const [type, source, itemCode, originalCode, name, quantity, unit, unitPrice, total] = normalizedFields;
    const normalizedSource = String(source || "").toLowerCase();
    const existing =
      [...existingById.values()].find(
        (item) =>
          String(item.source || "").toLowerCase() === normalizedSource &&
          String(item.code || "") === String(itemCode || "") &&
          String(item.name || "") === String(name || ""),
      ) || null;
    const quantityValue = parseEstimateTextNumber(quantity);
    const unitPriceValue = parseEstimateTextNumber(unitPrice);
    let totalValue = parseEstimateTextNumber(total);
    if (!totalValue && quantityValue && unitPriceValue) {
      totalValue = quantityValue * unitPriceValue;
    }

    const item = {
      id: existing?.id || `line_${Date.now()}_${newItems.length}`,
      type: type || "position",
      source: normalizedSource,
      code: itemCode || "",
      recordCode: itemCode || existing?.recordCode || "",
      originalCode: originalCode || "",
      name: name || "",
      unit: unit || "",
      quantity: quantityValue,
      unitPrice: unitPriceValue,
      total: totalValue,
      hasResources: existing?.hasResources || false,
      children: existing?.children,
      rawText: line,
    };
    if (normalizedSource === "user_position" && existing?.sourceId) {
      item.sourceId = existing.sourceId;
    }
    newItems.push(item);
  }

  estimate.items = newItems;
}

function trimSourceDataRecordLine(line, lineNumber) {
  const trimmed = String(line || "").trimEnd();
  if (!trimmed.endsWith("*")) {
    throw new Error(`Строка ${lineNumber}: должна заканчиваться на *`);
  }
  return trimmed.slice(0, -1);
}

function applySourceDataTextToEstimate(estimate, lines) {
  if (lines.length < 3) {
    throw new Error("Формат «Исходные данные»: ожидается минимум 3 строки (Э, Ю, F(49))");
  }

  const header = parseSourceDataEstimateHeaderLine(trimSourceDataRecordLine(lines[0], 1));
  const construction = parseSourceDataConstructionLine(trimSourceDataRecordLine(lines[1], 2));
  const configLineRaw = trimSourceDataRecordLine(lines[2], 3);
  parseSourceDataConfigLine(configLineRaw);

  const previousDistrict = estimate.district || "";
  estimate.sourceDataNumericId = header.sourceDataNumericId;
  estimate.sourceDataDocumentSet = header.sourceDataDocumentSet || "";
  estimate.sourceDataCalculationFlags = header.sourceDataCalculationFlags || "";
  estimate.district = header.district || previousDistrict;
  estimate.constructionName = construction.constructionName;
  estimate.constructionCode = construction.constructionCode;
  estimate.objectName = construction.objectName;
  estimate.objectCode = construction.objectCode;
  estimate.sourceDataSummaryChapterNo = construction.sourceDataSummaryChapterNo || "";
  estimate.code = construction.code;
  estimate.title = construction.title;
  estimate.constructionLabel =
    `${construction.constructionCode || ""} ${construction.constructionName || ""}`.trim();
  estimate.objectLabel = `${construction.objectCode || ""} ${construction.objectName || ""}`.trim();
  estimate.sourceDataConfigLineRaw = configLineRaw;
  estimate.sourceDataBimConfig = extractSourceDataBimConfigFromLines(lines);
  estimate.fgisSetId = parseSourceDataFgisSetFromLine(configLineRaw);
  const existingItems = estimate.items || [];
  const existingById = new Map(existingItems.map((item) => [item.id, item]));
  const newItems = [];
  const gsnCodeOccurrence = new Map();

  for (let lineIndex = 3; lineIndex < lines.length; lineIndex += 1) {
    const rawLine = lines[lineIndex].trimEnd();
    if (!rawLine) {
      continue;
    }
    const line = trimSourceDataRecordLine(rawLine, lineIndex + 1);
    if (isSourceDataSkippedLine(line)) {
      continue;
    }

    if (line.startsWith("ПР")) {
      const name = unescapeEstimateTextField(line.slice(2));
      const existing = [...existingById.values()].find(
        (item) => item.type === "subsection" && String(item.name || "") === name,
      );
      newItems.push({
        id: existing?.id || `line_${Date.now()}_${newItems.length}`,
        type: "subsection",
        code: "",
        name,
        unit: "",
        quantity: 0,
        unitPrice: 0,
        total: 0,
        rawText: line,
      });
      continue;
    }

    if (line.startsWith("Р")) {
      const name = unescapeEstimateTextField(line.slice(1));
      const existing = [...existingById.values()].find(
        (item) => item.type === "section" && String(item.name || "") === name,
      );
      newItems.push({
        id: existing?.id || `line_${Date.now()}_${newItems.length}`,
        type: "section",
        code: "",
        name,
        unit: "",
        quantity: 0,
        unitPrice: 0,
        total: 0,
        rawText: line,
      });
      continue;
    }

    const fields = splitEstimateTextLine(line).map(unescapeEstimateTextField);
    if (fields.length < 2) {
      throw new Error(`Строка ${lineIndex + 1}: позиция должна содержать минимум шифр и объём`);
    }

    const parsedFields = parseSourceDataPositionLineFields(fields);

    const normalizedCode = parsedFields.sourceCode;
    const codeOccurrence = gsnCodeOccurrence.get(normalizedCode) || 0;
    const existing =
      [...existingById.values()].find((item) => String(item.rawText || "") === line) ||
      findExistingGsnItemByCodeOccurrence(existingItems, normalizedCode, codeOccurrence);
    gsnCodeOccurrence.set(normalizedCode, codeOccurrence + 1);

    const normalizedSource = String(existing?.source || "").toLowerCase();
    const isUserPosition = normalizedSource === "user_position" && existing?.sourceId;
    if (isUserPosition) {
      let totalValue = parsedFields.hasTotal ? parsedFields.total : Number(existing?.total || 0);
      let unitPriceValue = parsedFields.hasTotal
        ? parsedFields.unitPrice
        : Number(existing?.unitPrice || 0);
      if (!parsedFields.hasTotal && totalValue && parsedFields.quantity && !unitPriceValue) {
        unitPriceValue = totalValue / parsedFields.quantity;
      }
      if (!totalValue && parsedFields.quantity && unitPriceValue) {
        totalValue = parsedFields.quantity * unitPriceValue;
      }
      newItems.push({
        id: existing?.id || `line_${Date.now()}_${newItems.length}`,
        type: "position",
        source: "user_position",
        sourceId: existing.sourceId,
        sourceCode: "",
        code: normalizedCode || existing?.code || "",
        recordCode: normalizedCode || existing?.recordCode || "",
        originalCode: parsedFields.hasOriginalCode ? parsedFields.originalCode : existing?.originalCode || "",
        name: parsedFields.hasName ? parsedFields.name : existing?.name || "",
        unit: parsedFields.hasUnit ? parsedFields.unit : existing?.unit || "",
        quantity: parsedFields.hasQuantity ? Number(existing?.quantity || 0) : Number(existing?.quantity || 0),
        unitPrice: unitPriceValue,
        total: totalValue,
        hasResources: existing?.hasResources || false,
        children: existing?.children,
        calcStatus: "",
        calcError: "",
        calcJson: null,
        rawText: line,
      });
      continue;
    }

    newItems.push(
      buildGsnPositionItemFromSourceDataLine({
        existing,
        fields,
        rawText: line,
        lineIndex,
      }),
    );
  }

  estimate.items = newItems;
}

function findExistingGsnItemByCode(items, code) {
  return findExistingGsnItemByCodeOccurrence(items, code, 0);
}

function findExistingGsnItemByCodeOccurrence(items, code, occurrenceIndex = 0) {
  const normalizedCode = String(code || "").trim();
  if (!normalizedCode) {
    return null;
  }
  let seen = 0;
  for (const item of items || []) {
    if (!estimateLineIsGsn(item)) {
      continue;
    }
    const itemCode = estimateLineNormalizedSourceCode(item) || estimateRecordFetchCode(item);
    if (String(itemCode).trim() !== normalizedCode) {
      continue;
    }
    if (seen === occurrenceIndex) {
      return item;
    }
    seen += 1;
  }
  return null;
}

function invalidateEstimateTextDraft(estimateId) {
  delete state.estimateTextDrafts[estimateId];
}

function resolveEstimateTextDraftForParse(estimateId, estimate) {
  const textarea = els.editorContent?.querySelector(`[data-editor-estimate-text="${estimateId}"]`);
  if (textarea) {
    return textarea.value;
  }
  if (state.estimateTextDrafts[estimateId] !== undefined) {
    return state.estimateTextDrafts[estimateId];
  }
  return serializeEstimateToText(estimate);
}

function readEstimateTextDraft(estimateId) {
  const textarea = els.editorContent?.querySelector(`[data-editor-estimate-text="${estimateId}"]`);
  if (textarea) {
    state.estimateTextDrafts[estimateId] = textarea.value;
  }
  return state.estimateTextDrafts[estimateId];
}

function applyEstimateTextToEstimate(estimate, text) {
  const lines = text
    .split(/\r?\n/)
    .map((line) => line.trimEnd())
    .filter((line) => line.length > 0);
  if (!lines.length) {
    throw new Error("Текст сметы пуст");
  }

  if (isSourceDataTextFormat(lines)) {
    applySourceDataTextToEstimate(estimate, lines);
    return;
  }

  applyLegacyEstimateTextToEstimate(estimate, lines);
}

function getEstimateTextDraft(estimate) {
  if (state.estimateTextDrafts[estimate.id] === undefined) {
    state.estimateTextDrafts[estimate.id] = serializeEstimateToText(estimate);
  }
  return state.estimateTextDrafts[estimate.id];
}

function markGsnLinesQueuedLocally(items) {
  (items || []).forEach((item) => {
    if (estimateLineNeedsGsnCalc(item)) {
      item.calcStatus = "queued";
      item.calcError = "";
    }
  });
}

function estimateNeedsTableCalculation(estimate) {
  const items = estimate?.items || [];
  const calcTotal = estimateCalcProgressTotal(items);
  if (!calcTotal) {
    return false;
  }
  const key = estimateCalcTargetKey(estimate.id);
  if (key && estimateCalcAwaitingServer.has(key)) {
    return false;
  }
  const hasActiveQueue = items.some((item) => {
    if (!estimateLineNeedsGsnCalc(item)) {
      return false;
    }
    const status = String(item.calcStatus || "").trim();
    return status === "queued" || status === "leased";
  });
  if (hasActiveQueue) {
    return false;
  }
  return items.some((item) => {
    if (!estimateLineNeedsGsnCalc(item)) {
      return false;
    }
    const status = String(item.calcStatus || "").trim();
    return !status || status === "failed" || status === "dead";
  });
}

async function startEstimateCalculation(estimateId) {
  if (!state.me) {
    throw new Error("Нужна авторизация для запуска расчёта");
  }
  if (!isPersistedEstimateId(estimateId)) {
    throw new Error("Смета ещё не сохранена на сервере");
  }
  await api(`/api/estimates/${estimateId}/calc`, { method: "POST" });
}

async function runEstimateTableCalculation(estimateId, options = {}) {
  const estimateIndex = state.openEstimates.findIndex((item) => sameEstimateId(item.id, estimateId));
  if (estimateIndex < 0) {
    return;
  }

  const estimate = state.openEstimates[estimateIndex];
  const calcTotal = estimateCalcProgressTotal(estimate.items);
  if (!calcTotal) {
    showMessage("В смете нет позиций ГСН для расчёта", "error");
    return;
  }

  const fgisSetId = options.fgisSetId ?? estimate.fgisSetId ?? "";
  if (!options.skipStatePrep) {
    prepareEstimateTableCalculationState(estimateId, estimate);
  } else {
    markEstimateCalcAwaitingServer(estimateId);
    estimateCalcPollBlockedByPersist.add(estimateId);
  }

  if (
    state.section === "editor" &&
    sameEstimateId(state.editorTab, estimateId) &&
    state.estimateViewMode === "table"
  ) {
    updateEditorCalcProgressDom(estimate);
  }

  try {
    const blocked = await syncLicenseSessions(getEditorTableLicenseSubsectionIdsForFgisSet(fgisSetId));
    if (blocked.length) {
      if (options.revertToTextOnLicenseBlock) {
        state.estimateViewMode = "text";
        if (options.parsedEstimate && options.fgisSetIdFromText !== undefined) {
          applyParsedSourceDataFgisSet(estimate, options.parsedEstimate, options.fgisSetIdFromText);
        }
        saveEditorState();
        writeEstimateTextDraftFromState(estimateId);
        renderEditor();
      }
      showMessage(formatLicenseBlockedMessage(blocked), "error");
      try {
        await persistOpenEstimate(estimateId, { skipTextDraftSync: true, requireSave: true });
      } catch {
        return;
      }
      return;
    }

    setSectionLicenseBlocked("editor", false);
    startLicenseSessionSync();
    await persistOpenEstimate(estimateId, { requireSave: true });
    await startEstimateCalculation(estimateId);
    estimateCalcPollBlockedByPersist.delete(estimateId);
    syncEstimateCalcStatusPolling();

    if (
      state.section === "editor" &&
      sameEstimateId(state.editorTab, estimateId) &&
      state.estimateViewMode === "table"
    ) {
      const currentEstimate = state.openEstimates.find((item) => sameEstimateId(item.id, estimateId));
      if (currentEstimate) {
        updateEditorTableTotals(currentEstimate);
        updateEditorCalcProgressDom(currentEstimate);
      }
    }
    showMessage(`Расчёт запущен: ${calcTotal} поз.`, "ok");
  } catch (error) {
    clearEstimateCalcAwaitingServer(estimateId);
    showMessage(error.message || "Не удалось запустить расчёт сметы", "error");
  } finally {
    estimateCalcPollBlockedByPersist.delete(estimateId);
    if (
      state.section === "editor" &&
      sameEstimateId(state.editorTab, estimateId) &&
      state.estimateViewMode === "table" &&
      !estimateCalcStatusPollTimer
    ) {
      syncEstimateCalcStatusPolling();
    }
  }
}

async function setEstimateViewMode(estimateId, mode) {
  if (mode !== "text" && mode !== "table") {
    return;
  }

  const estimate = state.openEstimates.find((item) => item.id === estimateId);
  if (!estimate) {
    return;
  }

  if (state.estimateViewMode === mode) {
    if (mode === "table" && estimateNeedsTableCalculation(estimate)) {
      void runEstimateTableCalculation(estimateId);
    }
    return;
  }

  if (state.estimateViewMode === "text" && mode === "table") {
    const previousDistrict = estimate.district || "";
    const draft = resolveEstimateTextDraftForParse(estimateId, estimate);
    const fgisSetIdFromText = extractFgisSetIdFromEstimateText(draft);
    const parsedEstimate = cloneOpenEstimate(estimate);
    try {
      applyEstimateTextToEstimate(parsedEstimate, draft);
      (parsedEstimate.items || []).forEach(clearGsnLineCalcEnrichment);
      enrichEditorEstimateItems(parsedEstimate.items);
      if (!parsedEstimate.district && previousDistrict) {
        parsedEstimate.district = previousDistrict;
      }
    } catch (error) {
      showMessage(error.message || "Не удалось разобрать текст сметы", "error");
      return;
    }

    const fgisSetId = applyParsedSourceDataFgisSet(parsedEstimate, parsedEstimate, fgisSetIdFromText);
    const estimateIndex = state.openEstimates.findIndex((item) => item.id === estimateId);
    if (estimateIndex >= 0) {
      state.openEstimates[estimateIndex] = parsedEstimate;
    }
    invalidateEstimateTextDraft(estimateId);

    resetEditorTableInteractionState();

    prepareEstimateTableCalculationState(estimateId, parsedEstimate);

    state.estimateViewMode = "table";
    renderEditor();
    const tableEstimate = state.openEstimates.find((item) => sameEstimateId(item.id, estimateId));
    if (tableEstimate) {
      updateEditorCalcProgressDom(tableEstimate);
      updateEditorTableTotals(tableEstimate, { skipCacheRefresh: true });
    }
    refreshEditorFgisSetSelect(parsedEstimate);
    saveEditorState();

    void runEstimateTableCalculation(estimateId, {
      fgisSetId,
      fgisSetIdFromText,
      parsedEstimate,
      revertToTextOnLicenseBlock: true,
      skipStatePrep: true,
    });
    return;
  }

  if (mode === "text") {
    readEstimateHeaderFieldsFromDom(estimateId);
    writeEstimateTextDraftFromState(estimateId);
    saveEditorState();
    stopEstimateCalcStatusPolling();
    stopLicenseSessionSync();
    void releaseLicenseSessions();
    setSectionLicenseBlocked("editor", false);
  }

  state.estimateViewMode = mode;
  renderEditor();
}

async function syncEstimateTextDraftIfNeeded(estimateId) {
  if (state.estimateViewMode !== "text") {
    return;
  }
  const estimate = state.openEstimates.find((item) => item.id === estimateId);
  if (!estimate) {
    return;
  }
  const draft = resolveEstimateTextDraftForParse(estimateId, estimate);
  applyEstimateTextToEstimate(estimate, draft);
  applyParsedSourceDataFgisSet(estimate, estimate, extractFgisSetIdFromEstimateText(draft));
  invalidateEstimateTextDraft(estimateId);
  saveEditorState();
}

const EXCEL_COLOR_BORDER = "DCE3EF";
const EXCEL_COLOR_MUTED = "8A96A8";
const EXCEL_COLOR_CHILD_BG = "FAFBFD";
const EXCEL_COLOR_TOTAL_BG = "F5F7FB";

function excelCellBorder() {
  const edge = { style: "thin", color: { rgb: EXCEL_COLOR_BORDER } };
  return { top: edge, bottom: edge, left: edge, right: edge };
}

function excelMergeStyles(...styles) {
  return styles.reduce((result, style) => {
    if (!style) {
      return result;
    }
    return {
      ...result,
      ...style,
      alignment: { ...result.alignment, ...style.alignment },
      border: style.border || result.border,
      fill: style.fill || result.fill,
      font: { ...result.font, ...style.font },
    };
  }, {});
}

function excelRowBaseFont(rowKind, itemType) {
  if (rowKind === "child") {
    return { sz: 10, color: { rgb: EXCEL_COLOR_MUTED } };
  }
  if (itemType === "subsection") {
    return { sz: 10, color: { rgb: EXCEL_COLOR_MUTED } };
  }
  if (itemType === "section") {
    return { sz: 11 };
  }
  return { sz: 11 };
}

function excelChildFill(rowKind) {
  if (rowKind !== "child") {
    return null;
  }
  return { patternType: "solid", fgColor: { rgb: EXCEL_COLOR_CHILD_BG } };
}

function excelSmallCellFont(rowKind) {
  return rowKind === "child" ? { sz: 9 } : { sz: 10 };
}

function setExcelStyledCell(worksheet, row, column, value, style) {
  const address = XLSX.utils.encode_cell({ r: row, c: column });
  const text = value == null ? "" : String(value);
  worksheet[address] = {
    v: text,
    t: "s",
    s: style,
  };
}

function collectEstimateExportRows(estimate) {
  const items = estimate.items || [];
  const expanded = getExpandedLineIds(estimate.id);
  const rows = [];
  let positionNumber = 0;

  items.forEach((item) => {
    const isStructural = estimateLineIsStructural(item);
    if (!isStructural) {
      positionNumber += 1;
    }
    const indexLabel = isStructural ? "" : String(positionNumber);
    const hasResources = estimateLineCanExpand(item);
    const isExpanded = expanded.has(item.id);

    rows.push({
      rowKind: "main",
      itemType: item.type || "position",
      indexLabel,
      code: estimateLineCodeExportText(item),
      name: item.name || "",
      unit: item.unit || "",
      quantity: isStructural ? "" : formatNumber(item.quantity),
      unitPrice: formatEstimateUnitPricePlain(item),
      total: formatEstimateMoney(estimateLineTotal(item)),
    });

    if (hasResources && isExpanded) {
      (item.children || []).forEach((child, childIndex) => {
        rows.push({
          rowKind: "child",
          itemType: child.type || "resource",
          indexLabel: `${positionNumber}.${childIndex + 1}`,
          code: estimateLineCodeExportText(child),
          name: child.name || "",
          unit: child.unit || "",
          quantity: formatNumber(child.quantity),
          unitPrice: formatEstimateUnitPricePlain(child),
          total: formatEstimateMoney(estimateLineTotal(child, item)),
        });
      });
    }
  });

  return rows;
}

function estimateExportFileName(estimate) {
  const raw = String(estimate.code || estimate.title || "smeta").trim() || "smeta";
  const safe = raw.replace(/[\\/:*?"<>|]/g, "_").slice(0, 80);
  return `${safe}.xlsx`;
}

function applyEstimateExcelTableCellStyle(row, column, exportRow) {
  const { rowKind, itemType } = exportRow;
  const border = excelCellBorder();
  const fill = excelChildFill(rowKind);
  const rowFont = excelRowBaseFont(rowKind, itemType);
  const compactFont = excelSmallCellFont(rowKind);

  let style = excelMergeStyles(
    { border, alignment: { vertical: "top", wrapText: true }, font: rowFont },
    fill ? { fill } : null,
  );

  if (column === 0) {
    style = excelMergeStyles(style, {
      alignment: { horizontal: "center", vertical: "top", wrapText: false },
      font: { ...rowFont, ...compactFont },
    });
  } else if (column === 1 || column === 3 || column === 4) {
    style = excelMergeStyles(style, { font: { ...rowFont, ...compactFont } });
  } else if (column === 2) {
    style = excelMergeStyles(style, {
      alignment: {
        horizontal: "left",
        vertical: "top",
        wrapText: true,
        indent: rowKind === "child" ? 2 : 0,
      },
    });
  } else if (column === 5) {
    style = excelMergeStyles(style, {
      alignment: { horizontal: "right", vertical: "top", wrapText: true },
      font: { ...rowFont, ...compactFont },
    });
  } else if (column === 6) {
    style = excelMergeStyles(style, {
      alignment: { horizontal: "right", vertical: "top", wrapText: false },
      font: { ...rowFont, ...compactFont },
    });
  }

  return style;
}

function buildEstimateExcelWorksheet(estimate) {
  const items = estimate.items || [];
  const tableRows = collectEstimateExportRows(estimate);
  const metaLabelColumn = 1;
  const metaValueColumn = 2;
  const sheetRows = [
    ["", "Шифр", estimate.code || ""],
    ["", "Наименование", estimate.title || ""],
    ["", "Сметный район", estimate.district || "—"],
    ["", "Сметные цены и индексы", fgisSetDisplayName(estimate.fgisSetId)],
    ["", "Сметная стоимость", money.format(estimateGrandTotal(items))],
    [],
    ["", "Стройка", estimate.constructionLabel || "-"],
    ["", "Объект", estimate.objectLabel || "-"],
    [],
    ["№ п/п", "Шифр", "Наименование", "Ед. изм.", "Объем\n/ расход", "Стоимость ед.", "Стоимость на объем"],
    ...tableRows.map((row) => [
      row.indexLabel,
      row.code,
      row.name,
      row.unit,
      row.quantity,
      row.unitPrice,
      row.total,
    ]),
  ];

  const worksheet = XLSX.utils.aoa_to_sheet(sheetRows);
  const tableHeaderRow = 9;
  const dataStartRow = 10;
  const metaLabelStyle = {
    font: { bold: true, color: { rgb: EXCEL_COLOR_MUTED }, sz: 10 },
    alignment: { vertical: "top" },
  };
  const metaValueStyle = {
    font: { sz: 11 },
    alignment: { vertical: "top", wrapText: true },
  };

  for (let row = 0; row < 5; row += 1) {
    setExcelStyledCell(worksheet, row, metaLabelColumn, sheetRows[row][metaLabelColumn], metaLabelStyle);
    const valueStyle =
      row === 4
        ? excelMergeStyles(metaValueStyle, {
            font: { bold: true, sz: 11 },
            fill: { patternType: "solid", fgColor: { rgb: EXCEL_COLOR_TOTAL_BG } },
            alignment: { horizontal: "right", vertical: "top" },
          })
        : metaValueStyle;
    setExcelStyledCell(worksheet, row, metaValueColumn, sheetRows[row][metaValueColumn], valueStyle);
  }

  setExcelStyledCell(worksheet, 6, metaLabelColumn, sheetRows[6][metaLabelColumn], metaLabelStyle);
  setExcelStyledCell(worksheet, 6, metaValueColumn, sheetRows[6][metaValueColumn], metaValueStyle);
  setExcelStyledCell(worksheet, 7, metaLabelColumn, sheetRows[7][metaLabelColumn], metaLabelStyle);
  setExcelStyledCell(worksheet, 7, metaValueColumn, sheetRows[7][metaValueColumn], metaValueStyle);

  const tableHeaderStyle = {
    border: excelCellBorder(),
    font: { bold: true, color: { rgb: EXCEL_COLOR_MUTED }, sz: 9 },
    alignment: { horizontal: "center", vertical: "center", wrapText: true },
  };
  for (let column = 0; column < 7; column += 1) {
    setExcelStyledCell(worksheet, tableHeaderRow, column, sheetRows[tableHeaderRow][column], tableHeaderStyle);
  }

  tableRows.forEach((exportRow, index) => {
    const rowIndex = dataStartRow + index;
    const values = [
      exportRow.indexLabel,
      exportRow.code,
      exportRow.name,
      exportRow.unit,
      exportRow.quantity,
      exportRow.unitPrice,
      exportRow.total,
    ];
    values.forEach((value, column) => {
      setExcelStyledCell(
        worksheet,
        rowIndex,
        column,
        value,
        applyEstimateExcelTableCellStyle(rowIndex, column, exportRow),
      );
    });
  });

  worksheet["!cols"] = [
    { wch: 5 },
    { wch: 16 },
    { wch: 48 },
    { wch: 8 },
    { wch: 9 },
    { wch: 12 },
    { wch: 14 },
  ];

  return worksheet;
}

async function exportEstimateToExcel(estimateId) {
  const estimate = state.openEstimates.find((item) => item.id === estimateId);
  if (!estimate) {
    return;
  }

  if (typeof XLSX === "undefined") {
    showMessage("Библиотека Excel не загружена", "error");
    return;
  }

  try {
    await syncEstimateTextDraftIfNeeded(estimateId);
  } catch (error) {
    showMessage(error.message || "Не удалось разобрать текст сметы", "error");
    return;
  }

  await loadFGISSets();

  const worksheet = buildEstimateExcelWorksheet(estimate);
  const workbook = XLSX.utils.book_new();
  XLSX.utils.book_append_sheet(workbook, worksheet, "Смета");
  XLSX.writeFile(workbook, estimateExportFileName(estimate), { cellStyles: true });
  showMessage("Файл Excel сформирован", "ok");
}

async function addBufferToEstimate(estimateId) {
  const estimate = state.openEstimates.find((item) => item.id === estimateId);
  if (!estimate || !state.buffer.length) {
    return;
  }

  const bufferItems = [...state.buffer];
  if (!estimate.items) {
    estimate.items = [];
  }

  let baseTimestamp = Date.now();
  let lineCounter = 0;
  const nextLineId = () => `line_${baseTimestamp}_${lineCounter += 1}`;

  try {
    for (const bufferItem of bufferItems) {
      if (bufferItem.source === "user_position") {
        const position = state.userPositions.find((item) => item.id === bufferItem.sourceId);
        if (position) {
          estimate.items.push(
            estimateLineFromUserPosition(position, bufferItem.quantity ?? 1, { id: nextLineId() }),
          );
          continue;
        }

        const quantity = Number(bufferItem.quantity || 1);
        const unitPrice = Number(bufferItem.unitPrice || 0);
        estimate.items.push({
          id: nextLineId(),
          type: "position",
          code: bufferItem.code || "",
          name: bufferItem.name || "",
          unit: bufferItem.unit || "",
          quantity,
          unitPrice,
          total: quantity * unitPrice,
          source: "user_position",
          sourceId: bufferItem.sourceId,
        });
        continue;
      }

      const node = {
        code: bufferItem.code,
        originalCode: bufferItem.originalCode || "",
        name: bufferItem.name || "",
        unit: bufferItem.unit || "",
      };
      const quantity = Number(bufferItem.quantity || 1);
      const records = await fetchGSNHierarchyRecords(
        node.code,
        estimate.fgisSetId || "",
        estimate.district || "",
      );
      const record = records[0];
      const lineId = nextLineId();
      const item = record
        ? estimateLineFromGSNRecord(record, node, lineId)
        : {
            id: lineId,
            type: "position",
            code: node.code || "",
            name: node.name || "",
            unit: node.unit || "",
            quantity,
            unitPrice: 0,
            total: 0,
          };

      if (record) {
        item.quantity = quantity;
        item.total = quantity * Number(item.unitPrice || 0);
      }
      if (bufferItem.source === "gsn") {
        item.source = "gsn";
        item.sourceId = node.code;
      }

      estimate.items.push(item);
      if (item.isWork !== false && item.children?.length) {
        getExpandedLineIds(estimate.id).add(item.id);
        state.estimateLinesExpanded[estimate.id] = [...getExpandedLineIds(estimate.id)];
      }
    }

    state.buffer = [];
    saveEstimateLinesExpanded();
    invalidateEstimateTextDraft(estimate.id);
    saveEditorState();
    await persistOpenEstimate(estimate.id);
    renderEditor();
    const count = bufferItems.length;
    showMessage(
      count === 1 ? "Позиция из буфера добавлена в смету" : `Добавлено позиций из буфера: ${count}`,
      "ok",
    );
  } catch (error) {
    showMessage(error.message || "Не удалось добавить позиции из буфера", "error");
  }
}

function findCachedSourceEstimate(estimateId) {
  return state.estimates.find((item) => sameEstimateId(item.id, estimateId)) || null;
}

function mergeEditorEstimateFromExisting(editorEstimate, existingEstimate) {
  if (!existingEstimate?.items?.length) {
    return editorEstimate;
  }
  const previousById = new Map(existingEstimate.items.map((line) => [line.id, line]));
  editorEstimate.items = editorEstimate.items.map((item) => {
    const previous = previousById.get(item.id);
    if (!previous) {
      return item;
    }
    return {
      ...item,
      sourceCode: previous.sourceCode || item.sourceCode || estimateLineSourceCode(item),
      recordCode: previous.recordCode || item.recordCode || item.code,
      children: previous.children,
      hasResources: previous.hasResources ?? item.hasResources,
    };
  });
  if (existingEstimate.district && !editorEstimate.district) {
    editorEstimate.district = existingEstimate.district;
  }
  if (existingEstimate.fgisSetId && !editorEstimate.fgisSetId) {
    editorEstimate.fgisSetId = existingEstimate.fgisSetId;
  }
  if (existingEstimate.code && !editorEstimate.code) {
    editorEstimate.code = existingEstimate.code;
  }
  if (existingEstimate.title && !editorEstimate.title) {
    editorEstimate.title = existingEstimate.title;
  }
  if (existingEstimate.sourceDataConfigLineRaw) {
    editorEstimate.sourceDataConfigLineRaw = existingEstimate.sourceDataConfigLineRaw;
  }
  if (existingEstimate.sourceDataBimConfig) {
    editorEstimate.sourceDataBimConfig = existingEstimate.sourceDataBimConfig;
  }
  return editorEstimate;
}

function buildEditorEstimateFromSource(sourceEstimate, existingEstimate = null) {
  const object = state.objects.find((item) => item.id === sourceEstimate.objectId);
  const construction = object
    ? state.constructions.find((item) => item.id === object.constructionId)
    : null;

  const editorEstimate = {
    id: sourceEstimate.id,
    objectId: sourceEstimate.objectId,
    code: sourceEstimate.code || "",
    title: sourceEstimate.title || "",
    description: sourceEstimate.description || "",
    status: sourceEstimate.status || "draft",
    district: sourceEstimate.district || "",
    fgisSetId: sourceEstimate.fgisSetId || "",
    sourceDataDocumentSet: sourceEstimate.sourceDataDocumentSet || "",
    sourceDataCalculationFlags: sourceEstimate.sourceDataCalculationFlags || "",
    sourceDataSummaryChapterNo: sourceEstimate.sourceDataSummaryChapterNo || "",
    sourceDataBimConfig: sourceEstimate.sourceDataBimConfig || "",
    sourceDataConfigLineRaw: sourceEstimate.sourceDataConfigLineRaw || "",
    constructionName: construction?.name || "",
    constructionCode: construction?.code || "",
    objectName: object?.name || "",
    objectCode: object?.code || "",
    constructionLabel: construction ? `${construction.code || ""} ${construction.name || ""}`.trim() : "",
    objectLabel: object ? `${object.code || ""} ${object.name || ""}`.trim() : "",
    items: (sourceEstimate.items || []).map((item) => {
      const sourceCode =
        estimateLineSourceCodeFromRawText(item.rawText) || String(item.code || "").trim();
      const editorItem = {
        id: item.id,
        type: item.type || "position",
        source: item.source || "",
        sourceCode,
        code: item.code || extractSourceDataPositionCipher(sourceCode) || "",
        recordCode: item.code || extractSourceDataPositionCipher(sourceCode) || "",
        originalCode: item.originalCode || "",
        name: item.name || "",
        unit: item.unit || "",
        quantity: Number(item.quantity || 0),
        unitPrice: Number(item.unitPrice || 0),
        total: Number(item.total || Number(item.quantity || 0) * Number(item.unitPrice || 0)),
        hasResources: Boolean(item.hasResources),
        rawText: item.rawText || "",
        calcStatus: item.calcStatus || "",
        calcError: item.calcError || "",
        calcJson: item.calcJson || null,
        calcJsonKey: JSON.stringify(item.calcJson || null),
      };
      hydrateEstimateItemFromSourceDataRawText(editorItem);
      return editorItem;
    }),
  };
  hydrateEstimateSourceDataFgisSet(editorEstimate);
  if (existingEstimate) {
    mergeEditorEstimateFromExisting(editorEstimate, existingEstimate);
  }
  return editorEstimate;
}

function resolveEditorEstimateObjectId() {
  if (state.editorTab && state.editorTab !== "buffer") {
    const current = state.openEstimates.find((item) => item.id === state.editorTab);
    if (current?.objectId) {
      return current.objectId;
    }
  }

  for (let index = state.openEstimates.length - 1; index >= 0; index -= 1) {
    const estimate = state.openEstimates[index];
    if (estimate?.objectId) {
      return estimate.objectId;
    }
  }

  const objects = [...state.objects].sort(compareByCode);
  return objects[0]?.id || "";
}

function suggestNextObjectCode(constructionId) {
  const construction = state.constructions.find((item) => item.id === constructionId);
  const prefix = construction?.code || "00";
  const objects = state.objects.filter((item) => item.constructionId === constructionId);
  const nextNumber = objects.length + 1;
  return `${prefix}-${String(nextNumber).padStart(2, "0")}`;
}

function nextEditorEstimateIdentity(objectId) {
  const objectEstimates = state.estimates.filter((item) => item.objectId === objectId);
  const nextNumber = objectEstimates.length + 1;
  return {
    code: `СМ-${String(nextNumber).padStart(3, "0")}`,
    title: `Смета ${nextNumber}`,
  };
}

async function ensureEditorEstimateObject() {
  const existingObjectId = resolveEditorEstimateObjectId();
  if (existingObjectId) {
    return existingObjectId;
  }

  let construction = [...state.constructions].sort(compareByCode)[0];
  if (!construction) {
    construction = await api("/api/constructions", {
      method: "POST",
      body: { code: "00", name: "Новая стройка" },
    });
    state.constructions.push(construction);
  }

  const object = await api("/api/objects", {
    method: "POST",
    body: {
      constructionId: construction.id,
      code: suggestNextObjectCode(construction.id),
      name: "Новый объект",
    },
  });
  state.objects.push(object);
  return object.id;
}

async function syncEstimateToConstructionTree(estimate) {
  const fields = getEstimateConstructionObjectFields(estimate);
  const constructionCode = String(fields.constructionCode || "").trim();
  const constructionName = String(fields.constructionName || "").trim();
  const objectCode = String(fields.objectCode || "").trim();
  const objectName = String(fields.objectName || "").trim();

  if (!constructionCode || !objectCode) {
    return estimate.objectId || "";
  }

  let construction = state.constructions.find((item) => item.code === constructionCode);
  if (!construction) {
    construction = await api("/api/constructions", {
      method: "POST",
      body: {
        code: constructionCode,
        name: constructionName || constructionCode,
      },
    });
    state.constructions.push(construction);
  }

  let object = state.objects.find(
    (item) => item.constructionId === construction.id && item.code === objectCode,
  );
  if (!object) {
    object = await api("/api/objects", {
      method: "POST",
      body: {
        constructionId: construction.id,
        code: objectCode,
        name: objectName || objectCode,
      },
    });
    state.objects.push(object);
  }

  estimate.objectId = object.id;
  estimate.constructionName = construction.name;
  estimate.constructionCode = construction.code;
  estimate.objectName = object.name;
  estimate.objectCode = object.code;
  estimate.constructionLabel = `${construction.code || ""} ${construction.name || ""}`.trim();
  estimate.objectLabel = `${object.code || ""} ${object.name || ""}`.trim();
  state.constructionExpanded[construction.id] = true;
  state.objectExpanded[object.id] = true;
  return object.id;
}

async function createEditorEstimate() {
  if (!state.me) {
    showMessage("Войдите в систему", "error");
    return;
  }

  try {
    await refreshConstructionData();
    const objectId = await ensureEditorEstimateObject();
    const { code, title } = nextEditorEstimateIdentity(objectId);
    const created = await api("/api/estimates", {
      method: "POST",
      body: { objectId, code, title },
    });

    await refreshConstructionData();
    const sourceEstimate = state.estimates.find((item) => item.id === created.id) || created;
    const editorEstimate = buildEditorEstimateFromSource(sourceEstimate);
    state.openEstimates.push(editorEstimate);
    state.editorTab = created.id;
    state.estimateViewMode = "text";
    state.gsnLoaded = false;

    const object = state.objects.find((item) => item.id === objectId);
    if (object) {
      state.constructionExpanded[object.constructionId] = true;
      state.objectExpanded[objectId] = true;
    }

    try {
      await acquireEstimateLock(created.id);
    } catch (error) {
      if (error.status === 409 && error.lock) {
        state.estimateLocks[created.id] = error.lock;
      } else {
        throw error;
      }
    }

    state.section = "editor";
    renderConstructionTree();
    renderSections();
    saveEditorState();
    showMessage("Новая смета создана в структуре «Стройки»", "ok");
  } catch (error) {
    showMessage(error.message || "Не удалось создать смету", "error");
  }
}

function editorItemFromGSNNode(node, quantity) {
  return {
    code: node.code,
    originalCode: node.originalCode || "",
    name: node.name || "",
    unit: node.unit || "",
    quantity: Number(quantity || 1),
    source: "gsn",
    sourceId: node.code,
  };
}

function upsertGSNInBuffer(node, quantity, options = {}) {
  const item = editorItemFromGSNNode(node, quantity);
  const existingIndex = state.buffer.findIndex(
    (bufferItem) => bufferItem.source === "gsn" && bufferItem.sourceId === node.code,
  );
  const isNew = existingIndex < 0;

  if (existingIndex >= 0) {
    state.buffer[existingIndex] = item;
  } else {
    state.buffer.push(item);
  }

  saveEditorState();
  if (state.section === "editor" && state.editorTab === "buffer") {
    renderEditor();
  }

  if (isNew && options.notify !== false) {
    showMessage("Позиция добавлена в буфер", "ok");
  }
}

function removeGSNFromBuffer(nodeCode) {
  let existingIndex = -1;
  for (let index = state.buffer.length - 1; index >= 0; index -= 1) {
    const bufferItem = state.buffer[index];
    if (bufferItem.source === "gsn" && bufferItem.sourceId === nodeCode) {
      existingIndex = index;
      break;
    }
  }

  if (existingIndex < 0) {
    return;
  }

  state.buffer.splice(existingIndex, 1);
  saveEditorState();
  if (state.section === "editor" && state.editorTab === "buffer") {
    renderEditor();
  }
}

async function upsertGSNInEstimate(node, quantity, options = {}) {
  const estimate = getAddLineTargetEstimate();
  if (!estimate) {
    return;
  }

  if (!estimate.items) {
    estimate.items = [];
  }

  const existingIndex = estimate.items.findIndex(
    (item) => item.source === "gsn" && item.sourceId === node.code,
  );

  if (existingIndex >= 0) {
    const item = estimate.items[existingIndex];
    const qty = Number(quantity || 1);
    item.quantity = qty;
    item.total = qty * Number(item.unitPrice || 0);
    invalidateEstimateTextDraft(estimate.id);
    saveEditorState();
    await persistOpenEstimate(estimate.id);
    if (state.section === "editor" && state.editorTab === estimate.id) {
      renderEditor();
    }
    return;
  }

  if (options.deactivated) {
    return;
  }

  try {
    const records = await fetchGSNHierarchyRecords(node.code, estimate.fgisSetId || "", estimate.district || "");
    const record = records[0];
    const lineId = `line_${Date.now()}`;
    const item = record
      ? estimateLineFromGSNRecord(record, node, lineId)
      : {
          id: lineId,
          type: "position",
          code: node.code || "",
          name: node.name || "",
          unit: node.unit || "",
          quantity: 1,
          unitPrice: 0,
          total: 0,
        };

    const qty = Number(quantity || 1);
    item.quantity = qty;
    item.total = qty * Number(item.unitPrice || 0);
    item.source = "gsn";
    item.sourceId = node.code;

    estimate.items.push(item);
    if (item.isWork !== false && item.children?.length) {
      getExpandedLineIds(estimate.id).add(item.id);
      state.estimateLinesExpanded[estimate.id] = [...getExpandedLineIds(estimate.id)];
      saveEstimateLinesExpanded();
    }

    invalidateEstimateTextDraft(estimate.id);
    saveEditorState();
    await persistOpenEstimate(estimate.id);
    if (state.section === "editor" && state.editorTab === estimate.id) {
      renderEditor();
    }
    if (options.notify !== false) {
      showMessage("Позиция добавлена в смету", "ok");
    }
  } catch (error) {
    showMessage(error.message || "Не удалось добавить позицию", "error");
  }
}

async function removeGSNFromEstimate(nodeCode) {
  const estimate = getAddLineTargetEstimate();
  if (!estimate?.items?.length) {
    return;
  }

  let existingIndex = -1;
  for (let index = estimate.items.length - 1; index >= 0; index -= 1) {
    const item = estimate.items[index];
    if (item.source === "gsn" && item.sourceId === nodeCode) {
      existingIndex = index;
      break;
    }
  }

  if (existingIndex < 0) {
    return;
  }

  const [line] = estimate.items.splice(existingIndex, 1);
  const expanded = getExpandedLineIds(estimate.id);
  if (expanded.delete(line.id)) {
    state.estimateLinesExpanded[estimate.id] = [...expanded];
    saveEstimateLinesExpanded();
  }

  saveEditorState();
  await persistOpenEstimate(estimate.id);
  if (state.section === "editor" && state.editorTab === estimate.id) {
    renderEditor();
  }
}

function addNodeToBuffer(node) {
  upsertGSNInBuffer(node, addPrimitiveInitialQuantity, { notify: true });
}

function editorItemFromUserPosition(position, quantity) {
  return {
    code: position.code || "",
    name: position.name || "",
    unit: position.unit || "",
    quantity: Number(quantity || 1),
    unitPrice: Number(position.cost || 0),
    source: "user_position",
    sourceId: position.id,
  };
}

function estimateLineFromUserPosition(position, quantity, existingItem = null) {
  const qty = Number(quantity || 1);
  const unitPrice = Number(position.cost || 0);
  return {
    id: existingItem?.id || `line_${Date.now()}`,
    type: "position",
    code: position.code || "",
    name: position.name || "",
    unit: position.unit || "",
    quantity: qty,
    unitPrice,
    total: qty * unitPrice,
    source: "user_position",
    sourceId: position.id,
  };
}

function getOnlyOpenEstimate() {
  if (state.openEstimates.length !== 1) {
    return null;
  }
  return state.openEstimates[0];
}

function upsertUserPositionInBuffer(positionId, quantity, options = {}) {
  const position = state.userPositions.find((item) => item.id === positionId);
  if (!position) {
    return;
  }

  const item = editorItemFromUserPosition(position, quantity);
  const existingIndex = state.buffer.findIndex(
    (bufferItem) => bufferItem.source === "user_position" && bufferItem.sourceId === positionId,
  );
  const isNew = existingIndex < 0;

  if (existingIndex >= 0) {
    state.buffer[existingIndex] = item;
  } else {
    state.buffer.push(item);
  }

  saveEditorState();
  if (state.section === "editor" && state.editorTab === "buffer") {
    renderEditor();
  }

  if (isNew && options.notify !== false) {
    showMessage("Позиция добавлена в буфер", "ok");
  }
}

function removeUserPositionFromBuffer(positionId) {
  let existingIndex = -1;
  for (let index = state.buffer.length - 1; index >= 0; index -= 1) {
    const bufferItem = state.buffer[index];
    if (bufferItem.source === "user_position" && bufferItem.sourceId === positionId) {
      existingIndex = index;
      break;
    }
  }

  if (existingIndex < 0) {
    return;
  }

  state.buffer.splice(existingIndex, 1);
  saveEditorState();
  if (state.section === "editor" && state.editorTab === "buffer") {
    renderEditor();
  }
}

function upsertUserPositionInEstimate(positionId, quantity, options = {}) {
  const estimate = getAddLineTargetEstimate();
  if (!estimate) {
    return;
  }

  const position = state.userPositions.find((item) => item.id === positionId);
  if (!position) {
    return;
  }

  if (!estimate.items) {
    estimate.items = [];
  }

  const existingIndex = estimate.items.findIndex(
    (item) => item.source === "user_position" && item.sourceId === positionId,
  );
  const existingItem = existingIndex >= 0 ? estimate.items[existingIndex] : null;
  const item = estimateLineFromUserPosition(position, quantity, existingItem);
  const isNew = existingIndex < 0;

  if (existingIndex >= 0) {
    estimate.items[existingIndex] = item;
  } else {
    estimate.items.push(item);
  }

  invalidateEstimateTextDraft(estimate.id);
  saveEditorState();
  if (state.section === "editor" && state.editorTab === estimate.id) {
    renderEditor();
  }
  void persistOpenEstimate(estimate.id)
    .then(() => {
      if (isNew && options.notify !== false) {
        showMessage("Позиция добавлена в смету", "ok");
      }
    })
    .catch(() => {});
}

function removeUserPositionFromEstimate(positionId) {
  const estimate = getAddLineTargetEstimate();
  if (!estimate || !estimate.items?.length) {
    return;
  }

  let existingIndex = -1;
  for (let index = estimate.items.length - 1; index >= 0; index -= 1) {
    const item = estimate.items[index];
    if (item.source === "user_position" && item.sourceId === positionId) {
      existingIndex = index;
      break;
    }
  }

  if (existingIndex < 0) {
    return;
  }

  estimate.items.splice(existingIndex, 1);
  invalidateEstimateTextDraft(estimate.id);
  saveEditorState();
  void persistOpenEstimate(estimate.id).then(() => {
    if (state.section === "editor" && state.editorTab === estimate.id) {
      renderEditor();
    }
  });
}

function parseUserPositionEstimateKey(key) {
  const prefix = "estimate:";
  if (!key.startsWith(prefix)) {
    return null;
  }

  const rest = key.slice(prefix.length);
  const marker = ":user-position:";
  const markerIndex = rest.indexOf(marker);
  if (markerIndex < 0) {
    return null;
  }

  return {
    estimateId: rest.slice(0, markerIndex),
    positionId: rest.slice(markerIndex + marker.length),
  };
}

function onAddPrimitiveQuantityChange(key, quantity, options = {}) {
  if (key.startsWith("buffer:gsn:")) {
    const node = getAddPrimitiveGSNNode(key);
    if (!node) {
      return;
    }
    if (options.deactivated) {
      removeGSNFromBuffer(node.code);
      return;
    }
    upsertGSNInBuffer(node, quantity, options);
    return;
  }

  if (key.includes(":gsn:")) {
    const node = getAddPrimitiveGSNNode(key);
    if (!node) {
      return;
    }
    if (options.deactivated) {
      void removeGSNFromEstimate(node.code);
      return;
    }
    void upsertGSNInEstimate(node, quantity, options);
    return;
  }

  if (key.startsWith("buffer:user-position:")) {
    const positionId = key.slice("buffer:user-position:".length);
    if (options.deactivated) {
      removeUserPositionFromBuffer(positionId);
      return;
    }
    upsertUserPositionInBuffer(positionId, quantity, options);
    return;
  }

  const estimateKey = parseUserPositionEstimateKey(key);
  if (estimateKey) {
    if (options.deactivated) {
      removeUserPositionFromEstimate(estimateKey.positionId);
      return;
    }
    upsertUserPositionInEstimate(estimateKey.positionId, quantity, options);
  }
}

function addNodeToOnlyEstimate(node) {
  void upsertGSNInEstimate(node, addPrimitiveInitialQuantity, { notify: true });
}

function editorItemFromNode(node) {
  return {
    code: node.code,
    originalCode: node.originalCode || "",
    name: node.name || "",
    unit: node.unit || "",
  };
}

function compactEstimateLineForSession(item) {
  if (!item || typeof item !== "object") {
    return item;
  }
  return {
    id: item.id,
    type: item.type,
    source: item.source,
    sourceCode: item.sourceCode,
    code: item.code,
    recordCode: item.recordCode,
    originalCode: item.originalCode,
    name: item.name,
    unit: item.unit,
    quantity: item.quantity,
    unitPrice: item.unitPrice,
    total: item.total,
    hasResources: item.hasResources,
    rawText: item.rawText,
    calcStatus: item.calcStatus,
    calcError: item.calcError,
    isWork: item.isWork,
    unitPriceText: item.unitPriceText,
    unitPriceIndex: item.unitPriceIndex,
  };
}

function compactOpenEstimatesForSession(estimates) {
  return (estimates || []).map((estimate) => ({
    id: estimate.id,
    objectId: estimate.objectId,
    code: estimate.code,
    title: estimate.title,
    description: estimate.description,
    status: estimate.status,
    district: estimate.district,
    fgisSetId: estimate.fgisSetId,
    sourceDataDocumentSet: estimate.sourceDataDocumentSet,
    sourceDataCalculationFlags: estimate.sourceDataCalculationFlags,
    sourceDataSummaryChapterNo: estimate.sourceDataSummaryChapterNo,
    sourceDataBimConfig: estimate.sourceDataBimConfig,
    sourceDataConfigLineRaw: estimate.sourceDataConfigLineRaw,
    constructionName: estimate.constructionName,
    constructionCode: estimate.constructionCode,
    objectName: estimate.objectName,
    objectCode: estimate.objectCode,
    constructionLabel: estimate.constructionLabel,
    objectLabel: estimate.objectLabel,
    items: Array.isArray(estimate.items) ? estimate.items.map(compactEstimateLineForSession) : [],
  }));
}

function writeSessionJSON(key, value) {
  sessionStorage.setItem(key, JSON.stringify(value));
}

function saveEditorState() {
  try {
    writeSessionJSON("nav_editor_buffer", state.buffer);
    writeSessionJSON("nav_editor_estimates", compactOpenEstimatesForSession(state.openEstimates));
    sessionStorage.setItem("nav_editor_tab", state.editorTab);
  } catch {
    try {
      writeSessionJSON(
        "nav_editor_estimates",
        state.openEstimates.map((estimate) => ({
          id: estimate.id,
          objectId: estimate.objectId,
        })),
      );
      sessionStorage.setItem("nav_editor_tab", state.editorTab);
    } catch {
      // Квота sessionStorage не должна блокировать работу редактора в памяти.
    }
  }
  refreshUserPositionsPanelIfVisible();
  refreshGSNLeafActionsInDOM();
}

function isPersistedEstimateId(estimateId) {
  if (!estimateId || String(estimateId).startsWith("session_est_")) {
    return false;
  }
  if (state.estimates.some((item) => item.id === estimateId)) {
    return true;
  }
  const openEstimate = state.openEstimates.find((item) => item.id === estimateId);
  return Boolean(openEstimate?.objectId);
}

function estimateItemsForApi(items) {
  return (items || []).map((item) => {
    if (item.source === "gsn") {
      return {
        id: item.id || "",
        type: item.type || "position",
        source: "gsn",
        code: estimateLineSourceCode(item) || estimateRecordFetchCode(item) || item.code || "",
        quantity: Number(item.quantity || 0),
        rawText: item.rawText || "",
      };
    }

    return {
      id: item.id || "",
      type: item.type || "position",
      source: item.source || "",
      code: estimateRecordFetchCode(item) || item.code || "",
      originalCode: String(item.originalCode || "").trim(),
      name: item.name || "",
      quantity: Number(item.quantity || 0),
      unit: item.unit || "",
      unitPrice: Number(item.unitPrice || 0),
      total: Number(item.total ?? Number(item.quantity || 0) * Number(item.unitPrice || 0)),
      rawText: item.rawText || "",
    };
  });
}

function estimateLineSyncKey(item) {
  if (!item) {
    return "";
  }
  return [
    item.type || "",
    item.source || "",
    item.code || "",
    item.originalCode || "",
    item.rawText || "",
  ].join("|");
}

async function persistOpenEstimate(estimateId, options = {}) {
  if (!state.me || !isPersistedEstimateId(estimateId)) {
    return;
  }

  const pending = persistEstimateInflight.get(estimateId);
  if (pending) {
    await pending;
  }

  const editorEstimate = state.openEstimates.find((item) => item.id === estimateId);
  if (!editorEstimate) {
    return;
  }

  if (!options.skipTextDraftSync) {
    try {
      await syncEstimateTextDraftIfNeeded(estimateId);
    } catch {
      return;
    }
  }

  try {
    await syncEstimateToConstructionTree(editorEstimate);
  } catch (error) {
    showMessage(error.message || "Не удалось обновить структуру «Стройки»", "error");
    throw error;
  }

  const sourceEstimate = state.estimates.find((item) => item.id === estimateId);
  const objectId = editorEstimate.objectId || sourceEstimate?.objectId;
  const code = editorEstimate.code || sourceEstimate?.code;
  const title = editorEstimate.title || sourceEstimate?.title;
  if (!objectId || !code || !title) {
    const message = "Не удалось сохранить смету: укажите объект, шифр и наименование";
    if (options.requireSave) {
      throw new Error(message);
    }
    return;
  }

  const request = (async () => {
    try {
      const updated = await api(`/api/estimates/${estimateId}`, {
        method: "PUT",
        body: {
          objectId,
          code,
          title,
          description: editorEstimate.description ?? sourceEstimate?.description ?? "",
          district: editorEstimate.district ?? sourceEstimate?.district ?? "",
          fgisSetId: editorEstimate.fgisSetId ?? sourceEstimate?.fgisSetId ?? "",
          status: editorEstimate.status || sourceEstimate?.status || "draft",
          items: estimateItemsForApi(editorEstimate.items),
        },
      });

      const index = state.estimates.findIndex((item) => item.id === estimateId);
      if (index >= 0) {
        state.estimates[index] = updated;
      } else {
        state.estimates.push(updated);
      }
      if (updated.fgisSetId !== undefined) {
        editorEstimate.fgisSetId = String(updated.fgisSetId || "");
        if (editorEstimate.fgisSetId) {
          applyFgisSetToEstimateSourceData(editorEstimate, editorEstimate.fgisSetId);
        }
      }
      if (Array.isArray(updated.items) && state.estimateViewMode !== "table") {
        const localItems = editorEstimate.items || [];
        const localById = new Map(localItems.map((item) => [item.id, item]));
        const localByKey = new Map();
        localItems.forEach((item) => {
          const key = estimateLineSyncKey(item);
          if (!key) {
            return;
          }
          const bucket = localByKey.get(key);
          if (bucket) {
            bucket.push(item);
          } else {
            localByKey.set(key, [item]);
          }
        });
        editorEstimate.items = updated.items.map((saved) => {
          let local = localById.get(saved.id) || null;
          if (!local) {
            const key = estimateLineSyncKey(saved);
            const bucket = localByKey.get(key);
            if (bucket?.length) {
              local = bucket.shift();
            }
          }
          const merged = {
            ...(local || {}),
            ...saved,
            quantity: Number(saved.quantity || 0),
            rawText: saved.rawText || local?.rawText || "",
          };
          if (shouldPreserveLocalGsnCalcOnMerge(local, saved)) {
            merged.calcStatus = local.calcStatus;
            merged.calcError = local.calcError;
            merged.calcJson = local.calcJson;
            merged.calcJsonKey = local.calcJsonKey;
            merged.calcRecordAppliedKey = local.calcRecordAppliedKey;
            merged.originalCode = local.originalCode || merged.originalCode;
            merged.name = local.name || merged.name;
            merged.unit = local.unit || merged.unit;
            merged.unitPrice = local.unitPrice;
            merged.unitPriceText = local.unitPriceText;
            merged.unitPriceIndex = local.unitPriceIndex;
            merged.total = local.total;
            merged.hasResources = local.hasResources;
            merged.children = local.children;
            merged.isWork = local.isWork;
          } else if (Number(saved?.total || 0) > 0) {
            merged.total = Number(saved.total);
            merged.unitPrice = Number(saved.unitPrice || merged.unitPrice || 0);
          }
          return merged;
        });
        editorEstimate.items.forEach(hydrateEstimateItemFromSourceDataRawText);
        ensureEstimateCalcProgressTarget(estimateId, editorEstimate);
        if (!estimateHasInFlightGsnCalc(editorEstimate.items)) {
          clearEstimateCalcAwaitingServer(estimateId);
        }
        if (state.editorTab === estimateId) {
          if (state.estimateViewMode === "table" && isEditorContentMountedForEstimate(estimateId)) {
            if (isEditorHeaderFieldActive()) {
              pendingEditorRemountEstimateId = estimateId;
            }
            updateEditorTableTotals(editorEstimate);
          } else {
            renderEditor();
          }
        }
      }
      renderConstructionTree();
    } catch (error) {
      showMessage(error.message || "Не удалось сохранить смету", "error");
      throw error;
    }
  })();

  persistEstimateInflight.set(estimateId, request);
  try {
    await request;
  } finally {
    if (persistEstimateInflight.get(estimateId) === request) {
      persistEstimateInflight.delete(estimateId);
    }
  }
}

async function flushPersistOpenEstimate(estimateId) {
  await persistOpenEstimate(estimateId);
}

function editorEstimateLabel(estimate) {
  const code = String(estimate.code || "").trim();
  const title = String(estimate.title || "").trim();
  if (code && title) {
    return `${code} ${title}`;
  }
  return title || code || "Смета";
}

function openEstimateAddLineDialog(estimateID) {
  state.addLineTargetEstimateId = estimateID;
  state.editorTab = estimateID;
  sessionStorage.setItem("nav_editor_tab", estimateID);
  els.estimateAddLineDialogForm.elements.estimateId.value = estimateID;
  els.estimateAddLineDialog.showModal();
}

function navigateToBaseForAddLine(estimateID, baseTab) {
  state.addLineTargetEstimateId = estimateID;
  state.editorTab = estimateID;
  sessionStorage.setItem("nav_editor_tab", estimateID);
  state.section = "base";
  state.baseTab = baseTab;
  if (baseTab === "gsn") {
    state.gsnLoaded = false;
  }
  renderSections();
}

function openEstimateStructuralLineDialog(estimateID, lineType) {
  const form = els.estimateLineDialogForm;
  form.elements.estimateId.value = estimateID;
  form.elements.lineId.value = "";
  form.elements.lineType.value = lineType;
  els.estimateLineDialogTitle.textContent =
    lineType === "section" ? "Добавить раздел" : "Добавить подраздел";
  configureEstimateLineDialogForm({ mode: "addStructural" });
  form.elements.name.value = "";
  els.estimateLineDialog.showModal();
  form.elements.name.focus();
}

function openEstimateLineDialog(estimateID, lineID = "") {
  const form = els.estimateLineDialogForm;
  form.elements.estimateId.value = estimateID;
  form.elements.lineId.value = lineID || "";

  const estimate = state.openEstimates.find((item) => item.id === estimateID);
  const line = estimate?.items?.find((item) => item.id === lineID);
  els.estimateLineDialogTitle.textContent = "Редактировать строку сметы";
  configureEstimateLineDialogForm({ mode: "edit", line });
  form.elements.name.value = line?.name || "";
  form.elements.code.value = estimateLineDisplayCode(line);
  form.elements.unit.value = line?.unit || "";
  form.elements.quantity.value = formatEstimateLineQuantityInput(line?.quantity);
  form.elements.unitPrice.value = formatEstimateLinePriceInput(line);

  els.estimateLineDialog.showModal();
  form.elements.name.focus();
}

async function updateEstimateHeaderField(estimateID, field, value) {
  const estimate = state.openEstimates.find((item) => item.id === estimateID);
  if (!estimate) {
    return;
  }

  const trimmed = String(value || "").trim();
  if (field === "code") {
    estimate.code = trimmed;
  } else if (field === "title") {
    estimate.title = trimmed;
  } else {
    return;
  }

  saveEditorState();
  renderEditorSubitems();
  try {
    await persistOpenEstimate(estimateID);
  } catch {
    return;
  }
}

async function updateEstimateFgisSet(estimateID, value) {
  const estimate = state.openEstimates.find((item) => item.id === estimateID);
  if (!estimate) {
    return;
  }

  const previousFgisSetId = estimate.fgisSetId || "";
  const nextFgisSetId = String(value || "");
  if (nextFgisSetId === previousFgisSetId) {
    return;
  }

  estimate.fgisSetId = nextFgisSetId;
  applyFgisSetToEstimateSourceData(estimate, nextFgisSetId);
  writeEstimateTextDraftFromState(estimateID);

  const blocked = await syncLicenseSessions(getEditorTableLicenseSubsectionIdsForFgisSet(nextFgisSetId));
  saveEditorState();
  refreshEditorFgisSetSelect(estimate);

  if (blocked.length) {
    setSectionLicenseBlocked("editor", true);
    renderLicenseBlockMessage("editor", blocked);
    showMessage(formatLicenseBlockedMessage(blocked), "error");
    try {
      await persistOpenEstimate(estimateID, { skipTextDraftSync: true });
    } catch {
      return;
    }
    return;
  }

  setSectionLicenseBlocked("editor", false);
  (estimate.items || []).forEach(clearGsnLineCalcEnrichment);
  if (state.estimateViewMode === "table" && state.editorTab === estimateID) {
    restartEstimateTableCalculation(estimateID, estimate);
  }

  try {
    renderEditor();
    startLicenseSessionSync();
    await persistOpenEstimate(estimateID, { skipTextDraftSync: true });
    void pollEstimateCalcStatus(estimateID);
    showMessage("Набор сметных цен обновлён, позиции поставлены в очередь на пересчёт", "ok");
  } catch (error) {
    estimate.fgisSetId = previousFgisSetId;
    applyFgisSetToEstimateSourceData(estimate, previousFgisSetId);
    writeEstimateTextDraftFromState(estimateID);
    refreshEditorFgisSetSelect(estimate);
    showMessage(error.message || "Не удалось пересчитать стоимости", "error");
    saveEditorState();
    renderEditor();
    try {
      await persistOpenEstimate(estimateID, { skipTextDraftSync: true });
    } catch {
      return;
    }
  }
}

async function updateEstimateLine(estimateID, lineID, fields) {
  const estimate = state.openEstimates.find((item) => item.id === estimateID);
  if (!estimate?.items?.length) {
    return;
  }

  const line = estimate.items.find((item) => item.id === lineID);
  if (!line) {
    return;
  }

  const trimmedName = String(fields.name || "").trim();
  if (!trimmedName) {
    showMessage("Укажите наименование строки", "error");
    return;
  }

  line.name = trimmedName;

  if (line.type === "position") {
    const trimmedCode = String(fields.code || "").trim();
    line.code = trimmedCode;
    if (String(line.originalCode || "").trim()) {
      line.originalCode = trimmedCode;
    }

    line.unit = String(fields.unit || "").trim();
    if (!line.unit) {
      line.unit = "шт";
    }

    line.quantity = parseLocalizedNumber(fields.quantity);

    if (!estimateLineIsWorkPosition(line)) {
      line.unitPrice = parseLocalizedNumber(fields.unitPrice);
      line.total = line.quantity * line.unitPrice;
    } else if (line.children?.length) {
      line.total = estimateLineTotal(line);
    }
  } else {
    line.unit = "";
  }

  invalidateEstimateTextDraft(estimateID);
  saveEditorState();
  await persistOpenEstimate(estimateID);
  renderEditor();
  showMessage("Строка обновлена", "ok");
}

async function deleteEstimateLine(estimateID, lineID) {
  const estimate = state.openEstimates.find((item) => item.id === estimateID);
  if (!estimate?.items?.length) {
    return;
  }

  const index = estimate.items.findIndex((item) => item.id === lineID);
  if (index < 0) {
    return;
  }

  const line = estimate.items[index];
  const lineLabel = String(line.name || line.code || "строка").trim();
  if (!window.confirm(`Удалить строку «${lineLabel}»?`)) {
    return;
  }

  estimate.items.splice(index, 1);
  const expanded = getExpandedLineIds(estimateID);
  if (expanded.delete(lineID)) {
    state.estimateLinesExpanded[estimateID] = [...expanded];
    saveEstimateLinesExpanded();
  }
  invalidateEstimateTextDraft(estimateID);
  saveEditorState();
  await persistOpenEstimate(estimateID);
  renderEditor();
  showMessage("Строка удалена", "ok");
}

async function moveEstimateLine(estimateID, lineID, direction) {
  const estimate = state.openEstimates.find((item) => item.id === estimateID);
  if (!estimate?.items?.length) {
    return;
  }

  const index = estimate.items.findIndex((item) => item.id === lineID);
  const targetIndex = index + direction;
  if (index < 0 || targetIndex < 0 || targetIndex >= estimate.items.length) {
    return;
  }

  const [line] = estimate.items.splice(index, 1);
  estimate.items.splice(targetIndex, 0, line);
  invalidateEstimateTextDraft(estimateID);
  saveEditorState();
  await persistOpenEstimate(estimateID);
  renderEditor();
}

async function addEstimateLine(estimateID, lineType, name) {
  const estimate = state.openEstimates.find((item) => item.id === estimateID);
  if (!estimate) {
    return;
  }

  const trimmedName = String(name || "").trim();
  if (!trimmedName) {
    showMessage("Укажите наименование строки", "error");
    return;
  }

  if (!estimate.items) {
    estimate.items = [];
  }

  estimate.items.push({
    id: `line_${Date.now()}`,
    type: lineType,
    code: "",
    name: trimmedName,
    unit: lineType === "position" ? "шт" : "",
    quantity: 0,
    unitPrice: 0,
    total: 0,
  });

  invalidateEstimateTextDraft(estimateID);
  saveEditorState();
  await persistOpenEstimate(estimateID);
  renderEditor();
  showMessage("Строка добавлена", "ok");
}

async function closeEstimateFromEditor(estimateID) {
  const index = state.openEstimates.findIndex((item) => item.id === estimateID);
  if (index < 0) {
    return;
  }

  try {
    await syncEstimateTextDraftIfNeeded(estimateID);
  } catch (error) {
    showMessage(error.message || "Не удалось разобрать текст сметы", "error");
    return;
  }

  await flushPersistOpenEstimate(estimateID);

  state.openEstimates.splice(index, 1);
  invalidateEstimateTextDraft(estimateID);
  await releaseEstimateLock(estimateID);
  void refreshEstimateLocks();

  if (state.editorTab === estimateID) {
    state.editorTab =
      state.openEstimates.length > 0
        ? state.openEstimates[Math.min(index, state.openEstimates.length - 1)].id
        : "buffer";
  }

  state.gsnLoaded = false;
  saveEditorState();
  renderEditor();
  showMessage("Смета закрыта", "ok");
}

async function openEstimateInEditor(estimateID) {
  const estimateId = normalizeEstimateId(estimateID);
  if (!estimateId) {
    showMessage("Смета не найдена", "error");
    return;
  }

  if (isEstimateLockedByOther(estimateId)) {
    const lock = getEstimateLockInfo(estimateId);
    showMessage(`Смета редактируется ${lock?.userName || "другим пользователем"}`, "error");
    return;
  }

  const existingIndex = state.openEstimates.findIndex((item) => sameEstimateId(item.id, estimateId));
  if (existingIndex >= 0) {
    try {
      await acquireEstimateLock(estimateId);
    } catch (error) {
      if (error.status === 409 && error.lock) {
        state.estimateLocks[estimateId] = error.lock;
        renderConstructionTree();
        showMessage(`Смета редактируется ${error.lock.userName}`, "error");
        return;
      }
      showMessage(error.message || "Не удалось открыть смету в редакторе", "error");
      return;
    }
    state.estimateViewMode = "text";
    state.editorTab = state.openEstimates[existingIndex].id;
    state.section = "editor";
    renderSections();
    saveEditorState();
    showMessage("Смета открыта в редакторе", "ok");
    return;
  }

  try {
    try {
      await acquireEstimateLock(estimateId);
    } catch (error) {
      if (error.status === 409 && error.lock) {
        state.estimateLocks[estimateId] = error.lock;
        renderConstructionTree();
        showMessage(`Смета редактируется ${error.lock.userName}`, "error");
        return;
      }
      throw error;
    }

    let sourceEstimate = findCachedSourceEstimate(estimateId);
    if (!sourceEstimate) {
      await refreshConstructionData();
      sourceEstimate = findCachedSourceEstimate(estimateId);
    }
    if (!sourceEstimate) {
      showMessage("Смета не найдена", "error");
      return;
    }

    const existingEstimate = state.openEstimates.find((item) => sameEstimateId(item.id, estimateId));
    editorOpeningEstimateId = estimateId;
    state.estimateViewMode = "text";
    state.editorTab = estimateId;
    state.section = "editor";
    renderSections();

    await new Promise((resolve) => window.setTimeout(resolve, 0));

    if (editorOpeningEstimateId !== estimateId) {
      return;
    }

    const editorEstimate = buildEditorEstimateFromSource(sourceEstimate, existingEstimate);
    editorEstimate.id = normalizeEstimateId(editorEstimate.id);
    hydrateEstimateSourceDataFgisSet(editorEstimate);

    if (existingEstimate?.district && !editorEstimate.district) {
      void persistOpenEstimate(editorEstimate.id).catch(() => {});
    }
    if (existingEstimate?.fgisSetId && !editorEstimate.fgisSetId) {
      void persistOpenEstimate(editorEstimate.id).catch(() => {});
    }

    if (editorOpeningEstimateId !== estimateId) {
      return;
    }

    state.openEstimates.push(editorEstimate);
    editorOpeningEstimateId = null;
    invalidateEstimateTextDraft(editorEstimate.id);
    renderEditor();
    saveEditorState();
    showMessage("Смета открыта в редакторе", "ok");
  } catch (error) {
    if (editorOpeningEstimateId === estimateId) {
      editorOpeningEstimateId = null;
      if (state.section === "editor" && sameEstimateId(state.editorTab, estimateId)) {
        state.editorTab = "buffer";
        state.section = "constructions";
        renderSections();
      }
    }
    showMessage(error.message || "Не удалось открыть смету в редакторе", "error");
  }
}

function formatNumber(value) {
  const number = Number(value || 0);
  return Number.isFinite(number) ? new Intl.NumberFormat("ru-RU").format(number) : "0";
}

const quantityInputFormat = new Intl.NumberFormat("ru-RU", { maximumFractionDigits: 6 });

function parseQuantityInput(value) {
  const text = String(value).trim().replace(/\s/g, "").replace(",", ".");
  if (!text) {
    return null;
  }
  const number = Number(text);
  if (!Number.isFinite(number) || number <= 0) {
    return null;
  }
  return number;
}

function formatQuantityInput(value) {
  const number = Number(value);
  if (!Number.isFinite(number) || number <= 0) {
    return "1";
  }
  return quantityInputFormat.format(number);
}

const addPrimitiveInitialQuantity = 1;

function renderAddPrimitive(key, label, quantity = null) {
  if (quantity == null) {
    return `
      <button
        class="add-primitive secondary"
        data-add-primitive="${escapeHTML(key)}"
        data-add-primitive-label="${escapeHTML(label)}"
        type="button"
      >
        ${escapeHTML(label)}
      </button>
    `;
  }

  return `
    <div
      class="add-primitive-stepper"
      data-add-primitive-stepper="${escapeHTML(key)}"
      data-add-primitive-label="${escapeHTML(label)}"
    >
      <button
        class="add-primitive-btn secondary"
        data-add-primitive-minus="${escapeHTML(key)}"
        type="button"
        aria-label="Уменьшить"
      >−</button>
      <input
        class="add-primitive-qty"
        data-add-primitive-qty="${escapeHTML(key)}"
        type="text"
        inputmode="decimal"
        value="${escapeHTML(formatQuantityInput(quantity))}"
        aria-label="Количество"
      />
      <button
        class="add-primitive-btn secondary"
        data-add-primitive-plus="${escapeHTML(key)}"
        type="button"
        aria-label="Увеличить"
      >+</button>
    </div>
  `;
}

function setAddPrimitiveQuantity(container, key, quantity) {
  state.addPrimitiveQuantities[key] = quantity;
  const input = container.querySelector(`[data-add-primitive-qty="${key}"]`);
  if (input) {
    input.value = formatQuantityInput(quantity);
  }
}

function collapseAddPrimitive(container, key, label) {
  delete state.addPrimitiveQuantities[key];
  const stepper = container.querySelector(`[data-add-primitive-stepper="${key}"]`);
  if (stepper) {
    stepper.outerHTML = renderAddPrimitive(key, label, null);
  }
}

function getAddPrimitiveCurrentQuantity(container, key) {
  const input = container.querySelector(`[data-add-primitive-qty="${key}"]`);
  const fromInput = input ? parseQuantityInput(input.value) : null;
  if (fromInput != null) {
    return fromInput;
  }
  return state.addPrimitiveQuantities[key] ?? addPrimitiveInitialQuantity;
}

function handleAddPrimitiveClick(event, container, onQuantityChange) {
  const activateButton = event.target.closest("[data-add-primitive]");
  if (activateButton) {
    const key = activateButton.dataset.addPrimitive;
    const label = activateButton.dataset.addPrimitiveLabel || "";
    const quantity = addPrimitiveInitialQuantity;
    state.addPrimitiveQuantities[key] = quantity;
    activateButton.outerHTML = renderAddPrimitive(key, label, quantity);
    onQuantityChange(key, quantity, { first: true });
    const input = container.querySelector(`[data-add-primitive-qty="${key}"]`);
    input?.focus();
    input?.select();
    return true;
  }

  const minusButton = event.target.closest("[data-add-primitive-minus]");
  if (minusButton) {
    const key = minusButton.dataset.addPrimitiveMinus;
    const stepper = minusButton.closest("[data-add-primitive-stepper]");
    const label = stepper?.dataset.addPrimitiveLabel || "";
    const current = getAddPrimitiveCurrentQuantity(container, key);
    if (current <= addPrimitiveInitialQuantity) {
      collapseAddPrimitive(container, key, label);
      onQuantityChange(key, null, { deactivated: true });
      return true;
    }

    const quantity = current - 1;
    setAddPrimitiveQuantity(container, key, quantity);
    onQuantityChange(key, quantity);
    return true;
  }

  const plusButton = event.target.closest("[data-add-primitive-plus]");
  if (plusButton) {
    const key = plusButton.dataset.addPrimitivePlus;
    const current = getAddPrimitiveCurrentQuantity(container, key);
    const quantity = current + 1;
    setAddPrimitiveQuantity(container, key, quantity);
    onQuantityChange(key, quantity);
    return true;
  }

  return false;
}

function handleAddPrimitiveQuantityBlur(event, container, onQuantityChange) {
  const input = event.target.closest("[data-add-primitive-qty]");
  if (!input) {
    return;
  }

  const key = input.dataset.addPrimitiveQty;
  window.setTimeout(() => {
    if (!container.querySelector(`[data-add-primitive-qty="${key}"]`)) {
      return;
    }

    const activeInput = container.querySelector(`[data-add-primitive-qty="${key}"]`);
    const parsed = parseQuantityInput(activeInput.value);
    const quantity = parsed ?? state.addPrimitiveQuantities[key] ?? addPrimitiveInitialQuantity;
    setAddPrimitiveQuantity(container, key, quantity);
    onQuantityChange(key, quantity);
  }, 0);
}

function handleAddPrimitiveQuantityKeydown(event, container, onQuantityChange) {
  if (event.key !== "Enter") {
    return;
  }

  const input = event.target.closest("[data-add-primitive-qty]");
  if (!input) {
    return;
  }

  event.preventDefault();
  const key = input.dataset.addPrimitiveQty;
  const parsed = parseQuantityInput(input.value);
  const quantity = parsed ?? state.addPrimitiveQuantities[key] ?? 1;
  setAddPrimitiveQuantity(container, key, quantity);
  onQuantityChange(key, quantity);
  input.blur();
}

function gsnNodeTitle(node) {
  return state.showHierarchyCode ? `${node.code} ${node.name || ""}` : node.name || node.code;
}

function isGSNPdfNode(node) {
  return Boolean(node.documentRef || node.nodeType === "Документ");
}

async function openGSNPdf(code, title = "") {
  if (!code || !state.gsnSupplement) {
    return;
  }

  const params = new URLSearchParams({
    supplement: state.gsnSupplement,
    code,
  });

  try {
    const response = await fetch(`/api/gsn/document?${params.toString()}`, {
      credentials: "include",
    });

    if (!response.ok) {
      const isJSON = response.headers.get("content-type")?.includes("application/json");
      const payload = isJSON ? await response.json() : null;
      throw new Error(payload?.error || `HTTP ${response.status}`);
    }

    const blob = await response.blob();
    const pdfUrl = URL.createObjectURL(blob);
    const tabTitle = String(title || code).trim() || "PDF";
    const safeTitle = escapeHTML(tabTitle);
    const html = `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8" />
<title>${safeTitle}</title>
<style>html,body{margin:0;height:100%;background:#f3f5f9}iframe{display:block;border:0;width:100%;height:100%}</style>
</head>
<body>
<iframe src="${pdfUrl}" title="${safeTitle}"></iframe>
</body>
</html>`;
    const htmlUrl = URL.createObjectURL(new Blob([html], { type: "text/html;charset=utf-8" }));
    const tab = window.open(htmlUrl, "_blank");
    if (!tab) {
      URL.revokeObjectURL(htmlUrl);
      URL.revokeObjectURL(pdfUrl);
      showMessage("Разрешите открытие новых вкладок в браузере", "error");
      return;
    }
    window.setTimeout(() => {
      URL.revokeObjectURL(htmlUrl);
      URL.revokeObjectURL(pdfUrl);
    }, 60_000);
  } catch (error) {
    showMessage(error.message, "error");
  }
}

function editorItemTitle(item) {
  return state.showHierarchyCode ? `${item.code} ${item.name || ""}` : item.name || item.code;
}

function encodeNodeAction(node) {
  return encodeURIComponent(
    JSON.stringify({
      code: node.code,
      originalCode: node.originalCode || "",
      name: node.name || "",
      unit: node.unit || "",
    }),
  );
}

function decodeNodeAction(value) {
  return JSON.parse(decodeURIComponent(value));
}

function openConstructionDialog(kind, parentId = "", editId = "") {
  const createTitles = {
    construction: "Добавить стройку",
    object: "Добавить объект",
    estimate: "Добавить смету",
  };
  const editTitles = {
    construction: "Редактировать стройку",
    object: "Редактировать объект",
    estimate: "Редактировать смету",
  };

  const entity = editId ? findConstructionEntity(kind, editId) : null;
  const isEdit = Boolean(entity);

  els.constructionDialogTitle.textContent = isEdit ? editTitles[kind] : createTitles[kind] || "Добавить";
  els.constructionDialogForm.kind.value = kind;
  els.constructionDialogForm.id.value = editId;
  els.constructionDialogForm.parentId.value = isEdit
    ? kind === "object"
      ? entity.constructionId
      : kind === "estimate"
        ? entity.objectId
        : ""
    : parentId;
  els.constructionDialogForm.code.value = isEdit ? entity.code || "" : "";
  els.constructionDialogForm.name.value = isEdit ? (kind === "estimate" ? entity.title : entity.name) : "";
  els.constructionDialog.showModal();
  els.constructionDialogForm.code.focus();
}

function findConstructionEntity(kind, id) {
  if (kind === "construction") {
    return state.constructions.find((item) => item.id === id);
  }
  if (kind === "object") {
    return state.objects.find((item) => item.id === id);
  }
  if (kind === "estimate") {
    return state.estimates.find((item) => item.id === id);
  }
  return null;
}

function constructionEntityDeleteMessage(kind, entity) {
  const label = kind === "estimate" ? entity.title : entity.name;
  const kindLabels = {
    construction: "стройку",
    object: "объект",
    estimate: "смету",
  };
  let message = `Удалить ${kindLabels[kind] || "элемент"} «${label}»?`;
  if (kind === "construction") {
    message += " Все объекты и сметы также будут удалены.";
  } else if (kind === "object") {
    message += " Все сметы также будут удалены.";
  }
  return message;
}

function renderConstructionEntityIconActions(kind, id) {
  return `
    <button
      class="icon-button secondary"
      data-edit="${escapeHTML(kind)}"
      data-edit-id="${escapeHTML(id)}"
      type="button"
      title="Изменить"
      aria-label="Изменить"
    >${iconPencil()}</button>
    <button
      class="icon-button secondary icon-button-danger"
      data-delete="${escapeHTML(kind)}"
      data-delete-id="${escapeHTML(id)}"
      type="button"
      title="Удалить"
      aria-label="Удалить"
    >${iconTrash()}</button>
  `;
}

function renderEstimateTreeActions(id) {
  return `
    <button
      class="secondary construction-add-btn"
      data-edit="estimate"
      data-edit-id="${escapeHTML(id)}"
      type="button"
    >Редактировать</button>
    <button
      class="icon-button secondary icon-button-danger"
      data-delete="estimate"
      data-delete-id="${escapeHTML(id)}"
      type="button"
      title="Удалить"
      aria-label="Удалить"
    >${iconTrash()}</button>
  `;
}

function cleanupConstructionTreeState(kind, id) {
  if (kind === "construction") {
    delete state.constructionExpanded[id];
    state.objects
      .filter((object) => object.constructionId === id)
      .forEach((object) => {
        delete state.objectExpanded[object.id];
      });
    return;
  }
  if (kind === "object") {
    delete state.objectExpanded[id];
  }
}

async function removeEstimatePermanently(estimateId) {
  const isOpen = state.openEstimates.some((item) => item.id === estimateId);
  if (isOpen) {
    try {
      await syncEstimateTextDraftIfNeeded(estimateId);
    } catch (error) {
      showMessage(error.message || "Не удалось разобрать текст сметы", "error");
      throw error;
    }
    await flushPersistOpenEstimate(estimateId);
  }

  await api(`/api/estimates/${estimateId}`, { method: "DELETE" });

  const index = state.openEstimates.findIndex((item) => item.id === estimateId);
  if (index >= 0) {
    state.openEstimates.splice(index, 1);
    invalidateEstimateTextDraft(estimateId);
    await releaseEstimateLock(estimateId);
    if (state.editorTab === estimateId) {
      state.editorTab =
        state.openEstimates.length > 0
          ? state.openEstimates[Math.min(index, state.openEstimates.length - 1)].id
          : "buffer";
    }
    state.gsnLoaded = false;
    saveEditorState();
    renderEditor();
  }

  delete state.estimateLocks[estimateId];
  await refreshConstructionData();
}

function pruneOpenEstimates() {
  const validIds = new Set(state.estimates.map((item) => item.id));
  let changed = false;
  for (let index = state.openEstimates.length - 1; index >= 0; index -= 1) {
    const estimate = state.openEstimates[index];
    if (validIds.has(estimate.id)) {
      continue;
    }
    state.openEstimates.splice(index, 1);
    invalidateEstimateTextDraft(estimate.id);
    delete state.estimateLocks[estimate.id];
    if (state.editorTab === estimate.id) {
      state.editorTab = "buffer";
    }
    changed = true;
  }
  if (!changed) {
    return;
  }
  state.gsnLoaded = false;
  saveEditorState();
  renderEditor();
}

async function deleteConstructionEntity(kind, id) {
  const entity = findConstructionEntity(kind, id);
  if (!entity) {
    showMessage("Элемент не найден", "error");
    return;
  }

  if (kind === "estimate" && isEstimateLockedByOther(id)) {
    const lock = getEstimateLockInfo(id);
    showMessage(`Смета редактируется ${lock?.userName || "другим пользователем"}`, "error");
    return;
  }

  if (!window.confirm(constructionEntityDeleteMessage(kind, entity))) {
    return;
  }

  try {
    if (kind === "estimate") {
      await removeEstimatePermanently(id);
      showMessage("Смета удалена", "ok");
      return;
    }

    if (kind === "construction") {
      await api(`/api/constructions/${id}`, { method: "DELETE" });
    } else if (kind === "object") {
      await api(`/api/objects/${id}`, { method: "DELETE" });
    } else {
      throw new Error("Неизвестный тип узла");
    }

    cleanupConstructionTreeState(kind, id);
    await refreshConstructionData();
    pruneOpenEstimates();
    showMessage("Удалено", "ok");
  } catch (error) {
    showMessage(error.message, "error");
  }
}

async function deleteEstimateFromEditor(estimateId) {
  const estimate =
    state.openEstimates.find((item) => item.id === estimateId) ||
    state.estimates.find((item) => item.id === estimateId);
  if (!estimate) {
    showMessage("Смета не найдена", "error");
    return;
  }

  if (!window.confirm(constructionEntityDeleteMessage("estimate", estimate))) {
    return;
  }

  try {
    await removeEstimatePermanently(estimateId);
    showMessage("Смета удалена", "ok");
  } catch (error) {
    if (error.message) {
      showMessage(error.message, "error");
    }
  }
}

async function saveConstructionDialog(data) {
  const code = data.code.trim();
  const name = data.name.trim();
  const id = (data.id || "").trim();
  if (!code || !name) {
    throw new Error("Укажите шифр и наименование");
  }

  if (id) {
    if (data.kind === "construction") {
      await api(`/api/constructions/${id}`, {
        method: "PUT",
        body: { code, name },
      });
      return;
    }

    if (data.kind === "object") {
      await api(`/api/objects/${id}`, {
        method: "PUT",
        body: { code, name },
      });
      return;
    }

    if (data.kind === "estimate") {
      await api(`/api/estimates/${id}`, {
        method: "PUT",
        body: { objectId: data.parentId, code, title: name },
      });
      return;
    }

    throw new Error("Неизвестный тип узла");
  }

  if (data.kind === "construction") {
    await api("/api/constructions", {
      method: "POST",
      body: { code, name },
    });
    return;
  }

  if (data.kind === "object") {
    await api("/api/objects", {
      method: "POST",
      body: { constructionId: data.parentId, code, name },
    });
    return;
  }

  if (data.kind === "estimate") {
    await api("/api/estimates", {
      method: "POST",
      body: { objectId: data.parentId, code, title: name },
    });
    return;
  }

  throw new Error("Неизвестный тип узла");
}

function compareByCode(left, right) {
  return String(left.code || "").localeCompare(String(right.code || ""), "ru", { numeric: true });
}

function isConstructionExpanded(id) {
  return Boolean(state.constructionExpanded[id]);
}

function isObjectExpanded(id) {
  return Boolean(state.objectExpanded[id]);
}

function renderConstructionTree() {
  const constructions = [...state.constructions].sort(compareByCode);
  const rows = [
    `
      <tr class="construction-toolbar">
        <td colspan="4">
          <button class="secondary construction-add-btn" data-add="construction" type="button">
            + Добавить стройку
          </button>
        </td>
      </tr>
    `,
  ];

  constructions.forEach((construction) => {
    const objects = state.objects
      .filter((object) => object.constructionId === construction.id)
      .sort(compareByCode);
    const constructionOpen = isConstructionExpanded(construction.id);

    rows.push(renderConstructionRow(construction, objects.length > 0, constructionOpen));

    objects.forEach((object) => {
      const estimates = state.estimates
        .filter((estimate) => estimate.objectId === object.id)
        .sort(compareByCode);
      const objectOpen = isObjectExpanded(object.id);

      rows.push(renderObjectRow(object, estimates.length > 0, objectOpen, constructionOpen));

      estimates.forEach((estimate) => {
        rows.push(renderEstimateRow(estimate, constructionOpen && objectOpen));
      });
    });
  });

  els.constructionTree.innerHTML = `
    <table class="construction-table">
      <tbody>${rows.join("")}</tbody>
    </table>
  `;
}

function renderConstructionRow(construction, hasChildren, expanded) {
  const toggle = hasChildren
    ? `<button
        class="tree-toggle secondary"
        data-toggle-construction="${escapeHTML(construction.id)}"
        type="button"
      >${expanded ? "-" : "+"}</button>`
    : `<span class="tree-toggle-placeholder"></span>`;

  return `
    <tr class="construction-table-row">
      <td class="construction-level-cell">
        <div class="construction-level-inner">${toggle}<span>Стройка</span></div>
      </td>
      <td class="construction-code-cell">${escapeHTML(construction.code || "")}</td>
      <td>${escapeHTML(construction.name)}</td>
      <td class="construction-actions-cell">
        <div class="construction-actions">
          <button
            class="secondary construction-add-btn"
            data-add="object"
            data-parent-id="${escapeHTML(construction.id)}"
            type="button"
          >+ Добавить объект</button>
          <div class="construction-icon-actions">
            ${renderConstructionEntityIconActions("construction", construction.id)}
          </div>
        </div>
      </td>
    </tr>
  `;
}

function renderObjectRow(object, hasChildren, expanded, constructionOpen) {
  const toggle = hasChildren
    ? `<button
        class="tree-toggle secondary"
        data-toggle-object="${escapeHTML(object.id)}"
        type="button"
      >${expanded ? "-" : "+"}</button>`
    : `<span class="tree-toggle-placeholder"></span>`;

  return `
    <tr class="construction-table-row ${constructionOpen ? "" : "hidden"}">
      <td class="construction-level-cell indent-1">
        <div class="construction-level-inner">${toggle}<span>Объект</span></div>
      </td>
      <td class="construction-code-cell">${escapeHTML(object.code || "")}</td>
      <td>${escapeHTML(object.name)}</td>
      <td class="construction-actions-cell">
        <div class="construction-actions">
          <button
            class="secondary construction-add-btn"
            data-add="estimate"
            data-parent-id="${escapeHTML(object.id)}"
            type="button"
          >+ Добавить смету</button>
          <div class="construction-icon-actions">
            ${renderConstructionEntityIconActions("object", object.id)}
          </div>
        </div>
      </td>
    </tr>
  `;
}

function renderEstimateRow(estimate, visible) {
  const lock = getEstimateLockInfo(estimate.id);
  const lockHint = lock ? `Редактируется ${lock.userName}` : "";
  const actionControl = lock
    ? `<span
          class="construction-lock-indicator icon-button secondary"
          title="${escapeHTML(lockHint)}"
          aria-label="${escapeHTML(lockHint)}"
        >${iconLock()}</span>`
    : `<div class="construction-actions construction-estimate-actions">${renderEstimateTreeActions(estimate.id)}</div>`;

  return `
    <tr class="construction-table-row ${visible ? "" : "hidden"}">
      <td class="construction-level-cell indent-2">
        <div class="construction-level-inner"><span class="tree-toggle-placeholder"></span><span>Смета</span></div>
      </td>
      <td class="construction-code-cell">${escapeHTML(estimate.code || "")}</td>
      <td>${escapeHTML(estimate.title)}</td>
      <td class="construction-actions-cell">
        ${actionControl}
      </td>
    </tr>
  `;
}

function readSessionJSON(key, fallback) {
  if (Array.isArray(fallback)) {
    return readSessionJSONArray(key, fallback);
  }
  if (fallback && typeof fallback === "object") {
    return readSessionJSONObject(key, fallback);
  }
  try {
    const raw = sessionStorage.getItem(key);
    return raw ? JSON.parse(raw) : fallback;
  } catch {
    return fallback;
  }
}

async function api(path, options = {}) {
  const headers = {
    "Content-Type": "application/json",
    ...(options.headers || {}),
  };

  const response = await fetch(path, {
    method: options.method || "GET",
    headers,
    credentials: "include",
    body: options.body ? JSON.stringify(options.body) : undefined,
  });

  const isJSON = response.headers.get("content-type")?.includes("application/json");
  const payload = isJSON ? await response.json() : null;
  if (!response.ok) {
    throw new Error(payload?.error || `HTTP ${response.status}`);
  }
  return payload;
}

function showMessage(text, kind = "") {
  els.message.textContent = text;
  els.message.className = kind ? `message ${kind}` : "message";
  els.message.classList.remove("hidden");
  window.clearTimeout(showMessage.timer);
  showMessage.timer = window.setTimeout(() => {
    els.message.classList.add("hidden");
  }, 3600);
}

function escapeHTML(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

function formatBreakableCode(code) {
  const text = String(code || "");
  if (!text) {
    return "";
  }

  const chunks = text.match(/.{1,3}/gu) || [];
  return chunks.map((chunk) => escapeHTML(chunk)).join("<wbr>");
}
