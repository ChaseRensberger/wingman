import * as React from "react"
import { Drawer as DrawerPrimitive } from "@base-ui/react/drawer"

import { cn } from "@/lib/utils"

function Drawer({ ...props }: DrawerPrimitive.Root.Props) {
	return <DrawerPrimitive.Root data-slot="drawer" {...props} />
}

function DrawerTrigger({
	...props
}: DrawerPrimitive.Trigger.Props) {
	return <DrawerPrimitive.Trigger data-slot="drawer-trigger" {...props} />
}

function DrawerPortal({
	...props
}: DrawerPrimitive.Portal.Props) {
	return <DrawerPrimitive.Portal data-slot="drawer-portal" {...props} />
}

function DrawerClose({
	...props
}: DrawerPrimitive.Close.Props) {
	return <DrawerPrimitive.Close data-slot="drawer-close" {...props} />
}

function DrawerOverlay({
	className,
	...props
}: DrawerPrimitive.Backdrop.Props) {
	return (
		<DrawerPrimitive.Backdrop
			data-slot="drawer-overlay"
			className={cn(
				"fixed inset-0 z-50 bg-black/50 data-closed:animate-out data-closed:fade-out-0 data-open:animate-in data-open:fade-in-0",
				className
			)}
			{...props}
		/>
	)
}

function DrawerContent({
	className,
	children,
	...props
}: DrawerPrimitive.Popup.Props) {
	return (
		<DrawerPortal>
			<DrawerOverlay />
			<DrawerPrimitive.Popup
				data-slot="drawer-popup"
				className={cn(
					"group/drawer-popup fixed inset-x-0 bottom-0 z-50 flex h-auto flex-col rounded-t-lg border-t bg-background shadow-lg outline-none data-closed:animate-out data-closed:slide-out-to-bottom data-closed:fade-out-0 data-open:animate-in data-open:slide-in-from-bottom data-open:fade-in-0",
					className
				)}
				{...props}
			>
				<div className="mx-auto mt-4 h-2 w-[100px] shrink-0 rounded-full bg-muted" />
				{children}
			</DrawerPrimitive.Popup>
		</DrawerPortal>
	)
}

function DrawerHeader({ className, ...props }: React.ComponentProps<"div">) {
	return (
		<div
			data-slot="drawer-header"
			className={cn(
				"flex flex-col gap-2 p-4 text-center sm:text-left",
				className
			)}
			{...props}
		/>
	)
}

function DrawerFooter({ className, ...props }: React.ComponentProps<"div">) {
	return (
		<div
			data-slot="drawer-footer"
			className={cn(
				"mt-auto flex flex-col gap-2 p-4",
				className
			)}
			{...props}
		/>
	)
}

function DrawerTitle({
	className,
	...props
}: DrawerPrimitive.Title.Props) {
	return (
		<DrawerPrimitive.Title
			data-slot="drawer-title"
			className={cn("text-lg leading-none font-semibold", className)}
			{...props}
		/>
	)
}

function DrawerDescription({
	className,
	...props
}: DrawerPrimitive.Description.Props) {
	return (
		<DrawerPrimitive.Description
			data-slot="drawer-description"
			className={cn("text-sm text-muted-foreground", className)}
			{...props}
		/>
	)
}

export {
	Drawer,
	DrawerPortal,
	DrawerOverlay,
	DrawerTrigger,
	DrawerClose,
	DrawerContent,
	DrawerHeader,
	DrawerFooter,
	DrawerTitle,
	DrawerDescription,
}
