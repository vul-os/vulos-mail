import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "@vulos/mail-ui/style.css"; // shared mail UI: OSS-native tokens + components
import "./styles.css";              // app shell + login styles (themes the mail UI)
import App from "./App.jsx";

// Apply the saved theme before first paint so the login screen matches the
// theme chosen inside the app. Single source of truth: the mail-ui settings
// store ("vulos-mail.settings.v1"); "system" resolves to the OS preference.
function resolveTheme() {
  let pref = "system";
  try {
    const raw = localStorage.getItem("vulos-mail.settings.v1");
    if (raw) pref = JSON.parse(raw).theme || "system";
    else pref = localStorage.getItem("vulos-mail.theme") || "system"; // legacy key
  } catch { /* fall through to system */ }
  if (pref === "light" || pref === "dark") return pref;
  const dark = typeof matchMedia === "undefined" || matchMedia("(prefers-color-scheme: dark)").matches;
  return dark ? "dark" : "light";
}
function applyTheme(theme) {
  document.documentElement.setAttribute("data-theme", theme);
  const meta = document.querySelector('meta[name="theme-color"]');
  if (meta) meta.setAttribute("content", theme === "light" ? "#fbfbfa" : "#0c0c0c");
}
applyTheme(resolveTheme());

createRoot(document.getElementById("root")).render(
  <StrictMode>
    <App />
  </StrictMode>
);
