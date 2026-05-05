import { MoonIcon, SunIcon } from "@phosphor-icons/react"
import { Button } from "@/components/core/button"
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "@/components/core/dropdown-menu"
import { useTheme } from "@/components/theme-provider"

export function ThemeToggle() {
	const { setTheme } = useTheme()

	return (
		<DropdownMenu>
			<DropdownMenuTrigger render={<Button variant="outline" size="icon" />}>
				<SunIcon className="h-8 w-8 scale-100 rotate-0 transition-all dark:scale-0 dark:-rotate-90" />
				<MoonIcon className="absolute h-8 w-8 scale-0 rotate-90 transition-all dark:scale-100 dark:rotate-0" />
				<span className="sr-only">Toggle theme</span>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="end">
				<DropdownMenuItem onClick={() => setTheme("light")}>
					Light
				</DropdownMenuItem>
				<DropdownMenuItem onClick={() => setTheme("dark")}>
					Dark
				</DropdownMenuItem>
				<DropdownMenuItem onClick={() => setTheme("system")}>
					System
				</DropdownMenuItem>
			</DropdownMenuContent>
		</DropdownMenu>
	)
}
