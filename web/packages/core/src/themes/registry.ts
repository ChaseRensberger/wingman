export type ThemeID = "default" | "gruvbox" | "dracula";
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
    label: "Default",
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
] as const;

export function getTheme(id: string | null | undefined): Theme {
  return themes.find((theme) => theme.id === id) ?? themes[0];
}

export function supportsColorMode(theme: Theme, mode: ColorMode) {
  return mode === "system" ? theme.modes.length === 2 : theme.modes.includes(mode);
}

export function normalizeColorMode(theme: Theme, mode: ColorMode): ColorMode {
  return supportsColorMode(theme, mode) ? mode : theme.modes[0];
}
