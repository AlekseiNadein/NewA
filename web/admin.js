import { bindMessage, createApi, escapeHTML, formData } from "./shared.js";

const TOKEN_KEY = "nav_admin_token";

const state = {
  token: localStorage.getItem(TOKEN_KEY) || "",
  me: null,
  companies: [],
  adminUsers: [],
};

const api = createApi(() => state.token);
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
};

els.loginForm.addEventListener("submit", async (event) => {
  event.preventDefault();

  try {
    const result = await api("/api/auth/login", {
      method: "POST",
      body: formData(els.loginForm),
      skipAuth: true,
    });

    if (!result.access?.admin) {
      throw new Error("У этой учётной записи нет прав администратора");
    }

    state.token = result.token;
    state.me = result.user;
    localStorage.setItem(TOKEN_KEY, state.token);
    await loadAdmin();
    showMessage("Вход выполнен", "ok");
  } catch (error) {
    showMessage(error.message, "error");
  }
});

els.logoutButton.addEventListener("click", () => {
  state.token = "";
  state.me = null;
  localStorage.removeItem(TOKEN_KEY);
  renderShell();
});

els.showCreateUserButton.addEventListener("click", () => {
  hideAdminEdit();
  openCreateUser();
});

els.cancelCreateUserButton.addEventListener("click", hideCreateUser);
els.cancelAdminEditButton.addEventListener("click", hideAdminEdit);

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
  if (!state.token) {
    renderShell();
    return;
  }

  try {
    const me = await api("/api/me");
    state.me = me.user;
    if (!me.access?.admin) {
      throw new Error("Нет прав администратора");
    }

    state.companies = await api("/api/companies");
    await refreshAdminUsers();
    renderShell();
  } catch {
    state.token = "";
    state.me = null;
    localStorage.removeItem(TOKEN_KEY);
    renderShell();
    showMessage("Сессия истекла или нет прав, войдите снова", "error");
  }
}

async function refreshAdminUsers() {
  state.adminUsers = await api("/api/users");
  renderAdminUsers();
}

function renderShell() {
  const loggedIn = Boolean(state.token && state.me);
  els.loginView.classList.toggle("hidden", loggedIn);
  els.adminView.classList.toggle("hidden", !loggedIn);
  els.logoutButton.classList.toggle("hidden", !loggedIn);

  if (loggedIn) {
    renderAdminUsers();
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
    <table class="admin-table">
      <thead>
        <tr>
          <th>Компания</th>
          <th>ФИО</th>
          <th>E-mail</th>
          <th>Авторизован</th>
          <th>Админ</th>
          ${isSuperAdmin ? "<th>Суперадмин</th>" : ""}
          <th></th>
        </tr>
      </thead>
      <tbody>
        ${state.adminUsers
          .map(
            (user) => `
              <tr>
                <td>${escapeHTML(user.companyName || "")}</td>
                <td>${escapeHTML(user.name)}</td>
                <td>${escapeHTML(user.email)}</td>
                <td>${user.authorized ? "да" : "нет"}</td>
                <td>${user.isAdministrator ? "да" : "нет"}</td>
                ${isSuperAdmin ? `<td>${user.isSuperAdministrator ? "да" : "нет"}</td>` : ""}
                <td><button data-edit-user="${user.id}" type="button">Изменить</button></td>
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

loadAdmin();
