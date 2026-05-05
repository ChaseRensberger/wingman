import { ScrollArea } from "@/components/core/scroll-area"
import { Separator } from "@/components/core/separator"

const tags = Array.from({ length: 50 }, (_, i) => `Item ${i + 1}`)

export function ScrollAreaShowcase() {
	return (
		<section className="py-4 space-y-8">
			<h2 className="text-2xl font-semibold">Scroll Area</h2>
			<div className="flex gap-6">
				<ScrollArea className="h-72 w-48 rounded-md border">
					<div className="p-4">
						<h4 className="mb-4 text-sm font-medium leading-none">Tags</h4>
						{tags.map((tag, i) => (
							<div key={tag}>
								<div className="text-sm">{tag}</div>
								{i < tags.length - 1 && <Separator className="my-2" />}
							</div>
						))}
					</div>
				</ScrollArea>
			</div>
		</section>
	)
}
