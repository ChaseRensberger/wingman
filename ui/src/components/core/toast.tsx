import { Toast as ToastPrimitive } from "@base-ui/react/toast"
import { XIcon } from "@phosphor-icons/react"
import { cn } from "@/lib/utils"
import { cva, type VariantProps } from "class-variance-authority"

const toastVariants = cva(
	"group/toast relative flex w-full items-start gap-3 rounded-lg border bg-background p-4 shadow-lg ring-1 ring-foreground/5 data-open:animate-in data-open:slide-in-from-bottom-5 data-open:fade-in-0 data-closed:animate-out data-closed:slide-out-to-right-full data-closed:fade-out-80",
	{
		variants: {
			variant: {
				default: "border-border",
				destructive: "border-destructive/20 bg-destructive/5 text-destructive",
				success: "border-green-500/20 bg-green-500/5 text-green-700 dark:text-green-400",
			},
		},
		defaultVariants: {
			variant: "default",
		},
	}
)

function ToastProvider({ ...props }: ToastPrimitive.Provider.Props) {
	return <ToastPrimitive.Provider {...props} />
}

function ToastViewport({ className, ...props }: ToastPrimitive.Viewport.Props) {
	return (
		<ToastPrimitive.Viewport
			data-slot="toast-viewport"
			className={cn(
				"fixed bottom-0 right-0 z-[100] flex max-h-screen flex-col-reverse gap-2 p-4 sm:max-w-[420px]",
				className
			)}
			{...props}
		/>
	)
}

function Toast({
	className,
	variant,
	...props
}: ToastPrimitive.Root.Props & VariantProps<typeof toastVariants>) {
	return (
		<ToastPrimitive.Root
			data-slot="toast"
			className={cn(toastVariants({ variant }), className)}
			{...props}
		/>
	)
}

function ToastTitle({ className, ...props }: ToastPrimitive.Title.Props) {
	return (
		<ToastPrimitive.Title
			data-slot="toast-title"
			className={cn("text-sm font-semibold leading-none tracking-tight", className)}
			{...props}
		/>
	)
}

function ToastDescription({ className, ...props }: ToastPrimitive.Description.Props) {
	return (
		<ToastPrimitive.Description
			data-slot="toast-description"
			className={cn("text-sm opacity-80", className)}
			{...props}
		/>
	)
}

function ToastClose({ className, ...props }: ToastPrimitive.Close.Props) {
	return (
		<ToastPrimitive.Close
			data-slot="toast-close"
			className={cn(
				"absolute right-2 top-2 rounded-md p-1 text-foreground/50 transition-opacity hover:text-foreground focus:outline-none focus-visible:ring-2 focus-visible:ring-ring",
				className
			)}
			{...props}
		>
			<XIcon className="size-4" />
		</ToastPrimitive.Close>
	)
}

export { ToastProvider, ToastViewport, Toast, ToastTitle, ToastDescription, ToastClose }
