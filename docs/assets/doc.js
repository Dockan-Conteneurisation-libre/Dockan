"use strict";
// Dockan documentation pages: language switch (shared choice with the
// landing page), copy buttons and a table of contents that follows the
// reading position. No dependency, no network request.
(() => {
  const LANGS = ["eo", "fr", "en", "es"];
  const STORAGE_KEY = "dockan-lang";
  const dict = JSON.parse(document.getElementById("i18n-data").textContent);
  const common = JSON.parse(document.getElementById("i18n-common").textContent);

  // Esperanto is the text written in the page: capture it once.
  const textNodes = [...document.querySelectorAll("[data-i18n]")];
  const htmlNodes = [...document.querySelectorAll("[data-i18n-html]")];
  const ariaNodes = [...document.querySelectorAll("[data-i18n-aria]")];
  dict.eo = Object.assign({}, common.eo);
  for (const el of textNodes) dict.eo[el.dataset.i18n] = el.textContent;
  for (const el of htmlNodes) dict.eo[el.dataset.i18nHtml] = el.innerHTML;
  for (const el of ariaNodes) dict.eo[el.dataset.i18nAria] = el.getAttribute("aria-label");
  for (const lang of ["fr", "en", "es"]) dict[lang] = Object.assign({}, common[lang], dict[lang]);

  let current = "eo";
  const t = (key) => dict[current][key] ?? dict.eo[key];

  function applyLang(lang) {
    current = LANGS.includes(lang) ? lang : "eo";
    document.documentElement.lang = current;
    for (const el of textNodes) el.textContent = t(el.dataset.i18n);
    for (const el of htmlNodes) el.innerHTML = t(el.dataset.i18nHtml);
    for (const el of ariaNodes) el.setAttribute("aria-label", t(el.dataset.i18nAria));
    for (const b of document.querySelectorAll(".langs button")) {
      b.setAttribute("aria-pressed", String(b.dataset.lang === current));
    }
    document.querySelector(".lang-select").value = current;
    const title = document.querySelector("h1");
    if (title) document.title = title.textContent + " | Dockan";
  }

  function chooseLang(lang) {
    applyLang(lang);
    try { localStorage.setItem(STORAGE_KEY, current); } catch (err) {
      console.warn("Language choice not saved:", err);
    }
  }

  for (const b of document.querySelectorAll(".langs button")) {
    b.addEventListener("click", () => chooseLang(b.dataset.lang));
  }
  document.querySelector(".lang-select").addEventListener("change", (e) => chooseLang(e.target.value));
  let saved = null;
  try { saved = localStorage.getItem(STORAGE_KEY); } catch (err) {
    console.warn("Language choice not readable:", err);
  }
  applyLang(saved ?? "eo");

  const menu = document.querySelector(".menu");
  for (const a of document.querySelectorAll(".menu-links a")) {
    a.addEventListener("click", () => { menu.open = false; });
  }

  // Copy buttons are added to every code block. When the clipboard is
  // refused, the command is selected so it can be copied by hand.
  for (const pre of document.querySelectorAll("article .code pre")) {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "copy";
    btn.textContent = t("ui.copy");
    btn.addEventListener("click", async () => {
      let state = "ui.copied";
      try {
        await navigator.clipboard.writeText(pre.textContent);
      } catch (err) {
        const range = document.createRange();
        range.selectNodeContents(pre);
        const sel = window.getSelection();
        sel.removeAllRanges();
        sel.addRange(range);
        state = "ui.selected";
        console.warn("Clipboard unavailable, text selected instead:", err);
      }
      btn.textContent = t(state);
      btn.dataset.state = "done";
      setTimeout(() => { btn.textContent = t("ui.copy"); delete btn.dataset.state; }, 1800);
    });
    pre.parentElement.appendChild(btn);
  }
  const copyLabel = () => {
    for (const b of document.querySelectorAll(".copy:not([data-state])")) b.textContent = t("ui.copy");
  };
  for (const b of document.querySelectorAll(".langs button")) b.addEventListener("click", copyLabel);
  document.querySelector(".lang-select").addEventListener("change", copyLabel);

  // Table of contents: the current section follows scrolling and clicks.
  const links = [...document.querySelectorAll(".toc a")];
  const sections = links.map((a) => document.getElementById(a.hash.slice(1))).filter(Boolean);
  const mark = (id) => {
    for (const a of links) {
      if (a.hash === "#" + id) a.setAttribute("aria-current", "true");
      else a.removeAttribute("aria-current");
    }
  };
  const update = () => {
    let active = sections[0];
    for (const s of sections) {
      if (s.getBoundingClientRect().top <= 120) active = s;
    }
    if (window.innerHeight + window.scrollY >= document.body.scrollHeight - 4) {
      active = sections[sections.length - 1];
    }
    if (active) mark(active.id);
  };
  for (const a of links) a.addEventListener("click", () => mark(a.hash.slice(1)));
  window.addEventListener("scroll", update, { passive: true });
  update();
})();
