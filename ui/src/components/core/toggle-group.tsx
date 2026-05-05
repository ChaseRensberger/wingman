import { ToggleGroup as ToggleGroupPrimitive } from "@base-ui/react/toggle-group"
import { Toggle as TogglePrimitive } from "@base-ui/react/toggle"
import { type VariantProps } from "class-variance-authority"

import { cn } from "@/lib/utils"
import { toggleVariants } from "@/components/core/toggle"

function ToggleGroup({
	className,
	...props
}: ToggleGroupPrimitive.Props) {
	return (
		<ToggleGroupPrimitive
			data-slot="toggle-group"
			className={cn(
				"flex items-center gap-0.5 data-[orientation=vertical]:flex-col",
				className
			)}
			{...props}
		/>
	)
}

function ToggleGroupItem({
	className,
	variant,
	size,
	...props
}: TogglePrimitive.Props & VariantProps<typeof toggleVariants>) {
	return (
		<TogglePrimitive
			data-slot="toggle-group-item"
			className={cn(toggleVariants({ variant, size, className }))}
			{...props}
		/>
	)
}

export { ToggleGroup, ToggleGroupItem }
