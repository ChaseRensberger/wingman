import {
	InputOTP,
	InputOTPSlot,
	InputOTPSeparator,
} from "@/components/core/input-otp"

export function InputOTPShowcase() {
	return (
		<section className="py-4 space-y-8">
			<h2 className="text-2xl font-semibold">Input OTP</h2>
			<div className="space-y-4">
				<InputOTP length={6}>
					<InputOTPSlot />
					<InputOTPSlot />
					<InputOTPSlot />
					<InputOTPSeparator />
					<InputOTPSlot />
					<InputOTPSlot />
					<InputOTPSlot />
				</InputOTP>
			</div>
		</section>
	)
}
