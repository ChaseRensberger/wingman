import * as React from "react"
import { formatHex, oklch, parse } from "culori"
import { ArrowCounterClockwiseIcon, PaletteIcon } from "@phosphor-icons/react"
import { cn } from "@/lib/utils"
import { Button } from "@/components/core/button"
import { useTheme } from "@/components/theme-provider"
import {
	Sheet,
	SheetContent,
	SheetHeader,
	SheetTitle,
	SheetTrigger,
	SheetFooter,
} from "@/components/core/sheet"

// ─── Types ────────────────────────────────────────────────────────────────────

type ColorVar = {
	name: string
	label: string
}

type OverrideMap = Record<string, string>

// ─── Constants ────────────────────────────────────────────────────────────────

const COLOR_VARS: ColorVar[] = [
	{ name: "--background", label: "Background" },
	{ name: "--foreground", label: "Foreground" },
	{ name: "--card", label: "Card" },
	{ name: "--card-foreground", label: "Card Foreground" },
	{ name: "--popover", label: "Popover" },
	{ name: "--popover-foreground", label: "Popover Foreground" },
	{ name: "--primary", label: "Primary" },
	{ name: "--primary-foreground", label: "Primary Foreground" },
	{ name: "--secondary", label: "Secondary" },
	{ name: "--secondary-foreground", label: "Secondary Foreground" },
	{ name: "--muted", label: "Muted" },
	{ name: "--muted-foreground", label: "Muted Foreground" },
	{ name: "--accent", label: "Accent" },
	{ name: "--accent-foreground", label: "Accent Foreground" },
	{ name: "--destructive", label: "Destructive" },
	{ name: "--border", label: "Border" },
	{ name: "--input", label: "Input" },
	{ name: "--ring", label: "Ring" },
]

const STORAGE_KEY = "wingui-theme-overrides"
const FONT_LINK_ID = "wingui-google-font"

// ─── Font Utils ───────────────────────────────────────────────────────────────

/**
 * Parses a Google Fonts URL (v1 or v2) and returns:
 * - `linkHref`: the URL to inject as a <link> stylesheet
 * - `familyName`: the CSS font-family name (e.g. "Inter")
 *
 * Supports:
 *   v1: https://fonts.googleapis.com/css?family=Roboto:300,400,500
 *   v2: https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600&display=swap
 */
function parseGoogleFontUrl(url: string): { linkHref: string; familyName: string } | null {
	try {
		const parsed = new URL(url.trim())

		// Handle specimen/preview page URLs: fonts.google.com/specimen/Golos+Text
		if (parsed.hostname === "fonts.google.com") {
			const match = parsed.pathname.match(/\/specimen\/([^/]+)/)
			if (!match) return null
			const familyName = decodeURIComponent(match[1]).replace(/\+/g, " ").trim()
			const apiFamily = familyName.replace(/ /g, "+")
			const linkHref = `https://fonts.googleapis.com/css2?family=${apiFamily}:wght@400;500;700&display=swap`
			return { linkHref, familyName }
		}

		// Handle embed URLs: fonts.googleapis.com/css or /css2
		if (parsed.hostname === "fonts.googleapis.com") {
			const family = parsed.searchParams.get("family")
			if (!family) return null

			// Family name is the part before ":" or "|" (v1 can have multiple families)
			const familyName = family.split("|")[0].split(":")[0].replace(/\+/g, " ").trim()
			if (!familyName) return null

			// Ensure display=swap is present for better perf
			if (!parsed.searchParams.has("display")) {
				parsed.searchParams.set("display", "swap")
			}

			return { linkHref: parsed.toString(), familyName }
		}

		return null
	} catch {
		return null
	}
}

function injectFontLink(href: string, familyName: string) {
	removeFontLink()
	const link = document.createElement("link")
	link.id = FONT_LINK_ID
	link.rel = "stylesheet"
	link.href = href
	document.head.appendChild(link)

	const style = document.createElement("style")
	style.id = FONT_LINK_ID + "-override"
	style.textContent = `* { font-family: "${familyName}", sans-serif !important; }`
	document.head.appendChild(style)
}

function removeFontLink() {
	document.getElementById(FONT_LINK_ID)?.remove()
	document.getElementById(FONT_LINK_ID + "-override")?.remove()
}

// ─── Color Utils ──────────────────────────────────────────────────────────────

function oklchStringToHex(value: string): string {
	try {
		const parsed = parse(value)
		if (!parsed) return "#888888"
		return formatHex(parsed) ?? "#888888"
	} catch {
		return "#888888"
	}
}

function hexToOklchString(hex: string): string {
	try {
		const color = oklch(parse(hex))
		if (!color) return "oklch(0.5 0 0)"
		const l = Math.round(color.l * 10000) / 10000
		const c = Math.round((color.c ?? 0) * 10000) / 10000
		const h = Math.round((color.h ?? 0) * 100) / 100
		return `oklch(${l} ${c} ${h})`
	} catch {
		return "oklch(0.5 0 0)"
	}
}

// ─── Override Persistence ─────────────────────────────────────────────────────

export function loadOverrides(): OverrideMap {
	try {
		const raw = localStorage.getItem(STORAGE_KEY)
		if (raw) return JSON.parse(raw)
	} catch {}
	return {}
}

export function saveOverrides(overrides: OverrideMap) {
	localStorage.setItem(STORAGE_KEY, JSON.stringify(overrides))
}

export function applyOverrides(overrides: OverrideMap) {
	for (const [name, value] of Object.entries(overrides)) {
		if (name === "--google-font-url") {
			const result = parseGoogleFontUrl(value)
			if (result) {
				injectFontLink(result.linkHref, result.familyName)
				document.documentElement.style.setProperty("--font-family-sans", `"${result.familyName}", ui-sans-serif, system-ui, sans-serif`)
				document.documentElement.style.setProperty("--font-family-mono", `"${result.familyName}", ui-monospace, monospace`)
			}
		} else {
			document.documentElement.style.setProperty(name, value)
		}
	}
}

export function clearAllOverrides(overrides: OverrideMap) {
	for (const name of Object.keys(overrides)) {
		if (name === "--google-font-url") {
			removeFontLink()
			document.documentElement.style.removeProperty("--font-family-sans")
			document.documentElement.style.removeProperty("--font-family-mono")
		} else {
			document.documentElement.style.removeProperty(name)
		}
	}
	window.dispatchEvent(new CustomEvent("wingui-overrides-cleared"))
}

function getComputedVar(name: string): string {
	return getComputedStyle(document.documentElement).getPropertyValue(name).trim()
}

// ─── Color Row ────────────────────────────────────────────────────────────────

type ColorRowProps = {
	variable: ColorVar
	overrides: OverrideMap
	onColorChange: (name: string, oklchValue: string) => void
}

function ColorRow({ variable, overrides, onColorChange }: ColorRowProps) {
	const currentOklch = overrides[variable.name] ?? getComputedVar(variable.name)
	const hexValue = oklchStringToHex(currentOklch)
	const isOverridden = variable.name in overrides

	return (
		<div className="flex items-center justify-between gap-2 py-1.5">
			<span className={cn("text-xs flex-1 truncate", isOverridden ? "text-primary font-medium" : "text-muted-foreground")}>
				{variable.label}
			</span>
			<div className="flex items-center gap-2">
				<span className="text-xs text-muted-foreground font-mono w-16 text-right truncate hidden sm:block" title={hexValue}>
					{hexValue}
				</span>
				<input
					type="color"
					value={hexValue}
					onChange={(e) => onColorChange(variable.name, hexToOklchString(e.target.value))}
					className="w-7 h-7 rounded cursor-pointer border border-border bg-transparent p-0.5 [&::-webkit-color-swatch-wrapper]:p-0 [&::-webkit-color-swatch]:rounded-sm [&::-webkit-color-swatch]:border-none"
					title={`${variable.name}: ${currentOklch}`}
				/>
			</div>
		</div>
	)
}

// ─── Font Input ───────────────────────────────────────────────────────────────

type FontInputProps = {
	label: string
	description: string
	storageKey: string
	currentUrl: string
	onApply: (url: string, familyName: string, cssVar: string) => void
	onClear: () => void
	isActive: boolean
	activeFamilyName: string
}

function FontInput({ label, description, storageKey: _sk, currentUrl, onApply, onClear, isActive, activeFamilyName }: FontInputProps) {
	const [inputValue, setInputValue] = React.useState(currentUrl)
	const [error, setError] = React.useState<string | null>(null)

	React.useEffect(() => {
		setInputValue(currentUrl)
	}, [currentUrl])

	function handleApply() {
		if (!inputValue.trim()) {
			onClear()
			setError(null)
			return
		}
		const result = parseGoogleFontUrl(inputValue)
		if (!result) {
			setError("Invalid Google Fonts URL")
			return
		}
		setError(null)
		const cssVar = label === "Mono Font" ? "--font-family-mono" : "--font-family-sans"
		onApply(inputValue.trim(), result.familyName, cssVar)
	}

	function handleKeyDown(e: React.KeyboardEvent) {
		if (e.key === "Enter") handleApply()
		if (e.key === "Escape") {
			setInputValue(currentUrl)
			setError(null)
		}
	}

	return (
		<div className="space-y-1.5">
			<div className="flex items-center justify-between">
				<span className={cn("text-xs", isActive ? "text-primary font-medium" : "text-muted-foreground")}>
					{label}
					{isActive && <span className="ml-1.5 text-xs font-normal normal-case">({activeFamilyName})</span>}
				</span>
				{isActive && (
					<button
						onClick={onClear}
						className="text-xs text-muted-foreground hover:text-foreground transition-colors"
					>
						clear
					</button>
				)}
			</div>
			<p className="text-xs text-muted-foreground/70">{description}</p>
			<div className="flex gap-1.5">
				<input
					type="url"
					value={inputValue}
					onChange={(e) => { setInputValue(e.target.value); setError(null) }}
					onKeyDown={handleKeyDown}
					placeholder="https://fonts.google.com/specimen/Inter"
					className={cn(
						"flex-1 min-w-0 text-xs px-2 py-1.5 rounded border bg-background text-foreground placeholder:text-muted-foreground/50 outline-none transition-colors",
						error ? "border-destructive focus:border-destructive" : "border-border focus:border-primary"
					)}
				/>
				<button
					onClick={handleApply}
					className="shrink-0 text-xs px-2.5 py-1.5 rounded border border-border hover:border-primary hover:text-primary transition-colors"
				>
					Apply
				</button>
			</div>
			{error && <p className="text-xs text-destructive">{error}</p>}
		</div>
	)
}

// ─── Main Component ───────────────────────────────────────────────────────────

export function ThemeCustomizer() {
	const { theme } = useTheme()
	const [overrides, setOverrides] = React.useState<OverrideMap>(() => loadOverrides())
	const [radius, setRadius] = React.useState<string>(() => loadOverrides()["--radius"] ?? "")

	// Apply persisted overrides on mount
	React.useEffect(() => {
		applyOverrides(overrides)
	}, [])

	// Sync state when overrides are cleared externally (e.g. theme switch)
	React.useEffect(() => {
		function handleStorage(e: StorageEvent) {
			if (e.key === STORAGE_KEY && e.newValue === null) {
				setOverrides({})
				setRadius("")
			}
		}
		function handleCleared() {
			setOverrides({})
			setRadius("")
		}
		window.addEventListener("storage", handleStorage)
		window.addEventListener("wingui-overrides-cleared", handleCleared)
		return () => {
			window.removeEventListener("storage", handleStorage)
			window.removeEventListener("wingui-overrides-cleared", handleCleared)
		}
	}, [])

	const isDark = theme === "dark" || (theme === "system" && window.matchMedia("(prefers-color-scheme: dark)").matches)
	const overrideCount = Object.keys(overrides).length

	function handleColorChange(name: string, oklchValue: string) {
		const next = { ...overrides, [name]: oklchValue }
		setOverrides(next)
		saveOverrides(next)
		document.documentElement.style.setProperty(name, oklchValue)
	}

	function handleRadiusChange(value: string) {
		const rem = `${value}rem`
		setRadius(rem)
		const next = { ...overrides, "--radius": rem }
		setOverrides(next)
		saveOverrides(next)
		document.documentElement.style.setProperty("--radius", rem)
	}

	function handleFontApply(storageUrlKey: string, url: string, familyName: string) {
		injectFontLink(url, familyName)
		document.documentElement.style.setProperty("--font-family-sans", `"${familyName}", ui-sans-serif, system-ui, sans-serif`)
		document.documentElement.style.setProperty("--font-family-mono", `"${familyName}", ui-monospace, monospace`)
		const next = { ...overrides, [storageUrlKey]: url }
		setOverrides(next)
		saveOverrides(next)
	}

	function handleFontClear(storageUrlKey: string) {
		removeFontLink()
		document.documentElement.style.removeProperty("--font-family-sans")
		document.documentElement.style.removeProperty("--font-family-mono")
		const next = { ...overrides }
		delete next[storageUrlKey]
		setOverrides(next)
		saveOverrides(next)
	}

	function handleReset() {
		clearAllOverrides(overrides)
		setOverrides({})
		setRadius("")
		localStorage.removeItem(STORAGE_KEY)
	}

	const currentRadius = radius || getComputedVar("--radius") || "0.625rem"
	const radiusNum = parseFloat(currentRadius)

	const sansFontUrl = overrides["--google-font-url"] ?? ""
	const sansFamilyName = sansFontUrl ? (parseGoogleFontUrl(sansFontUrl)?.familyName ?? "") : ""

	return (
		<Sheet>
			<SheetTrigger render={
				<Button variant="outline" className="relative w-8 h-8" aria-label="Customize theme">
					<PaletteIcon />
					{overrideCount > 0 && (
						<span className="absolute -top-1 -right-1 w-3.5 h-3.5 rounded-full bg-primary text-[8px] text-primary-foreground flex items-center justify-center font-bold leading-none">
							{overrideCount > 9 ? "9+" : overrideCount}
						</span>
					)}
				</Button>
			} />
			<SheetContent side="right" className="w-80 sm:max-w-80 p-0 flex flex-col gap-0">
				<SheetHeader className="px-4 py-3 border-b shrink-0">
					<div className="flex items-center gap-2">
						<PaletteIcon className="w-4 h-4 text-primary" />
						<SheetTitle className="text-sm">Theme Customizer</SheetTitle>
					</div>
				</SheetHeader>

				{/* Scrollable body */}
				<div className="flex-1 overflow-y-auto px-4 py-4 space-y-6">
					{/* Radius */}
					<section>
						<h3 className="text-xs font-semibold uppercase tracking-wide mb-3">Border Radius</h3>
						<input
							type="range"
							min="0"
							max="1.5"
							step="0.025"
							value={radiusNum}
							onChange={(e) => handleRadiusChange(e.target.value)}
							className="w-full h-2 accent-primary cursor-pointer mb-2"
						/>
						<div className="flex gap-1">
							{["0", "0.3", "0.625", "1", "1.5"].map((val) => (
								<button
									key={val}
									onClick={() => handleRadiusChange(val)}
									className={cn(
										"flex-1 text-xs py-1 rounded border transition-colors",
										radiusNum === parseFloat(val)
											? "border-primary bg-primary/10 text-primary"
											: "border-border text-muted-foreground hover:border-foreground/30"
									)}
								>
									{val}
								</button>
							))}
						</div>
					</section>

					{/* Fonts */}
					<section>
						<h3 className="text-xs font-semibold uppercase tracking-wide mb-3">Fonts</h3>
						<p className="text-xs text-muted-foreground mb-3">
							Paste a{" "}
							<a
								href="https://fonts.google.com"
								target="_blank"
								rel="noopener noreferrer"
								className="text-primary underline underline-offset-2"
							>
								Google Fonts
							</a>
							{" "}URL — specimen page or embed link both work.
						</p>
								<FontInput
							label="Font"
							description="Applied to all text across the site."
							storageKey="--google-font-url"
							currentUrl={sansFontUrl}
							isActive={!!sansFontUrl}
							activeFamilyName={sansFamilyName}
							onApply={(url, familyName) => handleFontApply("--google-font-url", url, familyName)}
							onClear={() => handleFontClear("--google-font-url")}
						/>
					</section>

					{/* Colors */}
					<section>
						<h3 className="text-xs font-semibold uppercase tracking-wide mb-1">
							Colors <span className="text-muted-foreground font-normal normal-case">({isDark ? "dark" : "light"})</span>
						</h3>
						<p className="text-xs text-muted-foreground mb-2">Editing active theme mode.</p>
						<div className="divide-y divide-border/50">
							{COLOR_VARS.map((v) => (
								<ColorRow
									key={v.name}
									variable={v}
									overrides={overrides}
									onColorChange={handleColorChange}
								/>
							))}
						</div>
					</section>
				</div>

				{/* Footer */}
				{overrideCount > 0 && (
					<SheetFooter className="px-4 py-3 border-t shrink-0">
						<div className="w-full">
							<p className="text-xs text-muted-foreground mb-2">
								{overrideCount} override{overrideCount !== 1 ? "s" : ""} active
							</p>
							<Button variant="outline" className="w-full h-8 text-xs" onClick={handleReset}>
								<ArrowCounterClockwiseIcon className="w-3 h-3 mr-1.5" />
								Reset to defaults
							</Button>
						</div>
					</SheetFooter>
				)}
			</SheetContent>
		</Sheet>
	)
}
