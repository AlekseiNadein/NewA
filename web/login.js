(() => {
  const LOGIN_DRAFT_KEY = "nav_login_draft";
  const ALLOWED_NEXT_PREFIXES = ["/", "/projectStatusDesktop", "/projectStatusMobile"];

  const els = {
    loginView: document.querySelector("#loginView"),
    registerView: document.querySelector("#registerView"),
    loginForm: document.querySelector("#loginForm"),
    registerForm: document.querySelector("#registerForm"),
    showRegisterButton: document.querySelector("#showRegisterButton"),
    showLoginButton: document.querySelector("#showLoginButton"),
    message: document.querySelector("#message"),
  };

  function formData(form) {
    return Object.fromEntries(new FormData(form).entries());
  }

  async function api(path, options = {}) {
    const response = await fetch(path, {
      method: options.method || "GET",
      headers: {
        "Content-Type": "application/json",
        ...(options.headers || {}),
      },
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
    if (!els.message) {
      return;
    }
    els.message.textContent = text || "";
    els.message.classList.toggle("hidden", !text);
    els.message.classList.toggle("ok", kind === "ok");
    els.message.classList.toggle("error", kind === "error");
  }

  function showAuthScreen(screen) {
    els.loginView?.classList.toggle("hidden", screen !== "login");
    els.registerView?.classList.toggle("hidden", screen !== "register");
  }

  function normalizeNext(raw) {
    const value = String(raw || "").trim();
    if (!value) {
      return "/";
    }
    if (!value.startsWith("/") || value.startsWith("//")) {
      return "/";
    }
    if (/^[a-zA-Z][a-zA-Z0-9+.-]*:/.test(value)) {
      return "/";
    }
    const path = value.split(/[?#]/)[0] || "/";
    const allowed =
      path === "/" ||
      ALLOWED_NEXT_PREFIXES.some((prefix) => prefix !== "/" && (path === prefix || path.startsWith(`${prefix}/`)));
    if (!allowed) {
      return "/";
    }
    return value;
  }

  function readNextTarget() {
    const params = new URLSearchParams(window.location.search);
    return normalizeNext(params.get("next") || params.get("returnUrl") || "/");
  }

  function redirectAfterLogin() {
    window.location.assign(readNextTarget());
  }

  function goToLogin(message) {
    showAuthScreen("login");
    if (message) {
      showMessage(message, "error");
    }
  }

  els.loginForm?.addEventListener("submit", async (event) => {
    event.preventDefault();
    const credentials = formData(els.loginForm);
    try {
      const result = await api("/api/auth/login", {
        method: "POST",
        body: credentials,
      });
      if (!result.access?.app) {
        await api("/api/auth/logout", { method: "POST" }).catch(() => {});
        throw new Error("Доступ к системе не открыт. Ожидайте подтверждения администратора");
      }
      localStorage.removeItem("nav_token");
      localStorage.removeItem("nav_admin_token");
      redirectAfterLogin();
    } catch (error) {
      showMessage(error.message || "Не удалось выполнить вход", "error");
    }
  });

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
      goToLogin(null);
      showMessage(result.message || "Заявка отправлена", "ok");
    } catch (error) {
      showMessage(error.message || "Не удалось отправить заявку", "error");
    }
  });

  els.showRegisterButton?.addEventListener("click", () => {
    showAuthScreen("register");
    showMessage("", "");
  });
  els.showLoginButton?.addEventListener("click", () => {
    showAuthScreen("login");
    showMessage("", "");
  });

  async function bootstrap() {
    showAuthScreen("login");
    const params = new URLSearchParams(window.location.search);
    const reason = params.get("reason");
    if (reason === "expired") {
      showMessage("Сессия истекла, войдите снова", "error");
    } else if (reason === "logout") {
      showMessage("Вы вышли из системы", "ok");
    }
    try {
      const me = await api("/api/me");
      if (me?.access?.app || me?.user?.authorized) {
        redirectAfterLogin();
        return;
      }
    } catch {
      // remain on login form
    }
  }

  void bootstrap();
})();
