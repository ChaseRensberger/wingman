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

// ─── Utils ────────────────────────────────────────────────────────────────────

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

function loadOverrides(): OverrideMap {
	try {
		const raw = localStorage.getItem(STORAGE_KEY)
		if (raw) return JSON.parse(raw)
	} catch {}
	return {}
}

function saveOverrides(overrides: OverrideMap) {
	localStorage.setItem(STORAGE_KEY, JSON.stringify(overrides))
}

function applyOverrides(overrides: OverrideMap) {
	for (const [name, value] of Object.entries(overrides)) {
		document.documentElement.style.setProperty(name, value)
	}
}

function clearOverrides(overrides: OverrideMap) {
	for (const name of Object.keys(overrides)) {
		document.documentElement.style.removeProperty(name)
	}
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

// ─── Main Component ───────────────────────────────────────────────────────────

export function ThemeCustomizer() {
	const { theme } = useTheme()
	const [overrides, setOverrides] = React.useState<OverrideMap>(() => loadOverrides())
	const [radius, setRadius] = React.useState<string>(() => loadOverrides()["--radius"] ?? "")

	// Apply persisted overrides on mount
	React.useEffect(() => {
		applyOverrides(overrides)
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

	function handleReset() {
		clearOverrides(overrides)
		setOverrides({})
		setRadius("")
		localStorage.removeItem(STORAGE_KEY)
	}

	const currentRadius = radius || getComputedVar("--radius") || "0.625rem"
	const radiusNum = parseFloat(currentRadius)

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
