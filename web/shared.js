export function createApi(getToken) {
  return async function api(path, options = {}) {
    const headers = {
      "Content-Type": "application/json",
      ...(options.headers || {}),
    };

    const token = getToken();
    if (!options.skipAuth && token) {
      headers.Authorization = `Bearer ${token}`;
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
  };
}

export function formData(form) {
  return Object.fromEntries(new FormData(form).entries());
}

export const LOGIN_DRAFT_KEY = "nav_login_draft";
export const ADMIN_LOGIN_DRAFT_KEY = "nav_admin_login_draft";

export function readLoginDraft(storageKey) {
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

export function saveLoginDraft(storageKey, data) {
  localStorage.setItem(
    storageKey,
    JSON.stringify({
      companyName: String(data.companyName || ""),
      name: String(data.name || ""),
      password: String(data.password || ""),
    }),
  );
}

export function applyLoginDraft(form, storageKey) {
  const draft = readLoginDraft(storageKey);
  if (!draft || !form) {
    return;
  }

  const companyInput = form.querySelector('[name="companyName"]');
  const nameInput = form.querySelector('[name="name"]');
  const passwordInput = form.querySelector('[name="password"]');
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

export function setupLoginDraftAutosave(form, storageKey) {
  if (!form) {
    return;
  }

  let timer = 0;
  const save = () => {
    window.clearTimeout(timer);
    timer = window.setTimeout(() => {
      saveLoginDraft(storageKey, formData(form));
    }, 250);
  };

  form.addEventListener("input", save);
  form.addEventListener("change", save);
}

export function escapeHTML(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

export function bindMessage(messageEl) {
  return function showMessage(text, kind = "") {
    messageEl.textContent = text;
    messageEl.className = `message ${kind}`;
    window.clearTimeout(showMessage.timer);
    showMessage.timer = window.setTimeout(() => {
      messageEl.classList.add("hidden");
    }, 3600);
  };
}

export function iconPencil() {
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

export function iconTrash() {
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
