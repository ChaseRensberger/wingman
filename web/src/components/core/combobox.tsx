import * as React from "react"
import { Combobox as ComboboxPrimitive } from "@base-ui/react/combobox"
import { CheckIcon } from "@phosphor-icons/react"
import { cn } from "@/lib/utils"

function Combobox<Value = string>({ ...props }: ComboboxPrimitive.Root.Props<Value>) {
	return <ComboboxPrimitive.Root data-slot="combobox" {...props} />
}

function ComboboxInputGroup({ className, ...props }: ComboboxPrimitive.InputGroup.Props) {
	return (
		<ComboboxPrimitive.InputGroup
			data-slot="combobox-input-group"
			className={cn("relative flex w-full items-center", className)}
			{...props}
		/>
	)
}

function ComboboxInput({ className, ...props }: ComboboxPrimitive.Input.Props) {
	return (
		<ComboboxPrimitive.Input
			data-slot="combobox-input"
			className={cn(
				"flex h-8 w-full rounded-lg border border-input bg-background px-2.5 py-1.5 pr-8 text-sm shadow-xs transition-[color,box-shadow] outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50",
				className
			)}
			{...props}
		/>
	)
}

function ComboboxTrigger({ className, ...props }: ComboboxPrimitive.Trigger.Props) {
	return (
		<ComboboxPrimitive.Trigger
			data-slot="combobox-trigger"
			className={cn(
				"absolute right-0 top-0 flex h-full items-center pr-2.5 text-muted-foreground",
				className
			)}
			{...props}
		/>
	)
}

function ComboboxPopup({ className, ...props }: ComboboxPrimitive.Popup.Props) {
	return (
		<ComboboxPrimitive.Portal>
			<ComboboxPrimitive.Positioner sideOffset={4} className="isolate z-50 outline-none">
				<ComboboxPrimitive.Popup
					data-slot="combobox-popup"
					className={cn(
						"z-50 max-h-60 w-(--anchor-width) min-w-32 overflow-y-auto rounded-lg bg-popover p-1 text-popover-foreground shadow-md ring-1 ring-foreground/10 outline-none data-open:animate-in data-open:fade-in-0 data-open:zoom-in-95 data-closed:animate-out data-closed:fade-out-0 data-closed:zoom-out-95",
						className
					)}
					{...props}
				/>
			</ComboboxPrimitive.Positioner>
		</ComboboxPrimitive.Portal>
	)
}

function ComboboxItem({ className, ...props }: ComboboxPrimitive.Item.Props) {
	return (
		<ComboboxPrimitive.Item
			data-slot="combobox-item"
			className={cn(
				"relative flex cursor-default items-center gap-1.5 rounded-md px-1.5 py-1 pl-7 text-sm outline-hidden select-none focus:bg-accent focus:text-accent-foreground data-disabled:pointer-events-none data-disabled:opacity-50 data-selected:font-medium",
				className
			)}
			{...props}
		/>
	)
}

function ComboboxItemIndicator({ className, ...props }: ComboboxPrimitive.ItemIndicator.Props) {
	return (
		<ComboboxPrimitive.ItemIndicator
			data-slot="combobox-item-indicator"
			className={cn("pointer-events-none absolute left-1.5 flex items-center justify-center", className)}
			{...props}
		>
			<CheckIcon className="size-4" />
		</ComboboxPrimitive.ItemIndicator>
	)
}

function ComboboxEmpty({ className, ...props }: ComboboxPrimitive.Empty.Props) {
	return (
		<ComboboxPrimitive.Empty
			data-slot="combobox-empty"
			className={cn("py-4 text-center text-sm text-muted-foreground", className)}
			{...props}
		/>
	)
}

function ComboboxGroup({ ...props }: ComboboxPrimitive.Group.Props) {
	return <ComboboxPrimitive.Group data-slot="combobox-group" {...props} />
}

function ComboboxGroupLabel({ className, ...props }: ComboboxPrimitive.GroupLabel.Props) {
	return (
		<ComboboxPrimitive.GroupLabel
			data-slot="combobox-group-label"
			className={cn("px-1.5 py-1 text-xs font-medium text-muted-foreground", className)}
			{...props}
		/>
	)
}

function ComboboxSeparator({ className, ...props }: React.ComponentProps<"div">) {
	return (
		<div
			data-slot="combobox-separator"
			className={cn("-mx-1 my-1 h-px bg-border", className)}
			{...props}
		/>
	)
}

export {
	Combobox,
	ComboboxInputGroup,
	ComboboxInput,
	ComboboxTrigger,
	ComboboxPopup,
	ComboboxItem,
	ComboboxItemIndicator,
	ComboboxEmpty,
	ComboboxGroup,
	ComboboxGroupLabel,
	ComboboxSeparator,
}
