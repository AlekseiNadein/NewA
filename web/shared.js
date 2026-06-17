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
