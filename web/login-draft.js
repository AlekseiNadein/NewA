(function () {
  const KEY = "nav_login_draft";

  function readDraft() {
    try {
      const raw = localStorage.getItem(KEY);
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

  function saveDraft(form) {
    if (!form) {
      return;
    }
    const data = Object.fromEntries(new FormData(form).entries());
    localStorage.setItem(
      KEY,
      JSON.stringify({
        companyName: String(data.companyName || ""),
        name: String(data.name || ""),
      }),
    );
  }

  function applyDraft(form) {
    const draft = readDraft();
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

  function setup(form) {
    if (!form || form.dataset.loginDraftReady === "true") {
      return;
    }
    form.dataset.loginDraftReady = "true";

    let timer = 0;
    const scheduleSave = () => {
      window.clearTimeout(timer);
      timer = window.setTimeout(() => saveDraft(form), 250);
    };

    form.addEventListener("input", scheduleSave);
    form.addEventListener("change", scheduleSave);
    form.addEventListener("submit", () => {
      saveDraft(form);
    }, true);
    applyDraft(form);
    window.requestAnimationFrame(() => applyDraft(form));
  }

  function init() {
    setup(document.querySelector("#loginForm"));
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
