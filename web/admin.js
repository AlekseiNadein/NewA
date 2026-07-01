import {
  applyLoginDraft,
  bindMessage,
  createApi,
  escapeHTML,
  formData,
  iconPencil,
  iconTrash,
  ADMIN_LOGIN_DRAFT_KEY,
  saveLoginDraft,
  setupLoginDraftAutosave,
} from "./shared.js";

const api = createApi();

const state = {
  me: null,
  companies: [],
  adminUsers: [],
  adminSection: "users",
  adminEstimateLocks: [],
  adminLicenses: null,
  adminLicensesCompanyId: "",
  adminQueueStats: null,
};

const showMessage = bindMessage(document.querySelector("#message"));

const els = {
  loginView: document.querySelector("#loginView"),
  adminView: document.querySelector("#adminView"),
  logoutButton: document.querySelector("#logoutButton"),
  loginForm: document.querySelector("#loginForm"),
  adminUsersTable: document.querySelector("#adminUsersTable"),
  adminScopeHint: document.querySelector("#adminScopeHint"),
  showCreateUserButton: document.querySelector("#showCreateUserButton"),
  adminCreatePanel: document.querySelector("#adminCreatePanel"),
  adminCreateForm: document.querySelector("#adminCreateForm"),
  createCompanySelectField: document.querySelector("#createCompanySelectField"),
  createCompanyNameField: document.querySelector("#createCompanyNameField"),
  createSuperField: document.querySelector("#createSuperField"),
  cancelCreateUserButton: document.querySelector("#cancelCreateUserButton"),
  adminEditPanel: document.querySelector("#adminEditPanel"),
  adminUserForm: document.querySelector("#adminUserForm"),
  adminCompanyField: document.querySelector("#adminCompanyField"),
  cancelAdminEditButton: document.querySelector("#cancelAdminEditButton"),
  adminNav: document.querySelector("#adminNav"),
  adminCurrentUser: document.querySelector("#adminCurrentUser"),
  adminCurrentRole: document.querySelector("#adminCurrentRole"),
  adminUsersSection: document.querySelector("#adminUsersSection"),
  adminLicensesSection: document.querySelector("#adminLicensesSection"),
  adminLicensesHint: document.querySelector("#adminLicensesHint"),
  adminLicensesTable: document.querySelector("#adminLicensesTable"),
  adminLicensesCompanyField: document.querySelector("#adminLicensesCompanyField"),
  adminLicensesCompanySelect: document.querySelector("#adminLicensesCompanySelect"),
  saveAdminLicensesButton: document.querySelector("#saveAdminLicensesButton"),
  adminEstimatesSection: document.querySelector("#adminEstimatesSection"),
  adminEstimatesTable: document.querySelector("#adminEstimatesTable"),
  adminEstimatesHint: document.querySelector("#adminEstimatesHint"),
  refreshAdminEstimatesButton: document.querySelector("#refreshAdminEstimatesButton"),
  adminQueueSection: document.querySelector("#adminQueueSection"),
  adminQueueStats: document.querySelector("#adminQueueStats"),
  adminQueueHistory: document.querySelector("#adminQueueHistory"),
  adminQueueHint: document.querySelector("#adminQueueHint"),
  adminQueuePurgeTarget: document.querySelector("#adminQueuePurgeTarget"),
  adminQueuePurgeButton: document.querySelector("#adminQueuePurgeButton"),
  refreshAdminQueueButton: document.querySelector("#refreshAdminQueueButton"),
};

let adminQueuePollTimer = null;

els.loginForm?.addEventListener("submit", async (event) => {
  event.preventDefault();
  const credentials = formData(els.loginForm);
  saveLoginDraft(ADMIN_LOGIN_DRAFT_KEY, credentials);

  try {
    const result = await api("/api/auth/login", {
      method: "POST",
      body: credentials,
    });

    if (!result.access?.admin) {
      await api("/api/auth/logout", { method: "POST" }).catch(() => {});
      throw new Error("У этой учётной записи нет прав администратора");
    }

    state.me = result.user;
    localStorage.removeItem("nav_admin_token");
    localStorage.removeItem("nav_token");
    await loadAdmin();
    showMessage("Вход выполнен", "ok");
  } catch (error) {
    applyLoginDraft(els.loginForm, ADMIN_LOGIN_DRAFT_KEY);
    showMessage(error.message, "error");
  }
});

setupLoginDraftAutosave(els.loginForm, ADMIN_LOGIN_DRAFT_KEY);
applyLoginDraft(els.loginForm, ADMIN_LOGIN_DRAFT_KEY);
void loadAdmin();

els.logoutButton?.addEventListener("click", async () => {
  try {
    await api("/api/auth/logout", { method: "POST" });
  } catch {
    // ignore logout errors
  }
  state.me = null;
  localStorage.removeItem("nav_admin_token");
  localStorage.removeItem("nav_token");
  renderShell();
  applyLoginDraft(els.loginForm, ADMIN_LOGIN_DRAFT_KEY);
});

els.showCreateUserButton.addEventListener("click", () => {
  hideAdminEdit();
  openCreateUser();
});

els.cancelCreateUserButton.addEventListener("click", hideCreateUser);
els.cancelAdminEditButton.addEventListener("click", hideAdminEdit);

els.adminNav?.addEventListener("click", (event) => {
  const button = event.target.closest("[data-admin-section]");
  if (!button) {
    return;
  }
  void switchAdminSection(button.dataset.adminSection);
});

els.refreshAdminEstimatesButton?.addEventListener("click", async () => {
  try {
    await refreshAdminEstimateLocks();
    showMessage("Список обновлён", "ok");
  } catch (error) {
    showMessage(error.message, "error");
  }
});

els.refreshAdminQueueButton?.addEventListener("click", async () => {
  try {
    await refreshAdminQueueStats();
    showMessage("Статистика обновлена", "ok");
  } catch (error) {
    showMessage(error.message, "error");
  }
});

const ADMIN_QUEUE_PURGE_LABELS = {
  dlq: "DLQ (estimate.calc.dlq)",
  main: "основную очередь (estimate.calc.main)",
  retry: "retry-очереди",
  all: "все очереди расчёта",
};

els.adminQueuePurgeButton?.addEventListener("click", async () => {
  const target = String(els.adminQueuePurgeTarget?.value || "dlq").trim() || "dlq";
  const targetLabel = ADMIN_QUEUE_PURGE_LABELS[target] || target;
  const confirmed = window.confirm(
    `Очистить ${targetLabel}?\n\nВсе сообщения в выбранной очереди будут безвозвратно удалены.`,
  );
  if (!confirmed) {
    return;
  }

  els.adminQueuePurgeButton.disabled = true;
  try {
    const result = await api("/api/admin/queue-purge", {
      method: "POST",
      body: { target },
    });
    const total = Number(result.total || 0);
    showMessage(`Очередь очищена: удалено ${total} сообщ.`, "ok");
    await refreshAdminQueueStats();
  } catch (error) {
    showMessage(error.message, "error");
  } finally {
    els.adminQueuePurgeButton.disabled = false;
  }
});

els.adminLicensesCompanySelect?.addEventListener("change", async () => {
  state.adminLicensesCompanyId = els.adminLicensesCompanySelect.value;
  try {
    await refreshAdminLicenses();
  } catch (error) {
    showMessage(error.message, "error");
  }
});

els.saveAdminLicensesButton?.addEventListener("click", async () => {
  try {
    await saveAdminLicenses();
    showMessage("Лицензии сохранены", "ok");
  } catch (error) {
    showMessage(error.message, "error");
  }
});

els.adminCreateForm.addEventListener("submit", async (event) => {
  event.preventDefault();

  const data = formData(els.adminCreateForm);
  const isSuperAdmin = Boolean(state.me?.isSuperAdministrator);
  const payload = {
    companyId: isSuperAdmin ? data.companyId || "" : undefined,
    companyName: isSuperAdmin ? data.companyName || "" : undefined,
    name: data.name,
    email: data.email,
    password: data.password,
    authorized: Boolean(els.adminCreateForm.authorized.checked),
    isAdministrator: Boolean(els.adminCreateForm.isAdministrator.checked),
    isSuperAdministrator: Boolean(els.adminCreateForm.isSuperAdministrator?.checked),
  };

  try {
    await api("/api/users", {
      method: "POST",
      body: payload,
    });
    hideCreateUser();
    await refreshAdminUsers();
    showMessage("Пользователь создан", "ok");
  } catch (error) {
    showMessage(error.message, "error");
  }
});

els.adminUserForm.addEventListener("submit", async (event) => {
  event.preventDefault();

  const data = formData(els.adminUserForm);
  const editedUser = state.adminUsers.find((item) => item.id === data.id);
  const payload = {
    companyId: data.companyId || undefined,
    name: data.name,
    email: data.email,
    password: data.password || "",
    authorized: Boolean(els.adminUserForm.authorized.checked),
    isAdministrator: Boolean(els.adminUserForm.isAdministrator.checked),
    isSuperAdministrator: Boolean(editedUser?.isSuperAdministrator),
  };

  try {
    await api(`/api/users/${data.id}`, {
      method: "PUT",
      body: payload,
    });
    await refreshAdminUsers();
    hideAdminEdit();
    showMessage("Пользователь сохранён", "ok");
  } catch (error) {
    showMessage(error.message, "error");
  }
});

async function loadAdmin() {
  try {
    const me = await api("/api/me");
    state.me = me.user;
    if (!me.access?.admin) {
      throw new Error("Нет прав администратора");
    }

    state.companies = await api("/api/companies");
    await refreshAdminUsers();
    renderShell();
  } catch (error) {
    const message = error?.message || "";
    const hadSession = Boolean(state.me);
    state.me = null;
    localStorage.removeItem("nav_admin_token");
    localStorage.removeItem("nav_token");
    renderShell();
    applyLoginDraft(els.loginForm, ADMIN_LOGIN_DRAFT_KEY);
    if (hadSession) {
      showMessage("Сессия истекла или нет прав, войдите снова", "error");
    } else if (/Нет прав администратора/i.test(message)) {
      showMessage(message, "error");
    }
  }
}

async function refreshAdminUsers() {
  state.adminUsers = await api("/api/users");
  if (state.adminSection === "users") {
    renderAdminUsers();
  }
}

async function refreshAdminEstimateLocks() {
  state.adminEstimateLocks = await api("/api/admin/estimate-locks");
  if (state.adminSection === "estimates") {
    renderAdminEstimates();
  }
}

async function refreshAdminQueueStats() {
  state.adminQueueStats = await api("/api/admin/queue-stats");
  if (state.adminSection === "queue") {
    renderAdminQueue();
  }
}

async function refreshAdminLicenses() {
  const isSuperAdmin = Boolean(state.me?.isSuperAdministrator);
  const companyId = isSuperAdmin ? state.adminLicensesCompanyId || state.me?.companyId : state.me?.companyId;
  if (!companyId) {
    state.adminLicenses = null;
    return;
  }

  const query = isSuperAdmin ? `?companyId=${encodeURIComponent(companyId)}` : "";
  state.adminLicenses = await api(`/api/admin/licenses${query}`);
  state.adminLicensesCompanyId = state.adminLicenses.companyId;

  if (state.adminSection === "licenses") {
    renderAdminLicenses();
  }
}

async function saveAdminLicenses() {
  const licenses = state.adminLicenses;
  if (!licenses?.editable) {
    throw new Error("Недостаточно прав для изменения лицензий");
  }

  const inputs = els.adminLicensesTable.querySelectorAll("[data-license-count]");
  const items = {};
  inputs.forEach((input) => {
    items[input.dataset.licenseCount] = Math.max(0, Number.parseInt(input.value, 10) || 0);
  });

  state.adminLicenses = await api("/api/admin/licenses", {
    method: "PUT",
    body: {
      companyId: licenses.companyId,
      items,
    },
  });
  renderAdminLicenses();
}

const adminSectionViews = {
  users: () => els.adminUsersSection,
  licenses: () => els.adminLicensesSection,
  estimates: () => els.adminEstimatesSection,
  queue: () => els.adminQueueSection,
};

function setActiveAdminSection(section) {
  Object.entries(adminSectionViews).forEach(([key, getElement]) => {
    getElement().classList.toggle("hidden", key !== section);
  });
}

async function switchAdminSection(section) {
  if (!section || section === state.adminSection) {
    return;
  }

  if (section === "users") {
    hideCreateUser();
    hideAdminEdit();
  }

  state.adminSection = section;
  renderAdminNav();
  setActiveAdminSection(section);
  stopAdminQueuePoll();

  if (section === "users") {
    await refreshAdminUsers();
    return;
  }

  if (section === "licenses") {
    await refreshAdminLicenses();
    return;
  }

  if (section === "queue") {
    await refreshAdminQueueStats();
    startAdminQueuePoll();
    return;
  }

  await refreshAdminEstimateLocks();
}

function adminRoleLabel(user) {
  if (user.isSuperAdministrator) {
    return "суперадминистратор";
  }
  if (user.isAdministrator) {
    return "администратор";
  }
  return user.authorized ? "пользователь" : "ожидает подтверждения";
}

function renderAdminNav() {
  els.adminNav?.querySelectorAll("[data-admin-section]").forEach((button) => {
    button.classList.toggle("active", button.dataset.adminSection === state.adminSection);
  });
}

function renderShell() {
  const loggedIn = Boolean(state.me);
  els.loginView.classList.toggle("hidden", loggedIn);
  els.adminView.classList.toggle("hidden", !loggedIn);
  els.logoutButton.classList.toggle("hidden", !loggedIn);

  if (loggedIn) {
    const companyName =
      state.companies.find((company) => company.id === state.me.companyId)?.name || state.me.companyId;
    els.adminCurrentUser.textContent = `${state.me.name} (${companyName})`;
    els.adminCurrentRole.textContent = adminRoleLabel(state.me);
    renderAdminNav();
    if (state.adminSection === "estimates") {
      renderAdminEstimates();
    } else if (state.adminSection === "licenses") {
      renderAdminLicenses();
    } else if (state.adminSection === "queue") {
      renderAdminQueue();
      startAdminQueuePoll();
    } else {
      renderAdminUsers();
    }
  }
}

function renderAdminUsers() {
  const isSuperAdmin = Boolean(state.me?.isSuperAdministrator);
  els.adminScopeHint.textContent = isSuperAdmin
    ? "Суперадминистратор видит всех пользователей системы."
    : "Администратор компании видит пользователей только своей компании.";

  if (!state.adminUsers.length) {
    els.adminUsersTable.innerHTML = `<p class="muted">Пользователи не найдены.</p>`;
    return;
  }

  els.adminUsersTable.innerHTML = `
    <table class="admin-table admin-table--users">
      <thead>
        <tr>
          <th>Компания</th>
          <th>ФИО</th>
          <th>E-mail</th>
          <th>Авт.</th>
          <th>Адм.</th>
          ${isSuperAdmin ? "<th>Суп.</th>" : ""}
          <th class="admin-col-actions"></th>
        </tr>
      </thead>
      <tbody>
        ${state.adminUsers
          .map(
            (user) => `
              <tr>
                <td>${escapeHTML(user.companyName || "")}</td>
                <td>${escapeHTML(user.name)}</td>
                <td class="admin-cell-email" title="${escapeHTML(user.email)}">${escapeHTML(user.email)}</td>
                <td>${user.authorized ? "да" : "нет"}</td>
                <td>${user.isAdministrator ? "да" : "нет"}</td>
                ${isSuperAdmin ? `<td>${user.isSuperAdministrator ? "да" : "нет"}</td>` : ""}
                <td class="admin-row-actions">
                  <button
                    class="icon-button secondary"
                    data-edit-user="${user.id}"
                    type="button"
                    title="Изменить"
                    aria-label="Изменить"
                  >${iconPencil()}</button>
                  ${
                    user.isSuperAdministrator
                      ? ""
                      : `<button
                          class="icon-button secondary icon-button-danger"
                          data-delete-user="${user.id}"
                          type="button"
                          title="Удалить"
                          aria-label="Удалить"
                        >${iconTrash()}</button>`
                  }
                </td>
              </tr>
            `,
          )
          .join("")}
      </tbody>
    </table>
  `;

  els.adminUsersTable.querySelectorAll("[data-edit-user]").forEach((button) => {
    button.addEventListener("click", () => openAdminEdit(button.dataset.editUser));
  });

  els.adminUsersTable.querySelectorAll("[data-delete-user]").forEach((button) => {
    button.addEventListener("click", () => deleteAdminUser(button.dataset.deleteUser));
  });
}

function renderAdminLicenses() {
  const isSuperAdmin = Boolean(state.me?.isSuperAdministrator);
  const licenses = state.adminLicenses;

  els.adminLicensesHint.textContent = isSuperAdmin
    ? "Суперадминистратор задаёт количество лицензий для каждой компании."
    : "Количество лицензий вашей компании на подразделы базы. Изменение доступно только суперадминистратору.";

  els.adminLicensesCompanyField.classList.toggle("hidden", !isSuperAdmin);
  els.saveAdminLicensesButton.classList.toggle("hidden", !licenses?.editable);

  if (isSuperAdmin) {
    const selectedId = licenses?.companyId || state.adminLicensesCompanyId || state.me?.companyId || "";
    const options = state.companies
      .map(
        (company) =>
          `<option value="${company.id}" ${company.id === selectedId ? "selected" : ""}>${escapeHTML(company.name)}</option>`,
      )
      .join("");
    els.adminLicensesCompanySelect.innerHTML = options;
    state.adminLicensesCompanyId = selectedId;
  }

  if (!licenses?.items?.length) {
    els.adminLicensesTable.innerHTML = `<p class="muted">Лицензии не найдены.</p>`;
    return;
  }

  const editable = Boolean(licenses.editable);
  els.adminLicensesTable.innerHTML = `
    <table class="admin-table admin-table--licenses">
      <thead>
        <tr>
          <th>Подраздел «Базы»</th>
          <th>Доступно лицензий</th>
        </tr>
      </thead>
      <tbody>
        ${licenses.items
          .map(
            (item) => `
              <tr>
                <td>${escapeHTML(item.name)}</td>
                <td>
                  ${
                    editable
                      ? `<input
                          class="admin-license-count"
                          data-license-count="${escapeHTML(item.subsectionId)}"
                          type="number"
                          min="0"
                          step="1"
                          value="${item.available}"
                        />`
                      : escapeHTML(String(item.available))
                  }
                </td>
              </tr>
            `,
          )
          .join("")}
      </tbody>
    </table>
  `;
}

async function deleteAdminUser(userId) {
  const user = state.adminUsers.find((item) => item.id === userId);
  if (!user) {
    return;
  }

  if (!window.confirm(`Удалить пользователя «${user.name}» (${user.email})?`)) {
    return;
  }

  try {
    await api(`/api/users/${userId}`, { method: "DELETE" });
    if (els.adminUserForm.id.value === userId) {
      hideAdminEdit();
    }
    await refreshAdminUsers();
    showMessage("Пользователь удалён", "ok");
  } catch (error) {
    showMessage(error.message, "error");
  }
}

function formatAdminDateTime(value) {
  if (!value) {
    return "—";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "—";
  }
  return date.toLocaleString("ru-RU");
}

function renderAdminEstimates() {
  const isSuperAdmin = Boolean(state.me?.isSuperAdministrator);
  els.adminEstimatesHint.textContent = isSuperAdmin
    ? "Суперадминистратор видит активные сессии редактирования по всем компаниям."
    : "Администратор компании видит активные сессии редактирования только своей компании.";

  if (!state.adminEstimateLocks.length) {
    els.adminEstimatesTable.innerHTML = `<p class="muted">Сейчас нет смет в редактировании.</p>`;
    return;
  }

  const estimateLabel = (lock) => {
    const parts = [lock.estimateCode, lock.estimateTitle].filter(Boolean);
    return parts.length ? parts.join(" — ") : lock.estimateId;
  };

  els.adminEstimatesTable.innerHTML = `
    <table class="admin-table">
      <thead>
        <tr>
          ${isSuperAdmin ? "<th>Компания</th>" : ""}
          <th>Смета</th>
          <th>Редактор</th>
          <th>Начато</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        ${state.adminEstimateLocks
          .map(
            (lock) => `
              <tr>
                ${isSuperAdmin ? `<td>${escapeHTML(lock.companyName || "")}</td>` : ""}
                <td>${escapeHTML(estimateLabel(lock))}</td>
                <td>${escapeHTML(lock.userName)}</td>
                <td>${escapeHTML(formatAdminDateTime(lock.updatedAt))}</td>
                <td class="admin-row-actions">
                  <button
                    data-finish-estimate-lock="${lock.estimateId}"
                    class="secondary"
                    type="button"
                  >Завершить редактирование</button>
                </td>
              </tr>
            `,
          )
          .join("")}
      </tbody>
    </table>
  `;

  els.adminEstimatesTable.querySelectorAll("[data-finish-estimate-lock]").forEach((button) => {
    button.addEventListener("click", () => finishAdminEstimateLock(button.dataset.finishEstimateLock));
  });
}

async function finishAdminEstimateLock(estimateId) {
  const lock = state.adminEstimateLocks.find((item) => item.estimateId === estimateId);
  if (!lock) {
    return;
  }

  const estimateLabel = [lock.estimateCode, lock.estimateTitle].filter(Boolean).join(" — ") || estimateId;
  if (!window.confirm(`Завершить редактирование сметы «${estimateLabel}» пользователем ${lock.userName}?`)) {
    return;
  }

  try {
    await api(`/api/admin/estimate-locks/${estimateId}`, { method: "DELETE" });
    await refreshAdminEstimateLocks();
    showMessage("Редактирование завершено", "ok");
  } catch (error) {
    showMessage(error.message, "error");
  }
}

function queueStatusClass(value, okWhen) {
  return value === okWhen ? "admin-queue-ok" : "admin-queue-warn";
}

function queueBoolLabel(value) {
  return value ? "да" : "нет";
}

function renderAdminQueue() {
  const stats = state.adminQueueStats;
  const queueMode = stats?.queue?.mode || "";
  const purgeEnabled = queueMode === "rabbit" || queueMode === "dual";
  if (els.adminQueuePurgeButton) {
    els.adminQueuePurgeButton.disabled = !purgeEnabled;
  }
  if (els.adminQueuePurgeTarget) {
    els.adminQueuePurgeTarget.disabled = !purgeEnabled;
  }
  if (!stats) {
    els.adminQueueStats.innerHTML = `<p class="muted">Загрузка…</p>`;
    els.adminQueueHistory.innerHTML = "";
    return;
  }

  const queue = stats.queue || {};
  const depths = queue.depths || {};
  const dlq = stats.dlq || {};
  const alerts = stats.alerts || {};
  const dlqCurrent = dlq.current ?? depths["estimate.calc.dlq"] ?? 0;
  const mainDepth = depths["estimate.calc.main"] ?? 0;
  const growth10m = dlq.growth10m ?? 0;
  const growthKnown = Boolean(dlq.hasGrowthBaseline);
  const growthText = growthKnown ? `${growth10m >= 0 ? "+" : ""}${growth10m}` : "—";
  const activeAlerts = Object.entries(alerts)
    .filter(([, active]) => active)
    .map(([key]) => key);

  els.adminQueueHint.textContent =
    queue.mode === "rabbit"
      ? "Режим RabbitMQ: мониторинг DLQ, outbox и подключений publisher/consumer. Очистка очереди доступна ниже."
      : `Режим «${queue.mode || "—"}»: статистика очереди в основном актуальна для rabbit.`;

  els.adminQueueStats.innerHTML = `
    <div class="admin-queue-grid">
      <div class="admin-queue-card">
        <span class="admin-queue-label">Статус</span>
        <strong class="${stats.status === "ok" ? "admin-queue-ok" : "admin-queue-warn"}">${escapeHTML(stats.status || "—")}</strong>
      </div>
      <div class="admin-queue-card">
        <span class="admin-queue-label">DLQ (сейчас)</span>
        <strong class="${dlqCurrent === 0 ? "admin-queue-ok" : "admin-queue-warn"}">${dlqCurrent}</strong>
      </div>
      <div class="admin-queue-card">
        <span class="admin-queue-label">Рост DLQ за 10 мин</span>
        <strong class="${dlq.growthAlert ? "admin-queue-warn" : "admin-queue-ok"}">${escapeHTML(growthText)}</strong>
        ${growthKnown ? `<span class="muted admin-queue-sub">порог ${dlq.growthThreshold ?? 50}</span>` : `<span class="muted admin-queue-sub">нужна история</span>`}
      </div>
      <div class="admin-queue-card">
        <span class="admin-queue-label">Основная очередь</span>
        <strong>${mainDepth}</strong>
      </div>
      <div class="admin-queue-card">
        <span class="admin-queue-label">Outbox pending</span>
        <strong class="${(queue.outboxPending ?? 0) > 100 ? "admin-queue-warn" : ""}">${queue.outboxPending ?? 0}</strong>
      </div>
      <div class="admin-queue-card">
        <span class="admin-queue-label">Pipeline ready</span>
        <strong class="${queueStatusClass(stats.pipelineReady, true)}">${queueBoolLabel(stats.pipelineReady)}</strong>
      </div>
      <div class="admin-queue-card">
        <span class="admin-queue-label">Release ready</span>
        <strong class="${queueStatusClass(stats.releaseReady, true)}">${queueBoolLabel(stats.releaseReady)}</strong>
      </div>
      <div class="admin-queue-card">
        <span class="admin-queue-label">Publisher</span>
        <strong class="${queueStatusClass(queue.publisher?.connected, true)}">${queueBoolLabel(queue.publisher?.connected)}</strong>
      </div>
      <div class="admin-queue-card">
        <span class="admin-queue-label">Consumer</span>
        <strong class="${queueStatusClass(queue.consumer?.connected, true)}">${queueBoolLabel(queue.consumer?.connected)}</strong>
      </div>
    </div>
    ${
      activeAlerts.length
        ? `<p class="admin-queue-alerts"><strong>Алерты:</strong> ${activeAlerts.map((key) => escapeHTML(key)).join(", ")}</p>`
        : `<p class="muted admin-queue-alerts">Активных алертов нет.</p>`
    }
  `;

  const history = Array.isArray(stats.history) ? [...stats.history].reverse() : [];
  if (!history.length) {
    els.adminQueueHistory.innerHTML = `<p class="muted">История накопится после нескольких опросов (не чаще раза в минуту).</p>`;
    return;
  }

  els.adminQueueHistory.innerHTML = `
    <table class="admin-table admin-table--queue">
      <thead>
        <tr>
          <th>Время</th>
          <th>DLQ</th>
          <th>Main</th>
          <th>Outbox</th>
        </tr>
      </thead>
      <tbody>
        ${history
          .map(
            (point) => `
              <tr>
                <td>${escapeHTML(formatAdminDateTime(point.at))}</td>
                <td class="${point.dlq > 0 ? "admin-queue-warn" : ""}">${point.dlq ?? 0}</td>
                <td>${point.main ?? 0}</td>
                <td>${point.outboxPending ?? 0}</td>
              </tr>
            `,
          )
          .join("")}
      </tbody>
    </table>
  `;
}

function startAdminQueuePoll() {
  stopAdminQueuePoll();
  adminQueuePollTimer = window.setInterval(() => {
    void refreshAdminQueueStats().catch(() => {});
  }, 30000);
}

function stopAdminQueuePoll() {
  if (adminQueuePollTimer != null) {
    window.clearInterval(adminQueuePollTimer);
    adminQueuePollTimer = null;
  }
}

function openCreateUser() {
  const isSuperAdmin = Boolean(state.me?.isSuperAdministrator);
  els.createCompanySelectField.classList.toggle("hidden", !isSuperAdmin);
  els.createCompanyNameField.classList.toggle("hidden", !isSuperAdmin);
  els.createSuperField.classList.toggle("hidden", !isSuperAdmin);

  if (isSuperAdmin) {
    const options = state.companies
      .map((company) => `<option value="${company.id}">${escapeHTML(company.name)}</option>`)
      .join("");
    els.adminCreateForm.companyId.innerHTML = `<option value="">— выберите компанию —</option>${options}`;
  }

  els.adminCreateForm.reset();
  els.adminCreatePanel.classList.remove("hidden");
  els.adminCreatePanel.scrollIntoView({ behavior: "smooth", block: "start" });
}

function hideCreateUser() {
  els.adminCreatePanel.classList.add("hidden");
  els.adminCreateForm.reset();
}

function openAdminEdit(userId) {
  hideCreateUser();

  const user = state.adminUsers.find((item) => item.id === userId);
  if (!user) {
    return;
  }

  const isSuperAdmin = Boolean(state.me?.isSuperAdministrator);
  els.adminCompanyField.classList.toggle("hidden", !isSuperAdmin);

  if (isSuperAdmin) {
    const options = state.companies
      .map(
        (company) =>
          `<option value="${company.id}" ${company.id === user.companyId ? "selected" : ""}>${escapeHTML(company.name)}</option>`,
      )
      .join("");
    els.adminUserForm.companyId.innerHTML = options;
  }

  els.adminUserForm.id.value = user.id;
  els.adminUserForm.name.value = user.name;
  els.adminUserForm.email.value = user.email;
  els.adminUserForm.password.value = "";
  els.adminUserForm.authorized.checked = Boolean(user.authorized);
  els.adminUserForm.isAdministrator.checked = Boolean(user.isAdministrator);

  els.adminEditPanel.classList.remove("hidden");
  els.adminEditPanel.scrollIntoView({ behavior: "smooth", block: "start" });
}

function hideAdminEdit() {
  els.adminEditPanel.classList.add("hidden");
  els.adminUserForm.reset();
}
