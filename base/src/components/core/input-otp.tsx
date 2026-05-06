import { OTPFieldPreview as OtpFieldPrimitive } from "@base-ui/react/otp-field"
import { cn } from "@/lib/utils"

function InputOTP({
	className,
	...props
}: OtpFieldPrimitive.Root.Props) {
	return (
		<OtpFieldPrimitive.Root
			data-slot="input-otp"
			className={cn("flex items-center gap-2", className)}
			{...props}
		/>
	)
}

function InputOTPSlot({
	className,
	...props
}: OtpFieldPrimitive.Input.Props) {
	return (
		<OtpFieldPrimitive.Input
			data-slot="input-otp-slot"
			className={cn(
				"relative flex h-10 w-10 items-center justify-center rounded-lg border border-input bg-background text-sm font-medium shadow-sm transition-all text-center caret-transparent outline-none data-active:ring-2 data-active:ring-ring data-active:ring-offset-1 data-disabled:pointer-events-none data-disabled:opacity-50",
				className
			)}
			{...props}
		/>
	)
}

function InputOTPSeparator({
	className,
	...props
}: React.ComponentProps<"span">) {
	return (
		<span
			data-slot="input-otp-separator"
			role="separator"
			className={cn("text-muted-foreground", className)}
			{...props}
		>
			-
		</span>
	)
}

export { InputOTP, InputOTPSlot, InputOTPSeparator }
