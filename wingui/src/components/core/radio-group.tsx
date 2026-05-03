import { RadioGroup as RadioGroupPrimitive } from "@base-ui/react/radio-group"
import { Radio as RadioPrimitive } from "@base-ui/react/radio"
import { CircleIcon } from "@phosphor-icons/react"

import { cn } from "@/lib/utils"

function RadioGroup({
	className,
	...props
}: RadioGroupPrimitive.Props) {
	return (
		<RadioGroupPrimitive
			data-slot="radio-group"
			className={cn("flex flex-col gap-2", className)}
			{...props}
		/>
	)
}

function RadioGroupItem({
	className,
	...props
}: RadioPrimitive.Root.Props) {
	return (
		<RadioPrimitive.Root
			data-slot="radio-group-item"
			className={cn(
				"peer size-4 shrink-0 rounded-full border border-input shadow-xs transition-shadow outline-none",
				"data-checked:bg-primary data-checked:border-primary data-checked:text-primary-foreground",
				"focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50",
				"aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40",
				"data-disabled:cursor-not-allowed data-disabled:opacity-50",
				className
			)}
			{...props}
		>
			<RadioPrimitive.Indicator
				data-slot="radio-group-indicator"
				className="flex h-full w-full items-center justify-center"
			>
				<CircleIcon
					weight="fill"
					className="size-3 text-primary-foreground"
				/>
			</RadioPrimitive.Indicator>
		</RadioPrimitive.Root>
	)
}

export { RadioGroup, RadioGroupItem }
