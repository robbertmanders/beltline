import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BeltlineApp } from "./app/App";
import { applyStoredAppearance } from "./state/theme";
import "./theme/tokens.css";
import "./theme/fields.css";
import "./theme/shell.css";
import "./components/components.css";
import "./pages/run/run.css";
import "./components/assistantSidebar.css";
import "./theme/belts.css";
import "./theme/shadow.css";

const root = document.getElementById("root");
if (!root) throw new Error("The app root is missing.");

// Before the first render, so a stored Dark choice never shows the loading, sign-in or offline states in Light.
applyStoredAppearance();
createRoot(root).render(<StrictMode><BeltlineApp /></StrictMode>);
