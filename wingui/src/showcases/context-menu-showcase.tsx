import {
	ContextMenu,
	ContextMenuTrigger,
	ContextMenuContent,
	ContextMenuItem,
	ContextMenuSeparator,
	ContextMenuLabel,
} from "@/components/core/context-menu"

export function ContextMenuShowcase() {
	return (
		<section className="py-4 space-y-8">
			<h2 className="text-2xl font-semibold">Context Menu</h2>
			<ContextMenu>
				<ContextMenuTrigger>
					<div className="flex h-32 w-64 items-center justify-center rounded-lg border border-dashed border-border text-sm text-muted-foreground select-none">
						Right-click here
					</div>
				</ContextMenuTrigger>
				<ContextMenuContent>
					<ContextMenuLabel>Actions</ContextMenuLabel>
					<ContextMenuItem>Open</ContextMenuItem>
					<ContextMenuItem>Copy link</ContextMenuItem>
					<ContextMenuSeparator />
					<ContextMenuItem>Share</ContextMenuItem>
					<ContextMenuSeparator />
					<ContextMenuItem variant="destructive">Delete</ContextMenuItem>
				</ContextMenuContent>
			</ContextMenu>
		</section>
	)
}
