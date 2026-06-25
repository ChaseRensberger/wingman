import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { ThemeProvider } from "@wingman/core/components/theme-provider"
import "./styles/globals.css"
import { App } from "./App"

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ThemeProvider defaultTheme="system" storageKey="wingman-home-theme">
      <App />
    </ThemeProvider>
  </StrictMode>
)
