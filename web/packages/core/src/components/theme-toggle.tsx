import { MoonIcon, SunIcon } from "@phosphor-icons/react"
import { Button } from "#components/core/button"
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#components/core/dropdown-menu"
import { useTheme } from "#components/theme-provider"

export function ThemeToggle() {
	const { setColorMode } = useTheme()

	return (
		<DropdownMenu>
			<DropdownMenuTrigger render={<Button variant="outline" size="icon" />}>
				<SunIcon className="size-4 dark:hidden" />
				<MoonIcon className="hidden size-4 dark:block" />
				<span className="sr-only">Toggle theme</span>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="end">
				<DropdownMenuItem onClick={() => setColorMode("light")}>
					Light
				</DropdownMenuItem>
				<DropdownMenuItem onClick={() => setColorMode("dark")}>
					Dark
				</DropdownMenuItem>
				<DropdownMenuItem onClick={() => setColorMode("system")}>
					System
				</DropdownMenuItem>
			</DropdownMenuContent>
		</DropdownMenu>
	)
}
