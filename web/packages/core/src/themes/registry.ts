export type ThemeID = "default" | "gruvbox" | "dracula" | "nord" | "rose-pine";
export type ColorMode = "light" | "dark" | "system";
export type ResolvedColorMode = Exclude<ColorMode, "system">;

export type Theme = {
  id: ThemeID;
  label: string;
  modes: readonly ResolvedColorMode[];
  shiki: Partial<Record<ResolvedColorMode, string>>;
};

export const themes: readonly Theme[] = [
  {
    id: "default",
    label: "WingTheme",
    modes: ["light", "dark"],
    shiki: { light: "github-light", dark: "github-dark" },
  },
  {
    id: "gruvbox",
    label: "Gruvbox",
    modes: ["light", "dark"],
    shiki: { light: "gruvbox-light-medium", dark: "gruvbox-dark-medium" },
  },
  {
    id: "dracula",
    label: "Dracula",
    modes: ["dark"],
    shiki: { dark: "dracula" },
  },
  {
    id: "nord",
    label: "Nord",
    modes: ["light", "dark"],
    shiki: { light: "nord", dark: "nord" },
  },
  {
    id: "rose-pine",
    label: "Rosé Pine",
    modes: ["light", "dark"],
    shiki: { light: "rose-pine-dawn", dark: "rose-pine" },
  },
] as const;

export function getTheme(id: string | null | undefined): Theme {
  return themes.find((theme) => theme.id === id) ?? themes[0];
}

export function supportsColorMode(theme: Theme, mode: ColorMode) {
  return mode === "system" || theme.modes.includes(mode);
}

export function normalizeColorMode(_theme: Theme, mode: ColorMode): ColorMode {
  return mode;
}
