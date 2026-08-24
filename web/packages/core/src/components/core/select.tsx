import { Select as SelectPrimitive } from "@base-ui/react/select"
import { CaretUpDownIcon, CheckIcon } from "@phosphor-icons/react"

import { cn } from "#lib/utils"

function Select({ ...props }: SelectPrimitive.Root.Props<string>) {
	return <SelectPrimitive.Root data-slot="select" {...props} />
}

function SelectTrigger({
	className,
	children,
	...props
}: SelectPrimitive.Trigger.Props) {
	return (
		<SelectPrimitive.Trigger
			data-slot="select-trigger"
			className={cn(
				"flex h-9 w-full items-center justify-between gap-2 rounded-[var(--radius)] border border-input bg-background px-2.5 py-1 text-sm shadow-xs outline-none transition-[color,box-shadow] duration-150 focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 data-disabled:pointer-events-none data-disabled:opacity-50 data-placeholder:text-muted-foreground *:data-[slot=select-value]:line-clamp-1 *:data-[slot=select-value]:flex *:data-[slot=select-value]:items-center *:data-[slot=select-value]:gap-2",
				className
			)}
			{...props}
		>
			{children}
			<SelectPrimitive.Icon className="size-4 shrink-0 text-muted-foreground">
				<CaretUpDownIcon />
			</SelectPrimitive.Icon>
		</SelectPrimitive.Trigger>
	)
}

function SelectValue({ ...props }: SelectPrimitive.Value.Props) {
	return <SelectPrimitive.Value data-slot="select-value" {...props} />
}

function SelectContent({
	className,
	children,
	align = "center",
	sideOffset = 4,
	alignItemWithTrigger = true,
	...props
}: SelectPrimitive.Popup.Props &
	Pick<SelectPrimitive.Positioner.Props, "align" | "sideOffset"> & {
		alignItemWithTrigger?: boolean
	}) {
	return (
		<SelectPrimitive.Portal>
			<SelectPrimitive.Positioner
				className="isolate z-50"
				align={align}
				sideOffset={sideOffset}
				alignItemWithTrigger={alignItemWithTrigger}
			>
				<SelectPrimitive.Popup
					data-slot="select-content"
					className={cn(
						"relative z-50 max-h-(--available-height) min-w-(--anchor-width) overflow-y-auto rounded-[var(--radius)] border bg-popover p-1 text-popover-foreground shadow-sm",
						className
					)}
					{...props}
				>
					<SelectPrimitive.List className="flex flex-col gap-0.5">
						{children}
					</SelectPrimitive.List>
				</SelectPrimitive.Popup>
			</SelectPrimitive.Positioner>
		</SelectPrimitive.Portal>
	)
}

function SelectGroup({ ...props }: SelectPrimitive.Group.Props) {
	return (
		<SelectPrimitive.Group data-slot="select-group" {...props} />
	)
}

function SelectLabel({
	className,
	...props
}: SelectPrimitive.GroupLabel.Props) {
	return (
		<SelectPrimitive.GroupLabel
			data-slot="select-label"
			className={cn("px-2 py-1.5 text-xs font-medium text-muted-foreground", className)}
			{...props}
		/>
	)
}

function SelectItem({
	className,
	children,
	...props
}: SelectPrimitive.Item.Props) {
	return (
		<SelectPrimitive.Item
			data-slot="select-item"
			className={cn(
				"group/select-item relative flex w-full cursor-pointer items-center gap-2 rounded-[var(--radius)] py-1 pr-8 pl-2 text-sm outline-none select-none focus:bg-accent focus:text-accent-foreground data-disabled:pointer-events-none data-disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
				className
			)}
			{...props}
		>
			<span className="absolute right-2 flex size-3.5 items-center justify-center">
				<SelectPrimitive.ItemIndicator>
					<CheckIcon className="size-4" />
				</SelectPrimitive.ItemIndicator>
			</span>
			<SelectPrimitive.ItemText>{children}</SelectPrimitive.ItemText>
		</SelectPrimitive.Item>
	)
}

function SelectSeparator({
	className,
	...props
}: SelectPrimitive.Separator.Props) {
	return (
		<SelectPrimitive.Separator
			data-slot="select-separator"
			className={cn("-mx-1 my-1 h-px bg-border", className)}
			{...props}
		/>
	)
}

export {
	Select,
	SelectTrigger,
	SelectValue,
	SelectContent,
	SelectGroup,
	SelectLabel,
	SelectItem,
	SelectSeparator,
}
