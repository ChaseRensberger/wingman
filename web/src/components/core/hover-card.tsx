import { PreviewCard as HoverCardPrimitive } from "@base-ui/react/preview-card"
import { cn } from "@/lib/utils"

function HoverCard({ ...props }: HoverCardPrimitive.Root.Props) {
	return <HoverCardPrimitive.Root data-slot="hover-card" {...props} />
}

function HoverCardTrigger({ ...props }: HoverCardPrimitive.Trigger.Props) {
	return <HoverCardPrimitive.Trigger data-slot="hover-card-trigger" {...props} />
}

function HoverCardContent({
	className,
	side = "bottom",
	sideOffset = 4,
	...props
}: HoverCardPrimitive.Popup.Props &
	Pick<HoverCardPrimitive.Positioner.Props, "side" | "sideOffset">) {
	return (
		<HoverCardPrimitive.Portal>
			<HoverCardPrimitive.Positioner side={side} sideOffset={sideOffset}>
				<HoverCardPrimitive.Popup
					data-slot="hover-card-content"
					className={cn(
						"z-50 w-64 origin-(--transform-origin) rounded-lg bg-popover p-4 text-popover-foreground shadow-md ring-1 ring-foreground/10 outline-none data-open:animate-in data-open:fade-in-0 data-open:zoom-in-95 data-closed:animate-out data-closed:fade-out-0 data-closed:zoom-out-95",
						className
					)}
					{...props}
				/>
			</HoverCardPrimitive.Positioner>
		</HoverCardPrimitive.Portal>
	)
}

export { HoverCard, HoverCardTrigger, HoverCardContent }
