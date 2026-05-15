import { createContext, useContext, useEffect, useState } from "react"

export type Theme = "dark" | "light" | "system"

type ThemeProviderProps = {
	children: React.ReactNode
	defaultTheme?: Theme
	storageKey?: string
}

type ThemeProviderState = {
	theme: Theme
	setTheme: (theme: Theme) => void
}

const lightModeDisabled = true

const resolveTheme = (theme: Theme): Exclude<Theme, "light"> => {
	if (theme === "light") return "dark"

	return theme
}

const initialState: ThemeProviderState = {
	theme: "system",
	setTheme: () => null,
}

const ThemeProviderContext = createContext<ThemeProviderState>(initialState)

export function ThemeProvider({
	children,
	defaultTheme = "system",
	storageKey = "vite-ui-theme",
	...props
}: ThemeProviderProps) {
	const [theme, setTheme] = useState<Theme>(
		() => resolveTheme((localStorage.getItem(storageKey) as Theme) || defaultTheme)
	)

	useEffect(() => {
		const root = window.document.documentElement
		const activeTheme = resolveTheme(theme)

		root.classList.remove("light", "dark")

		if (activeTheme === "system") {
			const systemTheme = window.matchMedia("(prefers-color-scheme: dark)")
				.matches
				? "dark"
				: lightModeDisabled
					? "dark"
					: "light"

			root.classList.add(systemTheme)
			return
		}

		root.classList.add(activeTheme)
	}, [theme])

	const value = {
		theme,
		setTheme: (newTheme: Theme) => {
			const nextTheme = resolveTheme(newTheme)

			localStorage.removeItem("ui-theme-overrides")
			localStorage.setItem(storageKey, nextTheme)
			setTheme(nextTheme)
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
