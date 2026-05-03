import { Badge } from '@/components/core/badge'

export function BadgeShowcase() {
	return (
		<section className="py-4 space-y-8">
			<h2 className="text-2xl font-semibold">Badge</h2>
			<div className="space-y-4">
				<div className="flex flex-wrap gap-3">
					<Badge variant="default">Default</Badge>
					<Badge variant="secondary">Secondary</Badge>
					<Badge variant="destructive">Destructive</Badge>
					<Badge variant="outline">Outline</Badge>
					<Badge variant="ghost">Ghost</Badge>
				</div>
			</div>
		</section>
	)
}
