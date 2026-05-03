import { useState } from "react"
import {
	Command,
	CommandInput,
	CommandList,
	CommandEmpty,
	CommandGroup,
	CommandGroupHeading,
	CommandItem,
	CommandSeparator,
	CommandShortcut,
} from "@/components/core/command"
import {
	FileIcon,
	GearIcon,
	UserIcon,
	CalendarIcon,
	CodeIcon,
} from "@phosphor-icons/react"

export function CommandShowcase() {
	const [search, setSearch] = useState("")

	const suggestions = [
		{
			group: "Suggestions",
			items: [
				{ icon: CalendarIcon, label: "Calendar", shortcut: "⌘C" },
				{ icon: UserIcon, label: "Search Users", shortcut: "⌘U" },
				{ icon: CodeIcon, label: "Open Editor", shortcut: "⌘E" },
			],
		},
		{
			group: "Settings",
			items: [
				{ icon: GearIcon, label: "Preferences", shortcut: "⌘," },
				{ icon: FileIcon, label: "Profile", shortcut: "⌘P" },
			],
		},
	]

	const filtered = suggestions
		.map((g) => ({
			...g,
			items: g.items.filter((i) =>
				i.label.toLowerCase().includes(search.toLowerCase())
			),
		}))
		.filter((g) => g.items.length > 0)

	return (
		<section className="py-4 space-y-8">
			<h2 className="text-2xl font-semibold">Command</h2>
			<Command className="max-w-sm">
				<CommandInput
					placeholder="Type a command or search..."
					value={search}
					onChange={(e) => setSearch(e.target.value)}
				/>
				<CommandList>
					{filtered.length === 0 && <CommandEmpty>No results found.</CommandEmpty>}
					{filtered.map((group, i) => (
						<div key={group.group}>
							{i > 0 && <CommandSeparator />}
							<CommandGroup>
								<CommandGroupHeading>{group.group}</CommandGroupHeading>
								{group.items.map((item) => (
									<CommandItem key={item.label}>
										<item.icon />
										{item.label}
										<CommandShortcut>{item.shortcut}</CommandShortcut>
									</CommandItem>
								))}
							</CommandGroup>
						</div>
					))}
				</CommandList>
			</Command>
		</section>
	)
}
