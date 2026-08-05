import { createContext, useContext, useEffect, useState } from "react"
import {
	getTheme,
	normalizeColorMode,
	type ColorMode,
	type ResolvedColorMode,
	type Theme,
	type ThemeID,
} from "#themes/registry"

export type { ColorMode, ResolvedColorMode, Theme, ThemeID } from "#themes/registry"

type ThemeProviderProps = {
	children: React.ReactNode
	defaultTheme?: ThemeID
	defaultColorMode?: ColorMode
	storageKey?: string
}

type ThemeProviderState = {
	theme: Theme
	colorMode: ColorMode
	resolvedColorMode: ResolvedColorMode
	setTheme: (theme: ThemeID, colorMode?: ColorMode) => void
	setColorMode: (mode: ColorMode) => void
}

const initialState: ThemeProviderState = {
	theme: getTheme("default"),
	colorMode: "system",
	resolvedColorMode: "light",
	setTheme: () => null,
	setColorMode: () => null,
}

const ThemeProviderContext = createContext<ThemeProviderState>(initialState)

export function ThemeProvider({
	children,
	defaultTheme = "default",
	defaultColorMode = "system",
	storageKey = "wingman-theme",
	...props
}: ThemeProviderProps) {
	const [theme, setThemeState] = useState(() => getTheme(localStorage.getItem(storageKey) ?? defaultTheme))
	const [colorMode, setColorModeState] = useState<ColorMode>(() => normalizeColorMode(theme, (localStorage.getItem(`${storageKey}-mode`) as ColorMode) || defaultColorMode))
	const [systemColorMode, setSystemColorMode] = useState<ResolvedColorMode>(() => window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light")
	const preferredColorMode = colorMode === "system" ? systemColorMode : colorMode
	const resolvedColorMode = theme.modes.includes(preferredColorMode) ? preferredColorMode : theme.modes[0]

	useEffect(() => {
		if (colorMode !== "system") return
		const media = window.matchMedia("(prefers-color-scheme: dark)")
		const updateSystemColorMode = () => setSystemColorMode(media.matches ? "dark" : "light")
		updateSystemColorMode()
		media.addEventListener("change", updateSystemColorMode)
		return () => media.removeEventListener("change", updateSystemColorMode)
	}, [colorMode])

	useEffect(() => {
		const root = window.document.documentElement
		root.dataset.theme = theme.id
		root.dataset.mode = resolvedColorMode
		root.classList.toggle("dark", resolvedColorMode === "dark")
		root.classList.toggle("light", resolvedColorMode === "light")
	}, [resolvedColorMode, theme])

	const value = {
		theme,
		colorMode,
		resolvedColorMode,
		setTheme: (id: ThemeID, mode?: ColorMode) => {
			const nextTheme = getTheme(id)
			const nextColorMode = normalizeColorMode(nextTheme, mode ?? colorMode)
			localStorage.setItem(storageKey, nextTheme.id)
			localStorage.setItem(`${storageKey}-mode`, nextColorMode)
			setThemeState(nextTheme)
			setColorModeState(nextColorMode)
		},
		setColorMode: (mode: ColorMode) => {
			const nextColorMode = normalizeColorMode(theme, mode)
			localStorage.setItem(`${storageKey}-mode`, nextColorMode)
			setColorModeState(nextColorMode)
		},
	}

	return (
		<ThemeProviderContext.Provider {...props} value={value}>
			{children}
		</ThemeProviderContext.Provider>
	)
}

export const useTheme = () => {
	const context = useContext(ThemeProviderContext)

	if (context === undefined)
		throw new Error("useTheme must be used within a ThemeProvider")

	return context
}
