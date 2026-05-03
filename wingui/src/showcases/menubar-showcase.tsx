import {
	Menubar,
	MenubarMenu,
	MenubarTrigger,
	MenubarContent,
	MenubarItem,
	MenubarSeparator,
	MenubarLabel,
} from "@/components/core/menubar"

export function MenubarShowcase() {
	return (
		<section className="py-4 space-y-8">
			<h2 className="text-2xl font-semibold">Menubar</h2>
			<Menubar>
				<MenubarMenu>
					<MenubarTrigger>File</MenubarTrigger>
					<MenubarContent>
						<MenubarLabel>File Actions</MenubarLabel>
						<MenubarItem>New File</MenubarItem>
						<MenubarItem>Open...</MenubarItem>
						<MenubarSeparator />
						<MenubarItem>Save</MenubarItem>
						<MenubarItem>Save As...</MenubarItem>
					</MenubarContent>
				</MenubarMenu>
				<MenubarMenu>
					<MenubarTrigger>Edit</MenubarTrigger>
					<MenubarContent>
						<MenubarItem>Undo</MenubarItem>
						<MenubarItem>Redo</MenubarItem>
						<MenubarSeparator />
						<MenubarItem>Cut</MenubarItem>
						<MenubarItem>Copy</MenubarItem>
						<MenubarItem>Paste</MenubarItem>
					</MenubarContent>
				</MenubarMenu>
				<MenubarMenu>
					<MenubarTrigger>View</MenubarTrigger>
					<MenubarContent>
						<MenubarItem>Command Palette</MenubarItem>
						<MenubarSeparator />
						<MenubarItem>Zoom In</MenubarItem>
						<MenubarItem>Zoom Out</MenubarItem>
					</MenubarContent>
				</MenubarMenu>
			</Menubar>
		</section>
	)
}
