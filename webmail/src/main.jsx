import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./styles.css";
import App from "./App.jsx";
import { ToastProvider } from "./components/Toasts.jsx";

// Apply the saved theme before first paint (parity with the vanilla SPA).
function applyTheme(t) {
  const theme = t === "light" ? "light" : "dark";
  document.documentElement.setAttribute("data-theme", theme);
  const meta = document.querySelector('meta[name="theme-color"]');
  if (meta) meta.setAttribute("content", theme === "light" ? "#fbfbfa" : "#0c0c0c");
}
try { applyTheme(localStorage.getItem("vulos-mail.theme") || "dark"); } catch { applyTheme("dark"); }

createRoot(document.getElementById("root")).render(
  <StrictMode>
    <ToastProvider>
      <App />
    </ToastProvider>
  </StrictMode>
);
