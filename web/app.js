const state = {
  token: localStorage.getItem("nav_token") || "",
  me: null,
  section: "base",
  baseTab: "gsn",
  companies: [],
  constructions: [],
  objects: [],
  estimates: [],
  gsnLoaded: false,
  gsnBaseInfoLoaded: false,
  gsnBaseInfo: null,
  showHierarchyCode: localStorage.getItem("nav_show_hierarchy_code") === "true",
  editorTab: sessionStorage.getItem("nav_editor_tab") || "buffer",
  buffer: readSessionJSON("nav_editor_buffer", []),
  openEstimates: readSessionJSON("nav_editor_estimates", []),
};

const defaultItems = [
  { name: "Бетонные работы", quantity: 1, unit: "м3", unitPrice: 1500 },
  { name: "Работа специалистов", quantity: 8, unit: "ч", unitPrice: 900 },
];

const money = new Intl.NumberFormat("ru-RU", {
  style: "currency",
  currency: "RUB",
  maximumFractionDigits: 0,
});

const els = {
  loginView: document.querySelector("#loginView"),
  appView: document.querySelector("#appView"),
  logoutButton: document.querySelector("#logoutButton"),
  loginForm: document.querySelector("#loginForm"),
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
  createEditorEstimateButton: document.querySelector("#createEditorEstimateButton"),
  editorContent: document.querySelector("#editorContent"),
  baseTitle: document.querySelector("#baseTitle"),
  baseDescription: document.querySelector("#baseDescription"),
  gsnTree: document.querySelector("#gsnTree"),
  showHierarchyCodeToggle: document.querySelector("#showHierarchyCodeToggle"),
  constructionForm: document.querySelector("#constructionForm"),
  objectForm: document.querySelector("#objectForm"),
  estimateForm: document.querySelector("#estimateForm"),
  resetEstimateForm: document.querySelector("#resetEstimateForm"),
  constructionTree: document.querySelector("#constructionTree"),
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
    state.token = result.token;
    localStorage.setItem("nav_token", state.token);
    await loadApp();
    showMessage("Вход выполнен", "ok");
  } catch (error) {
    showMessage(error.message, "error");
  }
});

document.querySelectorAll("[data-demo]").forEach((button) => {
  button.addEventListener("click", () => {
    const [email, password] = button.dataset.demo.split("|");
    els.loginForm.email.value = email;
    els.loginForm.password.value = password;
  });
});

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

els.constructionForm.addEventListener("submit", async (event) => {
  event.preventDefault();

  try {
    await api("/api/constructions", {
      method: "POST",
      body: formData(els.constructionForm),
    });
    els.constructionForm.reset();
    await refreshConstructionData();
    showMessage("Стройка создана", "ok");
  } catch (error) {
    showMessage(error.message, "error");
  }
});

els.objectForm.addEventListener("submit", async (event) => {
  event.preventDefault();

  try {
    await api("/api/objects", {
      method: "POST",
      body: formData(els.objectForm),
    });
    els.objectForm.reset();
    await refreshConstructionData();
    showMessage("Объект создан", "ok");
  } catch (error) {
    showMessage(error.message, "error");
  }
});

els.estimateForm.addEventListener("submit", async (event) => {
  event.preventDefault();

  const data = formData(els.estimateForm);
  try {
    data.items = JSON.parse(data.items);
  } catch {
    showMessage("Позиции должны быть корректным JSON-массивом", "error");
    return;
  }

  const id = data.id;
  delete data.id;

  try {
    await api(id ? `/api/estimates/${id}` : "/api/estimates", {
      method: id ? "PUT" : "POST",
      body: data,
    });
    resetEstimateForm();
    await refreshConstructionData();
    showMessage("Смета сохранена", "ok");
  } catch (error) {
    showMessage(error.message, "error");
  }
});

els.resetEstimateForm.addEventListener("click", resetEstimateForm);

els.showHierarchyCodeToggle.checked = state.showHierarchyCode;
els.showHierarchyCodeToggle.addEventListener("change", () => {
  state.showHierarchyCode = els.showHierarchyCodeToggle.checked;
  localStorage.setItem("nav_show_hierarchy_code", String(state.showHierarchyCode));
  loadGSNRoot({ force: true });
  renderEditor();
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
  const button = event.target.closest("[data-editor-tab]");
  if (!button) {
    return;
  }

  state.editorTab = button.dataset.editorTab;
  sessionStorage.setItem("nav_editor_tab", state.editorTab);
  state.section = "editor";
  renderSections();
});

els.createEditorEstimateButton.addEventListener("click", () => {
  createEditorEstimate();
});

async function loadApp() {
  if (!state.token) {
    renderShell();
    return;
  }

  try {
    const me = await api("/api/me");
    state.me = me.user;
    await Promise.all([refreshCompanies(), refreshConstructionData()]);
    renderShell();
  } catch {
    state.token = "";
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
  renderConstructionForms();
  renderConstructionTree();
}

function renderShell() {
  const loggedIn = Boolean(state.token && state.me);
  els.loginView.classList.toggle("hidden", loggedIn);
  els.appView.classList.toggle("hidden", !loggedIn);
  els.logoutButton.classList.toggle("hidden", !loggedIn);

  if (!loggedIn) {
    return;
  }

  els.currentUser.textContent = `${state.me.name} (${state.me.email})`;
  els.currentRole.textContent = state.me.role;
  renderSections();
  renderConstructionForms();
  renderConstructionTree();
  resetEstimateForm();
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
    renderGSNBaseInfo();
    els.gsnTree.classList.remove("hidden");
    loadGSNBaseInfo();
    loadGSNRoot();
  } else if (state.section === "base") {
    els.baseTitle.textContent = "Позиции пользователя";
    els.baseDescription.textContent =
      "Здесь будут пользовательские позиции, которые пользователь сможет применять в сметах.";
    els.gsnTree.classList.add("hidden");
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
          ${escapeHTML(estimate.title)}
        </button>
      `,
    )
    .join("");

  els.editorSubitems.innerHTML = `
    <button class="nav-sublink ${state.editorTab === "buffer" ? "active" : ""}" data-editor-tab="buffer" type="button">Буфер</button>
    ${estimateButtons}
  `;
}

async function loadGSNBaseInfo() {
  if (!state.token || state.gsnBaseInfoLoaded) {
    return;
  }

  try {
    state.gsnBaseInfo = await api("/api/gsn/base-info");
    state.gsnBaseInfoLoaded = true;
    renderGSNBaseInfo();
  } catch (error) {
    state.gsnBaseInfoLoaded = false;
    els.baseTitle.textContent = "ГСН-2022";
    els.baseDescription.textContent = error.message;
  }
}

function renderGSNBaseInfo() {
  const info = state.gsnBaseInfo;
  els.baseTitle.textContent = info?.edition || "ГСН-2022";
  els.baseDescription.textContent = info?.version ? `Версия: ${info.version}` : "Версия: -";
}

async function loadGSNRoot(options = {}) {
  if (!state.token || state.baseTab !== "gsn") {
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
  const params = new URLSearchParams({ limit: "300" });
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
    els.editorContent.innerHTML = state.buffer.length
      ? renderEditorItems(state.buffer, "Буфер пока пуст.")
      : `<p class="muted">Буфер пока пуст.</p>`;
    return;
  }

  const estimate = state.openEstimates.find((item) => item.id === state.editorTab);
  els.editorTitle.textContent = estimate.title;
  els.editorDescription.textContent = "Сессионная смета редактора. Пока хранится только в текущей сессии браузера.";
  els.editorContent.innerHTML = estimate.items.length
    ? renderEditorItems(estimate.items, "В смете пока нет позиций.")
    : `<p class="muted">В смете пока нет позиций.</p>`;
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

function createEditorEstimate() {
  const estimate = {
    id: `session_est_${Date.now()}`,
    title: `Смета ${state.openEstimates.length + 1}`,
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

function renderConstructionForms() {
  const constructionOptions = state.constructions
    .map((construction) => `<option value="${construction.id}">${escapeHTML(construction.name)}</option>`)
    .join("");

  els.objectForm.constructionId.innerHTML = constructionOptions;
  els.objectForm.querySelector("button").disabled = state.constructions.length === 0;

  const objectOptions = state.objects
    .map((object) => `<option value="${object.id}">${escapeHTML(objectPath(object))}</option>`)
    .join("");

  els.estimateForm.objectId.innerHTML = objectOptions;
  els.estimateForm.querySelector("button").disabled = state.objects.length === 0;
}

function renderConstructionTree() {
  if (!state.constructions.length) {
    els.constructionTree.innerHTML = `<p class="muted">Пока нет строек. Создайте стройку первого уровня.</p>`;
    return;
  }

  els.constructionTree.innerHTML = state.constructions
    .map((construction) => {
      const objects = state.objects.filter((object) => object.constructionId === construction.id);
      return `
        <article class="tree-node level-1">
          <header>
            <strong>Стройка: ${escapeHTML(construction.name)}</strong>
            <span class="muted">${construction.id}</span>
          </header>
          <div class="tree-children">
            ${
              objects.length
                ? objects.map(renderObjectNode).join("")
                : `<p class="muted">Добавьте объект второго уровня.</p>`
            }
          </div>
        </article>
      `;
    })
    .join("");

  els.constructionTree.querySelectorAll("[data-edit]").forEach((button) => {
    button.addEventListener("click", () => editEstimate(button.dataset.edit));
  });
  els.constructionTree.querySelectorAll("[data-delete]").forEach((button) => {
    button.addEventListener("click", () => deleteEstimate(button.dataset.delete));
  });
}

function renderObjectNode(object) {
  const estimates = state.estimates.filter((estimate) => estimate.objectId === object.id);
  return `
    <article class="tree-node level-2">
      <header>
        <strong>Объект: ${escapeHTML(object.name)}</strong>
        <span class="muted">${object.id}</span>
      </header>
      <div class="tree-children">
        ${
          estimates.length
            ? estimates.map(renderEstimateNode).join("")
            : `<p class="muted">Добавьте смету третьего уровня.</p>`
        }
      </div>
    </article>
  `;
}

function renderEstimateNode(estimate) {
  return `
    <article class="tree-node level-3">
      <header>
        <div>
          <strong>Смета: ${escapeHTML(estimate.title)}</strong>
          <p class="muted">${escapeHTML(estimate.description || "")}</p>
        </div>
        <span class="pill">${escapeHTML(estimate.status)}</span>
      </header>
      <ul class="estimate-items">
        ${estimate.items
          .map(
            (item) => `
              <li>
                ${escapeHTML(item.name)}:
                ${item.quantity} ${escapeHTML(item.unit)}
                x ${money.format(item.unitPrice)}
                = ${money.format(item.total)}
              </li>
            `,
          )
          .join("")}
      </ul>
      <footer>
        <span class="pill">Итого: ${money.format(estimate.total)}</span>
        <button data-edit="${estimate.id}" type="button">Редактировать</button>
        <button data-delete="${estimate.id}" class="danger" type="button">Удалить</button>
      </footer>
    </article>
  `;
}

function editEstimate(id) {
  const estimate = state.estimates.find((item) => item.id === id);
  if (!estimate) {
    return;
  }

  els.estimateForm.id.value = estimate.id;
  els.estimateForm.objectId.value = estimate.objectId;
  els.estimateForm.title.value = estimate.title;
  els.estimateForm.status.value = estimate.status;
  els.estimateForm.description.value = estimate.description || "";
  els.estimateForm.items.value = JSON.stringify(
    estimate.items.map(({ name, quantity, unit, unitPrice }) => ({ name, quantity, unit, unitPrice })),
    null,
    2,
  );
  els.estimateForm.scrollIntoView({ behavior: "smooth", block: "start" });
}

async function deleteEstimate(id) {
  if (!window.confirm("Удалить смету?")) {
    return;
  }

  try {
    await api(`/api/estimates/${id}`, { method: "DELETE" });
    await refreshConstructionData();
    showMessage("Смета удалена", "ok");
  } catch (error) {
    showMessage(error.message, "error");
  }
}

function resetEstimateForm() {
  els.estimateForm.reset();
  els.estimateForm.id.value = "";
  els.estimateForm.status.value = "draft";
  if (state.objects[0]) {
    els.estimateForm.objectId.value = state.objects[0].id;
  }
  els.estimateForm.items.value = JSON.stringify(defaultItems, null, 2);
}

function objectPath(object) {
  const construction = state.constructions.find((item) => item.id === object.constructionId);
  return construction ? `${construction.name} / ${object.name}` : object.name;
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
