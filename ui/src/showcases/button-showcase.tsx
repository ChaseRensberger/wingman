import { Button } from '@/components/core/button'

export function ButtonShowcase() {
	return (
		<section className="py-4 space-y-8">
			<h2 className="text-2xl font-semibold">Button</h2>
			<div className="space-y-4">
				<div className="flex flex-wrap gap-3">
					<Button variant="default">Default</Button>
					<Button variant="secondary">Secondary</Button>
					<Button variant="outline">Outline</Button>
					<Button variant="ghost">Ghost</Button>
					<Button variant="destructive">Destructive</Button>
					<Button variant="link">Link</Button>
				</div>
			</div>
		</section>
	)
}
