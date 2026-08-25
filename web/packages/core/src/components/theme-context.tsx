import { createContext, useContext } from "react";

import {
  getTheme,
  type ColorMode,
  type ResolvedColorMode,
  type Theme,
  type ThemeID,
} from "#themes/registry";

type ThemeProviderState = {
  theme: Theme;
  colorMode: ColorMode;
  resolvedColorMode: ResolvedColorMode;
  setTheme: (theme: ThemeID, colorMode?: ColorMode) => void;
  setColorMode: (mode: ColorMode) => void;
};

const initialState: ThemeProviderState = {
  theme: getTheme("default"),
  colorMode: "system",
  resolvedColorMode: "light",
  setTheme: () => null,
  setColorMode: () => null,
};

export const ThemeProviderContext = createContext<ThemeProviderState>(initialState);

export function useTheme() {
  const context = useContext(ThemeProviderContext);

  if (context === undefined) throw new Error("useTheme must be used within a ThemeProvider");

  return context;
}
