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
      password: String(parsed.password || ""),
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
      password: String(data.password || ""),
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
  const passwordInput = target.querySelector('[name="password"]');
  if (companyInput) {
    companyInput.value = draft.companyName;
  }
  if (nameInput) {
    nameInput.value = draft.name;
  }
  if (passwordInput) {
    passwordInput.value = draft.password;
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

const state = {
  token: localStorage.getItem("nav_token") || "",
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
  buffer: readSessionJSON("nav_editor_buffer", []),
  openEstimates: readSessionJSON("nav_editor_estimates", []),
  estimateLinesExpanded: readSessionJSON("nav_editor_expanded", {}),
  constructionExpanded: {},
  objectExpanded: {},
  estimateLocks: {},
  userPositions: [],
  addPrimitiveQuantities: {},
  addLineTargetEstimateId: null,
  licenseBlocked: [],
  licenseHeld: [],
};

let userPositionDialogEditId = "";
let userPositionDialogSaving = false;
const persistEstimateInflight = new Map();
const ESTIMATE_LOCK_POLL_MS = 5000;
const ESTIMATE_LOCK_HEARTBEAT_MS = 30000;
const LICENSE_SESSION_HEARTBEAT_MS = 30000;
let estimateLockPollTimer = 0;
let estimateLockHeartbeatTimer = 0;
let licenseSessionHeartbeatTimer = 0;
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
      skipAuth: true,
    });
    if (!result.access?.app) {
      throw new Error("Доступ к системе не открыт. Ожидайте подтверждения администратора");
    }

    state.token = result.token;
    state.me = result.user;
    localStorage.setItem("nav_token", state.token);
    await loadApp();
    showMessage("Вход выполнен", "ok");
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
      skipAuth: true,
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
  state.token = "";
  state.me = null;
  state.estimateLocks = {};
  localStorage.removeItem("nav_token");
  renderShell();
  applyLoginDraft(els.loginForm, LOGIN_DRAFT_KEY);
});

window.addEventListener("beforeunload", () => {
  if (!state.token) {
    return;
  }
  fetch("/api/license-sessions", {
    method: "DELETE",
    headers: { Authorization: `Bearer ${state.token}` },
    keepalive: true,
  });
  for (const estimate of state.openEstimates) {
    if (!isPersistedEstimateId(estimate.id)) {
      continue;
    }
    fetch(`/api/estimates/${estimate.id}/lock`, {
      method: "DELETE",
      headers: { Authorization: `Bearer ${state.token}` },
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
      openEstimateInEditor(editButton.dataset.editId);
      return;
    }
    openConstructionDialog(editButton.dataset.edit, "", editButton.dataset.editId);
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
  const bufferButton = event.target.closest("[data-gsn-buffer]");
  if (bufferButton) {
    addNodeToBuffer(decodeNodeAction(bufferButton.dataset.gsnBuffer));
    return;
  }

  const estimateButton = event.target.closest("[data-gsn-estimate]");
  if (estimateButton) {
    addNodeToOnlyEstimate(decodeNodeAction(estimateButton.dataset.gsnEstimate));
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

els.editorSubitems.addEventListener("click", (event) => {
  if (event.target.closest("[data-editor-create-estimate]")) {
    createEditorEstimate();
    return;
  }

  const button = event.target.closest("[data-editor-tab]");
  if (!button) {
    return;
  }

  state.editorTab = button.dataset.editorTab;
  sessionStorage.setItem("nav_editor_tab", state.editorTab);
  state.section = "editor";
  renderSections();
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

async function loadApp() {
  if (!state.token) {
    renderShell();
    applyLoginDraft(els.loginForm, LOGIN_DRAFT_KEY);
    return;
  }

  try {
    const me = await api("/api/me");
    state.me = me.user;
    if (!me.access?.app && !me.user?.authorized) {
      throw new Error("Нет доступа к системе. Ожидайте подтверждения администратора");
    }

    await Promise.all([refreshCompanies(), refreshConstructionData()]);
    await restoreOpenEstimateLocks();
    startEstimateLockSync();
    void loadGSNRegions().catch(() => {});
    void loadFGISSets().catch(() => {});
    loadUserPositions();
    renderShell();
  } catch (error) {
    stopEstimateLockSync();
    const message = error?.message || "Не удалось загрузить приложение";
    const unauthorized = /HTTP 401|HTTP 403|Нет доступа/i.test(message);
    if (unauthorized) {
      state.token = "";
      state.me = null;
      state.estimateLocks = {};
      localStorage.removeItem("nav_token");
    }
    renderShell();
    applyLoginDraft(els.loginForm, LOGIN_DRAFT_KEY);
    showMessage(unauthorized ? "Сессия истекла, войдите снова" : message, "error");
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
  const loggedIn = Boolean(state.token && state.me);
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

  if (state.section === "editor") {
    const ids = ["gsn_supplement_18"];
    if (state.editorTab !== "buffer") {
      const estimate = state.openEstimates.find((item) => item.id === state.editorTab);
      const fgisId = subsectionIdForFGISSet(estimate?.fgisSetId || "");
      if (fgisId && !ids.includes(fgisId)) {
        ids.push(fgisId);
      }
    }
    return ids;
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

function renderLicenseBlockMessage(section, blockedItems) {
  const target = licenseSectionElements(section);
  if (!target?.block || !blockedItems.length) {
    return;
  }
  const names = blockedItems.map((item) => item.name).join(", ");
  target.block.innerHTML = `<p class="license-block-message">Нет свободных лицензий для ${escapeHTML(names)}. Обратитесь к администратору системы.</p>`;
}

async function releaseLicenseSessions() {
  if (!state.token) {
    state.licenseBlocked = [];
    state.licenseHeld = [];
    return;
  }

  try {
    await fetch("/api/license-sessions", {
      method: "DELETE",
      headers: { Authorization: `Bearer ${state.token}` },
    });
  } catch {
    // ignore release errors on logout/navigation
  }
  state.licenseBlocked = [];
  state.licenseHeld = [];
}

async function syncLicenseSessionsForCurrentSection() {
  const subsectionIds = getRequiredLicenseSubsectionIds();
  if (!state.token) {
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
        Authorization: `Bearer ${state.token}`,
      },
      body: JSON.stringify({ subsectionIds }),
    });
    const payload = await response.json().catch(() => ({}));
    if (response.status === 403 && payload?.blocked?.length) {
      state.licenseBlocked = payload.blocked;
      return payload.blocked;
    }
    if (!response.ok) {
      throw new Error(payload?.error || `HTTP ${response.status}`);
    }
    state.licenseBlocked = [];
    state.licenseHeld = payload.held || [];
    return [];
  } catch {
    return [];
  }
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
  const needsLicense = state.section === "base" || state.section === "editor";

  if (!needsLicense) {
    stopLicenseSessionSync();
    await releaseLicenseSessions();
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
  const lock = state.estimateLocks[estimateId];
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
  if (!state.token) {
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
  const headers = {
    "Content-Type": "application/json",
    Authorization: `Bearer ${state.token}`,
  };
  const response = await fetch(`/api/estimates/${estimateId}/lock`, {
    method: "PUT",
    headers,
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
  if (!state.token || !isPersistedEstimateId(estimateId)) {
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
  if (!state.token) {
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
        <button class="nav-sublink ${state.editorTab === estimate.id ? "active" : ""}" data-editor-tab="${estimate.id}" type="button">
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
  if (!state.token || state.baseTab !== "gsn") {
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
  if (!state.token || state.gsnSupplementsLoaded) {
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
  if (!state.token || state.gsnBaseInfoLoaded || !state.gsnSupplement) {
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
  if (!state.token || state.baseTab !== "gsn" || !state.gsnSupplement || !state.gsnSearch) {
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
    const canAddToEstimate = Boolean(getAddLineTargetEstimate());
    const originalCode = gsnLeafOriginalCode(node);
    return `
      <article class="tree-node gsn-node gsn-node-leaf">
        <header>
          <div class="tree-title">
            ${toggle}
            <div class="gsn-leaf-row">
              <div class="gsn-leaf-cell gsn-leaf-code" title="${escapeHTML(originalCode)}">${escapeHTML(originalCode || "-")}</div>
              <div class="gsn-leaf-cell gsn-leaf-name" title="${escapeHTML(node.name || "")}">${escapeHTML(node.name || "")}</div>
              <div class="gsn-leaf-cell gsn-leaf-unit" title="${escapeHTML(node.unit || "")}">${escapeHTML(node.unit || "-")}</div>
              <div class="gsn-leaf-cell gsn-leaf-actions">
                <button class="micro-button secondary" data-gsn-buffer="${encodedNode}" type="button">В буфер</button>
                ${canAddToEstimate ? `<button class="micro-button" data-gsn-estimate="${encodedNode}" type="button">В смету</button>` : ""}
              </div>
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
  if (!state.token || state.baseTab !== "gsn" || !state.gsnSupplement) {
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
    const canAddToEstimate = Boolean(getAddLineTargetEstimate());
    const originalCode = gsnLeafOriginalCode(node);
    return `
      <article class="tree-node gsn-node gsn-node-leaf">
        <header>
          <div class="tree-title">
            ${toggle}
            <div class="gsn-leaf-row">
              <div class="gsn-leaf-cell gsn-leaf-code" title="${escapeHTML(originalCode)}">${escapeHTML(originalCode || "-")}</div>
              <div class="gsn-leaf-cell gsn-leaf-name" title="${escapeHTML(node.name || "")}">${escapeHTML(node.name || "")}</div>
              <div class="gsn-leaf-cell gsn-leaf-unit" title="${escapeHTML(node.unit || "")}">${escapeHTML(node.unit || "-")}</div>
              <div class="gsn-leaf-cell gsn-leaf-actions">
                <button class="micro-button secondary" data-gsn-buffer="${encodedNode}" type="button">В буфер</button>
                ${canAddToEstimate ? `<button class="micro-button" data-gsn-estimate="${encodedNode}" type="button">В смету</button>` : ""}
              </div>
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
  if (state.editorTab !== "buffer" && !state.openEstimates.some((estimate) => estimate.id === state.editorTab)) {
    state.editorTab = "buffer";
    sessionStorage.setItem("nav_editor_tab", state.editorTab);
  }

  renderEditorSubitems();
  if (state.editorTab === "buffer") {
    els.editorEyebrow.textContent = "Редактор";
    els.editorTitle.classList.remove("hidden");
    els.editorTitle.textContent = "Буфер";
    els.editorDescription.textContent = "Сессионный буфер для элементов, добавленных из базы ГСН-2022.";
    els.editorDescription.classList.remove("hidden");
    els.editorContent.innerHTML = state.buffer.length
      ? renderEditorItems(state.buffer, "Буфер пока пуст.")
      : `<p class="muted">Буфер пока пуст.</p>`;
    return;
  }

  const estimate = state.openEstimates.find((item) => item.id === state.editorTab);
  if (!estimate) {
    els.editorEyebrow.textContent = "Редактор";
    els.editorTitle.classList.remove("hidden");
    els.editorTitle.textContent = "Редактор";
    els.editorDescription.textContent = "";
    els.editorDescription.classList.add("hidden");
    els.editorContent.innerHTML = `<p class="muted">Выберите смету для редактирования.</p>`;
    return;
  }
  els.editorEyebrow.textContent = "Редактор сметы";
  els.editorTitle.classList.add("hidden");
  els.editorDescription.classList.add("hidden");
  void loadFGISSets()
    .then(() => {
      if (state.editorTab === estimate.id) {
        els.editorContent.innerHTML = renderEstimateEditor(estimate);
      }
    })
    .catch(() => {});
  els.editorContent.innerHTML = renderEstimateEditor(estimate);
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
  if (!state.token || !state.fgisSetId) {
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
  if (!state.token) {
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
  saveEditorState();
  renderEditor();
  if (options.persist !== false) {
    void persistOpenEstimate(estimateID).catch(() => {});
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
    try {
      await recalculateEstimatePricing(estimate);
      showMessage("Сметный район обновлён, стоимости пересчитаны", "ok");
    } catch (error) {
      showMessage(error.message || "Не удалось пересчитать стоимости", "error");
    }
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
  return (item?.children || []).reduce(
    (sum, child) => sum + estimateResourceLineTotal(child, item.quantity),
    0,
  );
}

function estimateLineTotal(item, parentItem = null) {
  if (item?.type === "resource" && parentItem) {
    return estimateResourceLineTotal(item, parentItem.quantity);
  }
  if (item?.children?.length && estimateLineIsWorkPosition(item)) {
    return estimatePositionTotalFromResources(item);
  }
  if (item?.type === "position" && estimateLineIsResourcePosition(item)) {
    return estimateResourceUnitPriceValue(item) * Number(item.quantity || 0);
  }
  return Number(item?.total ?? Number(item?.quantity || 0) * Number(item?.unitPrice || 0));
}

function estimateGrandTotal(items) {
  return (items || []).reduce((sum, item) => sum + estimateLineTotal(item), 0);
}

function formatEstimateMoney(value) {
  const number = Number(value || 0);
  if (!Number.isFinite(number) || number === 0) {
    return "";
  }
  return estimateCostDetailed.format(number);
}

function estimateLineDisplayCode(item) {
  return String(item?.originalCode || item?.code || "").trim();
}

function estimateRecordFetchCode(item) {
  return String(item?.recordCode || item?.code || "").trim();
}

function estimateRecordLookupCode(item) {
  return estimateRecordFetchCode(item) || estimateLineDisplayCode(item);
}

function applyRecordDetailToEstimateItem(item, record) {
  if (!item || !record) {
    return;
  }
  if (record.code) {
    item.recordCode = record.code;
  }
  if (record.originalCode) {
    item.originalCode = record.originalCode;
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
  if (!text) {
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

function renderEstimateTableRow(options) {
  const { estimateId, item, indexLabel, rowKind, hasResources, isExpanded, lineIndex, lineCount, parentItem } =
    options;
  const isChild = rowKind === "child";
  const isStructural = estimateLineIsStructural(item);
  const lineTotal = estimateLineTotal(item, parentItem);
  const quantityCell = isStructural ? "" : formatNumber(item.quantity);
  return `
    <tr class="${estimateTableRowClass(item, rowKind)}">
      <td class="editor-estimate-index-cell">${escapeHTML(indexLabel)}</td>
      ${renderEstimateCodeCell(estimateId, item, { rowKind, hasResources, isExpanded })}
      <td class="editor-estimate-name-cell${isChild ? " editor-estimate-name-cell-child" : ""}">${escapeHTML(item.name || "")}</td>
      <td class="editor-estimate-unit-cell">${escapeHTML(item.unit || "")}</td>
      <td class="editor-estimate-quantity-cell">${quantityCell}</td>
      <td class="editor-estimate-price-cell">${formatEstimateUnitPrice(item)}</td>
      <td class="editor-estimate-total-cell">${formatEstimateMoney(lineTotal)}</td>
      ${renderEstimateLineActions(estimateId, item, lineIndex, lineCount, {
        rowKind,
        hasResources,
        isExpanded,
      })}
    </tr>
  `;
}

function buildEstimateTableRows(estimate) {
  const items = estimate.items || [];
  const expanded = getExpandedLineIds(estimate.id);
  const rows = [];
  let positionNumber = 0;

  items.forEach((item, index) => {
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
  });

  return rows;
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

function renderEstimateEditor(estimate) {
  const items = estimate.items || [];
  const rows = buildEstimateTableRows(estimate);

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
        <span class="muted">Сметная стоимость</span>
        <output class="editor-estimate-total-value">${money.format(estimateGrandTotal(items))}</output>
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
    <div class="table-wrap">
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
        <tbody>
          ${rows.length ? rows.join("") : `<tr><td colspan="8" class="muted">Строки сметы отсутствуют</td></tr>`}
        </tbody>
      </table>
    </div>
    <div class="editor-estimate-actions">
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
      <button
        class="secondary construction-add-btn"
        data-editor-close-estimate="${escapeHTML(estimate.id)}"
        type="button"
      >Закрыть смету</button>
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
            quantity: 1,
            unitPrice: 0,
            total: 0,
          };

      estimate.items.push(item);
      if (item.isWork !== false && item.children?.length) {
        getExpandedLineIds(estimate.id).add(item.id);
        state.estimateLinesExpanded[estimate.id] = [...getExpandedLineIds(estimate.id)];
      }
    }

    state.buffer = [];
    saveEstimateLinesExpanded();
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

function buildEditorEstimateFromSource(sourceEstimate) {
  const object = state.objects.find((item) => item.id === sourceEstimate.objectId);
  const construction = object
    ? state.constructions.find((item) => item.id === object.constructionId)
    : null;

  return {
    id: sourceEstimate.id,
    objectId: sourceEstimate.objectId,
    code: sourceEstimate.code || "",
    title: sourceEstimate.title || "",
    description: sourceEstimate.description || "",
    status: sourceEstimate.status || "draft",
    district: sourceEstimate.district || "",
    fgisSetId: sourceEstimate.fgisSetId || "",
    constructionLabel: construction ? `${construction.code || ""} ${construction.name || ""}`.trim() : "",
    objectLabel: object ? `${object.code || ""} ${object.name || ""}`.trim() : "",
    items: (sourceEstimate.items || []).map((item) => ({
      id: item.id,
      type: item.type || "position",
      code: item.code || "",
      recordCode: item.code || "",
      originalCode: item.originalCode || "",
      name: item.name || "",
      unit: item.unit || "",
      quantity: Number(item.quantity || 0),
      unitPrice: Number(item.unitPrice || 0),
      total: Number(item.total || Number(item.quantity || 0) * Number(item.unitPrice || 0)),
      hasResources: Boolean(item.hasResources),
    })),
  };
}

function createEditorEstimate() {
  const estimate = {
    id: `session_est_${Date.now()}`,
    title: `Смета ${state.openEstimates.length + 1}`,
    code: "",
    district: "",
    fgisSetId: "",
    constructionLabel: "",
    objectLabel: "",
    items: [],
  };
  state.openEstimates.push(estimate);
  state.editorTab = estimate.id;
  state.gsnLoaded = false;
  saveEditorState();
  state.section = "editor";
  renderSections();
  showMessage("Новая смета открыта в редакторе", "ok");
}

function addNodeToBuffer(node) {
  state.buffer.push(editorItemFromNode(node));
  saveEditorState();
  renderEditor();
  showMessage("Позиция добавлена в буфер", "ok");
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
  const estimate = getAddLineTargetEstimate();
  if (!estimate) {
    return;
  }
  if (!estimate.items) {
    estimate.items = [];
  }

  void (async () => {
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

      estimate.items.push(item);
      if (item.isWork !== false && item.children?.length) {
        getExpandedLineIds(estimate.id).add(item.id);
        state.estimateLinesExpanded[estimate.id] = [...getExpandedLineIds(estimate.id)];
        saveEstimateLinesExpanded();
      }

      saveEditorState();
      await persistOpenEstimate(estimate.id);
      renderEditor();
      showMessage("Позиция добавлена в смету", "ok");
    } catch (error) {
      showMessage(error.message || "Не удалось добавить позицию", "error");
    }
  })();
}

function editorItemFromNode(node) {
  return {
    code: node.code,
    originalCode: node.originalCode || "",
    name: node.name || "",
    unit: node.unit || "",
  };
}

function saveEditorState() {
  sessionStorage.setItem("nav_editor_buffer", JSON.stringify(state.buffer));
  sessionStorage.setItem("nav_editor_estimates", JSON.stringify(state.openEstimates));
  sessionStorage.setItem("nav_editor_tab", state.editorTab);
  refreshUserPositionsPanelIfVisible();
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
  return (items || []).map((item) => ({
    id: item.id || "",
    type: item.type || "position",
    code: estimateRecordFetchCode(item) || item.code || "",
    originalCode: String(item.originalCode || "").trim(),
    name: item.name || "",
    quantity: Number(item.quantity || 0),
    unit: item.unit || "",
    unitPrice: Number(item.unitPrice || 0),
    total: Number(item.total ?? Number(item.quantity || 0) * Number(item.unitPrice || 0)),
  }));
}

async function persistOpenEstimate(estimateId) {
  if (!state.token || !isPersistedEstimateId(estimateId)) {
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

  const sourceEstimate = state.estimates.find((item) => item.id === estimateId);
  const objectId = editorEstimate.objectId || sourceEstimate?.objectId;
  const code = editorEstimate.code || sourceEstimate?.code;
  const title = editorEstimate.title || sourceEstimate?.title;
  if (!objectId || !code || !title) {
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
      }
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

  try {
    await recalculateEstimatePricing(estimate);
    saveEditorState();
    renderEditor();
    void applyLicenseGate();
    await persistOpenEstimate(estimateID);
    showMessage("Набор сметных цен обновлён, стоимости пересчитаны", "ok");
  } catch (error) {
    showMessage(error.message || "Не удалось пересчитать стоимости", "error");
    saveEditorState();
    renderEditor();
    try {
      await persistOpenEstimate(estimateID);
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

  await flushPersistOpenEstimate(estimateID);

  state.openEstimates.splice(index, 1);
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
  if (isEstimateLockedByOther(estimateID)) {
    const lock = getEstimateLockInfo(estimateID);
    showMessage(`Смета редактируется ${lock?.userName || "другим пользователем"}`, "error");
    return;
  }

  try {
    await acquireEstimateLock(estimateID);
  } catch (error) {
    if (error.status === 409 && error.lock) {
      state.estimateLocks[estimateID] = error.lock;
      renderConstructionTree();
      showMessage(`Смета редактируется ${error.lock.userName}`, "error");
      return;
    }
    showMessage(error.message, "error");
    return;
  }

  await flushPersistOpenEstimate(estimateID);

  try {
    const [constructions, objects, estimates] = await Promise.all([
      api("/api/constructions"),
      api("/api/objects"),
      api("/api/estimates"),
    ]);
    state.constructions = constructions;
    state.objects = objects;
    state.estimates = estimates;
  } catch (error) {
    showMessage(error.message, "error");
    return;
  }

  const sourceEstimate = state.estimates.find((item) => item.id === estimateID);
  if (!sourceEstimate) {
    showMessage("Смета не найдена", "error");
    return;
  }

  const existingEstimate = state.openEstimates.find((item) => item.id === estimateID);
  const editorEstimate = buildEditorEstimateFromSource(sourceEstimate);
  if (existingEstimate?.items?.length) {
    editorEstimate.items = editorEstimate.items.map((item) => {
      const previous = existingEstimate.items.find((line) => line.id === item.id);
      if (!previous) {
        return item;
      }
      return {
        ...item,
        recordCode: previous.recordCode || item.recordCode || item.code,
        originalCode: previous.originalCode || item.originalCode,
        children: previous.children,
        hasResources: previous.hasResources ?? item.hasResources,
      };
    });
  }
  if (existingEstimate?.district && !editorEstimate.district) {
    editorEstimate.district = existingEstimate.district;
    void persistOpenEstimate(estimateID).catch(() => {});
  }
  if (existingEstimate?.fgisSetId && !editorEstimate.fgisSetId) {
    editorEstimate.fgisSetId = existingEstimate.fgisSetId;
    void persistOpenEstimate(estimateID).catch(() => {});
  }
  if (existingEstimate?.code && !editorEstimate.code) {
    editorEstimate.code = existingEstimate.code;
  }
  if (existingEstimate?.title && !editorEstimate.title) {
    editorEstimate.title = existingEstimate.title;
  }

  try {
    await recalculateEstimatePricing(editorEstimate);
  } catch (error) {
    showMessage(error.message || "Не удалось загрузить данные ГСН для сметы", "error");
  }

  const existingIndex = state.openEstimates.findIndex((item) => item.id === estimateID);
  if (existingIndex >= 0) {
    state.openEstimates[existingIndex] = editorEstimate;
  } else {
    state.openEstimates.push(editorEstimate);
  }

  state.editorTab = estimateID;
  state.section = "editor";
  saveEditorState();
  renderConstructionTree();
  renderSections();
  showMessage("Смета открыта в редакторе", "ok");
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
      headers: {
        Authorization: `Bearer ${state.token}`,
      },
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
          <button
            class="secondary construction-add-btn"
            data-edit="construction"
            data-edit-id="${escapeHTML(construction.id)}"
            type="button"
          >Изменить</button>
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
          <button
            class="secondary construction-add-btn"
            data-edit="object"
            data-edit-id="${escapeHTML(object.id)}"
            type="button"
          >Изменить</button>
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
    : `<button
          class="secondary construction-add-btn"
          data-edit="estimate"
          data-edit-id="${escapeHTML(estimate.id)}"
          type="button"
        >Изменить</button>`;

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

  if (!options.skipAuth && state.token) {
    headers.Authorization = `Bearer ${state.token}`;
  }

  const response = await fetch(path, {
    method: options.method || "GET",
    headers,
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
  els.message.className = `message ${kind}`;
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
