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
  showHierarchyCode: localStorage.getItem("nav_show_hierarchy_code") === "true",
  editorTab: sessionStorage.getItem("nav_editor_tab") || "buffer",
  buffer: readSessionJSON("nav_editor_buffer", []),
  openEstimates: readSessionJSON("nav_editor_estimates", []),
  constructionExpanded: {},
  objectExpanded: {},
};

const money = new Intl.NumberFormat("ru-RU", {
  style: "currency",
  currency: "RUB",
  maximumFractionDigits: 0,
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
  documentsSection: document.querySelector("#documentsSection"),
  settingsSection: document.querySelector("#settingsSection"),
  editorSubitems: document.querySelector("#editorSubitems"),
  editorTitle: document.querySelector("#editorTitle"),
  editorDescription: document.querySelector("#editorDescription"),
  editorContent: document.querySelector("#editorContent"),
  estimateLineDialog: document.querySelector("#estimateLineDialog"),
  estimateLineDialogForm: document.querySelector("#estimateLineDialogForm"),
  baseTitle: document.querySelector("#baseTitle"),
  baseDescription: document.querySelector("#baseDescription"),
  gsnSupplementBar: document.querySelector("#gsnSupplementBar"),
  gsnSupplementSelect: document.querySelector("#gsnSupplementSelect"),
  gsnTree: document.querySelector("#gsnTree"),
  showHierarchyCodeToggle: document.querySelector("#showHierarchyCodeToggle"),
  constructionTree: document.querySelector("#constructionTree"),
  constructionDialog: document.querySelector("#constructionDialog"),
  constructionDialogForm: document.querySelector("#constructionDialogForm"),
  constructionDialogTitle: document.querySelector("#constructionDialogTitle"),
  message: document.querySelector("#message"),
};

els.loginForm.addEventListener("submit", async (event) => {
  event.preventDefault();

  try {
    const result = await api("/api/auth/login", {
      method: "POST",
      body: formData(els.loginForm),
      skipAuth: true,
    });
    if (!result.access?.app) {
      throw new Error("Доступ к системе не открыт. Ожидайте подтверждения или войдите через /admin");
    }

    state.token = result.token;
    state.me = result.user;
    localStorage.setItem("nav_token", state.token);
    await loadApp();
    showMessage("Вход выполнен", "ok");
  } catch (error) {
    showMessage(error.message, "error");
  }
});

els.registerForm.addEventListener("submit", async (event) => {
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

els.showRegisterButton.addEventListener("click", () => showAuthScreen("register"));
els.showLoginButton.addEventListener("click", () => showAuthScreen("login"));

els.logoutButton.addEventListener("click", () => {
  state.token = "";
  state.me = null;
  localStorage.removeItem("nav_token");
  renderShell();
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
  loadGSNRoot({ force: true });
  renderEditor();
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
  loadGSNBaseInfo();
  loadGSNRoot({ force: true });
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
    openEstimateLineDialog(addLineButton.dataset.editorAddLine);
  }
});

els.editorContent.addEventListener("input", (event) => {
  const input = event.target.closest("[data-editor-district]");
  if (!input) {
    return;
  }
  const estimate = state.openEstimates.find((item) => item.id === input.dataset.editorDistrict);
  if (!estimate) {
    return;
  }
  estimate.district = input.value;
  saveEditorState();
});

els.estimateLineDialogForm.addEventListener("submit", (event) => {
  event.preventDefault();
  const data = formData(els.estimateLineDialogForm);
  addEstimateLine(data.estimateId, data.lineType, data.name);
  els.estimateLineDialog.close();
  els.estimateLineDialogForm.reset();
});

els.estimateLineDialogForm.querySelector("[data-dialog-cancel]").addEventListener("click", () => {
  els.estimateLineDialog.close();
  els.estimateLineDialogForm.reset();
});

async function loadApp() {
  if (!state.token) {
    renderShell();
    return;
  }

  try {
    const me = await api("/api/me");
    state.me = me.user;
    if (!me.access?.app && !me.user?.authorized) {
      throw new Error("Нет доступа");
    }

    await Promise.all([refreshCompanies(), refreshConstructionData()]);
    renderShell();
  } catch {
    state.token = "";
    state.me = null;
    localStorage.removeItem("nav_token");
    renderShell();
    showMessage("Сессия истекла, войдите снова", "error");
  }
}

async function refreshCompanies() {
  state.companies = await api("/api/companies");
}

async function refreshConstructionData() {
  const [constructions, objects, estimates] = await Promise.all([
    api("/api/constructions"),
    api("/api/objects"),
    api("/api/estimates"),
  ]);

  state.constructions = constructions;
  state.objects = objects;
  state.estimates = estimates;
  renderConstructionTree();
}

function showAuthScreen(screen) {
  state.authScreen = screen;
  els.loginView.classList.toggle("hidden", screen !== "login");
  els.registerView.classList.toggle("hidden", screen !== "register");
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
    documents: els.documentsSection,
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

  if (state.section === "base" && state.baseTab === "gsn") {
    els.baseDescription.classList.add("hidden");
    renderGSNBaseInfo();
    els.gsnTree.classList.remove("hidden");
    els.gsnSupplementBar.classList.remove("hidden");
    loadGSNPanel();
  } else if (state.section === "base") {
    els.baseTitle.textContent = "Позиции пользователя";
    els.baseDescription.textContent =
      "Здесь будут пользовательские позиции, которые пользователь сможет применять в сметах.";
    els.baseDescription.classList.remove("hidden");
    els.gsnTree.classList.add("hidden");
    els.gsnSupplementBar.classList.add("hidden");
  }

  if (state.section === "editor") {
    renderEditor();
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

async function loadGSNRoot(options = {}) {
  if (!state.token || state.baseTab !== "gsn" || !state.gsnSupplement) {
    return;
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

function renderGSNNode(node) {
  const toggle = node.hasChildren
    ? `<button class="tree-toggle" data-gsn-toggle="${escapeHTML(node.code)}" type="button">+</button>`
    : `<span class="tree-toggle-placeholder"></span>`;
  const encodedNode = encodeNodeAction(node);
  const actions = !node.hasChildren
    ? `
      <div class="node-actions">
        <button class="micro-button secondary" data-gsn-buffer="${encodedNode}" type="button">В буфер</button>
        ${
          state.openEstimates.length === 1
            ? `<button class="micro-button" data-gsn-estimate="${encodedNode}" type="button">В смету</button>`
            : ""
        }
      </div>
    `
    : "";

  return `
    <article class="tree-node gsn-node">
      <header>
        <div class="tree-title">
          ${toggle}
          <div>
            <strong>${escapeHTML(gsnNodeTitle(node))}</strong>
            ${actions}
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
    els.editorTitle.textContent = "Редактор";
    els.editorDescription.textContent = "";
    els.editorDescription.classList.add("hidden");
    els.editorContent.innerHTML = `<p class="muted">Выберите смету для редактирования.</p>`;
    return;
  }
  els.editorTitle.textContent = editorEstimateLabel(estimate);
  els.editorDescription.classList.add("hidden");
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
            </article>
          `,
        )
        .join("")}
    </div>
  `;
}

function renderEstimateEditor(estimate) {
  const rows = (estimate.items || []).map(
    (item, index) => `
      <tr>
        <td>${index + 1}</td>
        <td>${escapeHTML(item.code || "")}</td>
        <td>${escapeHTML(item.name || "")}</td>
        <td>${escapeHTML(item.unit || "")}</td>
        <td>${formatNumber(item.quantity)}</td>
        <td>${money.format(Number(item.unitPrice || 0))}</td>
        <td>${money.format(Number(item.total || Number(item.quantity || 0) * Number(item.unitPrice || 0)))}</td>
      </tr>
    `,
  );

  return `
    <div class="editor-estimate-meta">
      <span class="editor-meta-inline">
        <span class="muted">Стройка:</span>
        <strong>${escapeHTML(estimate.constructionLabel || "-")}</strong>
      </span>
      <span class="editor-meta-inline">
        <span class="muted">Объект:</span>
        <strong>${escapeHTML(estimate.objectLabel || "-")}</strong>
      </span>
      <label class="editor-meta-inline editor-district-inline">
        <span class="muted">Сметный район</span>
        <input data-editor-district="${escapeHTML(estimate.id)}" value="${escapeHTML(estimate.district || "")}" />
      </label>
      <button
        class="secondary construction-add-btn"
        data-editor-add-line="${escapeHTML(estimate.id)}"
        type="button"
      >+ Добавить строку сметы</button>
    </div>
    <div class="table-wrap">
      <table class="editor-estimate-table">
        <thead>
          <tr>
            <th>№ п/п</th>
            <th>Шифр</th>
            <th>Наименование</th>
            <th>Ед. изм.</th>
            <th>Объем</th>
            <th>Стоимость ед.</th>
            <th>Стоимость на объем</th>
          </tr>
        </thead>
        <tbody>
          ${rows.length ? rows.join("") : `<tr><td colspan="7" class="muted">Строки сметы отсутствуют</td></tr>`}
        </tbody>
      </table>
    </div>
  `;
}

function createEditorEstimate() {
  const estimate = {
    id: `session_est_${Date.now()}`,
    title: `Смета ${state.openEstimates.length + 1}`,
    code: "",
    district: "",
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

function addNodeToOnlyEstimate(node) {
  if (state.openEstimates.length !== 1) {
    return;
  }

  state.openEstimates[0].items.push(editorItemFromNode(node));
  saveEditorState();
  renderEditor();
  showMessage("Позиция добавлена в смету", "ok");
}

function editorItemFromNode(node) {
  return {
    code: node.code,
    name: node.name || "",
    unit: node.unit || "",
  };
}

function saveEditorState() {
  sessionStorage.setItem("nav_editor_buffer", JSON.stringify(state.buffer));
  sessionStorage.setItem("nav_editor_estimates", JSON.stringify(state.openEstimates));
  sessionStorage.setItem("nav_editor_tab", state.editorTab);
}

function editorEstimateLabel(estimate) {
  const code = String(estimate.code || "").trim();
  const title = String(estimate.title || "").trim();
  if (code && title) {
    return `${code} ${title}`;
  }
  return title || code || "Смета";
}

function openEstimateLineDialog(estimateID) {
  els.estimateLineDialogForm.elements.estimateId.value = estimateID;
  els.estimateLineDialogForm.elements.lineType.value = "position";
  els.estimateLineDialogForm.elements.name.value = "";
  els.estimateLineDialog.showModal();
  els.estimateLineDialogForm.elements.name.focus();
}

function addEstimateLine(estimateID, lineType, name) {
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
  renderEditor();
  showMessage("Строка добавлена", "ok");
}

function openEstimateInEditor(estimateID) {
  const sourceEstimate = state.estimates.find((item) => item.id === estimateID);
  if (!sourceEstimate) {
    showMessage("Смета не найдена", "error");
    return;
  }

  const object = state.objects.find((item) => item.id === sourceEstimate.objectId);
  const construction = object
    ? state.constructions.find((item) => item.id === object.constructionId)
    : null;

  const editorEstimate = {
    id: sourceEstimate.id,
    code: sourceEstimate.code || "",
    title: sourceEstimate.title || "",
    district: sourceEstimate.district || "",
    constructionLabel: construction ? `${construction.code || ""} ${construction.name || ""}`.trim() : "",
    objectLabel: object ? `${object.code || ""} ${object.name || ""}`.trim() : "",
    items: (sourceEstimate.items || []).map((item) => ({
      id: item.id,
      type: item.type || "position",
      code: item.code || "",
      name: item.name || "",
      unit: item.unit || "",
      quantity: Number(item.quantity || 0),
      unitPrice: Number(item.unitPrice || 0),
      total: Number(item.total || Number(item.quantity || 0) * Number(item.unitPrice || 0)),
    })),
  };

  const existingIndex = state.openEstimates.findIndex((item) => item.id === editorEstimate.id);
  if (existingIndex >= 0) {
    state.openEstimates[existingIndex] = editorEstimate;
  } else {
    state.openEstimates.push(editorEstimate);
  }

  state.editorTab = editorEstimate.id;
  state.section = "editor";
  saveEditorState();
  renderSections();
  showMessage("Смета открыта в редакторе", "ok");
}

function formatNumber(value) {
  const number = Number(value || 0);
  return Number.isFinite(number) ? new Intl.NumberFormat("ru-RU").format(number) : "0";
}

function gsnNodeTitle(node) {
  return state.showHierarchyCode ? `${node.code} ${node.name || ""}` : node.name || node.code;
}

function editorItemTitle(item) {
  return state.showHierarchyCode ? `${item.code} ${item.name || ""}` : item.name || item.code;
}

function encodeNodeAction(node) {
  return encodeURIComponent(
    JSON.stringify({
      code: node.code,
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
  return `
    <tr class="construction-table-row ${visible ? "" : "hidden"}">
      <td class="construction-level-cell indent-2">
        <div class="construction-level-inner"><span class="tree-toggle-placeholder"></span><span>Смета</span></div>
      </td>
      <td class="construction-code-cell">${escapeHTML(estimate.code || "")}</td>
      <td>${escapeHTML(estimate.title)}</td>
      <td class="construction-actions-cell">
        <button
          class="secondary construction-add-btn"
          data-edit="estimate"
          data-edit-id="${escapeHTML(estimate.id)}"
          type="button"
        >Изменить</button>
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

function formData(form) {
  return Object.fromEntries(new FormData(form).entries());
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

loadApp();
