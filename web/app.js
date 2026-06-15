const state = {
  token: localStorage.getItem("nav_token") || "",
  me: null,
  companies: [],
  users: [],
  estimates: [],
};

const defaultItems = [
  { name: "Concrete works", quantity: 1, unit: "m3", unitPrice: 1500 },
  { name: "Labor", quantity: 8, unit: "h", unitPrice: 900 },
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
  adminPanel: document.querySelector("#adminPanel"),
  companyForm: document.querySelector("#companyForm"),
  userForm: document.querySelector("#userForm"),
  estimateForm: document.querySelector("#estimateForm"),
  resetEstimateForm: document.querySelector("#resetEstimateForm"),
  companiesList: document.querySelector("#companiesList"),
  usersList: document.querySelector("#usersList"),
  estimatesList: document.querySelector("#estimatesList"),
  message: document.querySelector("#message"),
};

els.loginForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  const data = formData(els.loginForm);

  try {
    const result = await api("/api/auth/login", {
      method: "POST",
      body: data,
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

els.companyForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  try {
    await api("/api/companies", {
      method: "POST",
      body: formData(els.companyForm),
    });
    els.companyForm.reset();
    await refreshAdminData();
    showMessage("Компания создана", "ok");
  } catch (error) {
    showMessage(error.message, "error");
  }
});

els.userForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  try {
    await api("/api/users", {
      method: "POST",
      body: formData(els.userForm),
    });
    els.userForm.reset();
    await refreshAdminData();
    showMessage("Пользователь создан", "ok");
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
    await refreshEstimates();
    showMessage("Смета сохранена", "ok");
  } catch (error) {
    showMessage(error.message, "error");
  }
});

els.resetEstimateForm.addEventListener("click", resetEstimateForm);

async function loadApp() {
  if (!state.token) {
    renderShell();
    return;
  }

  try {
    const me = await api("/api/me");
    state.me = me.user;
    await Promise.all([refreshAdminData(), refreshEstimates()]);
    renderShell();
  } catch (error) {
    state.token = "";
    localStorage.removeItem("nav_token");
    renderShell();
    showMessage("Сессия истекла, войдите снова", "error");
  }
}

async function refreshAdminData() {
  state.companies = await api("/api/companies");
  if (canManageUsers()) {
    state.users = await api("/api/users");
  } else {
    state.users = [];
  }
  renderAdmin();
}

async function refreshEstimates() {
  state.estimates = await api("/api/estimates");
  renderEstimates();
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
  els.adminPanel.classList.toggle("hidden", !canManageUsers());
  els.companyForm.classList.toggle("hidden", state.me.role !== "super_admin");

  renderAdmin();
  renderEstimates();
  resetEstimateForm();
}

function renderAdmin() {
  if (!state.me) {
    return;
  }

  els.companiesList.innerHTML = state.companies
    .map(
      (company) => `
        <article class="list-item">
          <strong>${escapeHTML(company.name)}</strong>
          <span class="muted">${company.id}</span>
        </article>
      `,
    )
    .join("");

  els.usersList.innerHTML = state.users
    .map(
      (user) => `
        <article class="list-item">
          <strong>${escapeHTML(user.name)}</strong>
          <span>${escapeHTML(user.email)}</span>
          <span class="pill">${escapeHTML(user.role)}</span>
        </article>
      `,
    )
    .join("");

  const companySelect = els.userForm.companyId;
  companySelect.innerHTML = state.companies
    .map((company) => `<option value="${company.id}">${escapeHTML(company.name)}</option>`)
    .join("");
}

function renderEstimates() {
  if (!state.estimates.length) {
    els.estimatesList.innerHTML = `<p class="muted">Пока нет смет.</p>`;
    return;
  }

  els.estimatesList.innerHTML = state.estimates
    .map(
      (estimate) => `
        <article class="estimate">
          <header>
            <div>
              <strong>${escapeHTML(estimate.title)}</strong>
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
      `,
    )
    .join("");

  els.estimatesList.querySelectorAll("[data-edit]").forEach((button) => {
    button.addEventListener("click", () => editEstimate(button.dataset.edit));
  });
  els.estimatesList.querySelectorAll("[data-delete]").forEach((button) => {
    button.addEventListener("click", () => deleteEstimate(button.dataset.delete));
  });
}

function editEstimate(id) {
  const estimate = state.estimates.find((item) => item.id === id);
  if (!estimate) {
    return;
  }

  els.estimateForm.id.value = estimate.id;
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
    await refreshEstimates();
    showMessage("Смета удалена", "ok");
  } catch (error) {
    showMessage(error.message, "error");
  }
}

function resetEstimateForm() {
  els.estimateForm.reset();
  els.estimateForm.id.value = "";
  els.estimateForm.status.value = "draft";
  els.estimateForm.items.value = JSON.stringify(defaultItems, null, 2);
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

function canManageUsers() {
  return state.me && ["super_admin", "company_admin"].includes(state.me.role);
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
