import * as React from "react"
import { MagnifyingGlassIcon } from "@phosphor-icons/react"
import { cn } from "#lib/utils"

function Command({ className, ...props }: React.ComponentProps<"div">) {
	return (
		<div
			data-slot="command"
			className={cn(
				"flex flex-col overflow-hidden rounded-md border bg-popover text-popover-foreground shadow-sm",
				className
			)}
			{...props}
		/>
	)
}

function CommandInput({ className, ...props }: React.ComponentProps<"input">) {
	return (
		<div className="flex items-center gap-2 border-b px-2" data-slot="command-input-wrapper">
			<MagnifyingGlassIcon className="size-4 shrink-0 text-muted-foreground" />
			<input
				data-slot="command-input"
				className={cn(
					"flex h-8 w-full bg-transparent py-2 text-sm outline-none placeholder:text-muted-foreground focus-visible:ring-3 focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50",
					className
				)}
				{...props}
			/>
		</div>
	)
}

function CommandList({ className, ...props }: React.ComponentProps<"div">) {
	return (
		<div
			data-slot="command-list"
			className={cn("max-h-64 overflow-y-auto overflow-x-hidden", className)}
			{...props}
		/>
	)
}

function CommandEmpty({ className, ...props }: React.ComponentProps<"div">) {
	return (
		<div
			data-slot="command-empty"
			className={cn("py-6 text-center text-sm text-muted-foreground", className)}
			{...props}
		/>
	)
}

function CommandGroup({ className, ...props }: React.ComponentProps<"div">) {
	return (
		<div
			data-slot="command-group"
			className={cn("overflow-hidden p-1", className)}
			{...props}
		/>
	)
}

function CommandGroupHeading({ className, ...props }: React.ComponentProps<"div">) {
	return (
		<div
			data-slot="command-group-heading"
			className={cn("px-2 py-1.5 text-xs font-medium text-muted-foreground", className)}
			{...props}
		/>
	)
}

function CommandItem({ className, ...props }: React.ComponentProps<"div">) {
	return (
		<div
			data-slot="command-item"
			role="option"
			className={cn(
				"relative flex cursor-pointer select-none items-center gap-2 rounded-sm px-2 py-1 text-sm outline-none transition-[color,background-color,box-shadow] duration-150 hover:bg-accent hover:text-accent-foreground aria-selected:bg-accent aria-selected:text-accent-foreground data-disabled:pointer-events-none data-disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
				className
			)}
			{...props}
		/>
	)
}

function CommandSeparator({ className, ...props }: React.ComponentProps<"div">) {
	return (
		<div
			data-slot="command-separator"
			className={cn("-mx-1 h-px bg-border", className)}
			{...props}
		/>
	)
}

function CommandShortcut({ className, ...props }: React.ComponentProps<"span">) {
	return (
		<span
			data-slot="command-shortcut"
			className={cn("ml-auto text-xs tracking-widest text-muted-foreground", className)}
			{...props}
		/>
	)
}

export {
	Command,
	CommandInput,
	CommandList,
	CommandEmpty,
	CommandGroup,
	CommandGroupHeading,
	CommandItem,
	CommandSeparator,
	CommandShortcut,
}
