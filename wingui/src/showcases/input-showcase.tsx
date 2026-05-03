import { Input } from '@/components/core/input'

export function InputShowcase() {
	return (
		<section className="py-4 space-y-8">
			<h2 className="text-2xl font-semibold">Input</h2>
			<div className="space-y-4 max-w-sm">
				<Input placeholder="Enter your email" type="email" />
				<Input placeholder="Disabled input" disabled />
				<Input placeholder="Invalid input" aria-invalid />
			</div>
		</section>
	)
}
