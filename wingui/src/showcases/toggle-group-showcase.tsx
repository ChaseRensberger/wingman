import { TextB, TextItalic, TextUnderline } from "@phosphor-icons/react"
import { ToggleGroup, ToggleGroupItem } from "@/components/core/toggle-group"

export function ToggleGroupShowcase() {
	return (
		<section className="py-4 space-y-8">
			<h2 className="text-2xl font-semibold">Toggle Group</h2>
			<div className="space-y-4">
				<div className="flex flex-wrap gap-3">
					<ToggleGroup>
						<ToggleGroupItem value="bold" aria-label="Bold">
							<TextB />
						</ToggleGroupItem>
						<ToggleGroupItem value="italic" aria-label="Italic">
							<TextItalic />
						</ToggleGroupItem>
						<ToggleGroupItem value="underline" aria-label="Underline">
							<TextUnderline />
						</ToggleGroupItem>
					</ToggleGroup>
				</div>
				<div className="flex flex-wrap gap-3">
					<ToggleGroup>
						<ToggleGroupItem value="bold" variant="outline" aria-label="Bold">
							<TextB /> Bold
						</ToggleGroupItem>
						<ToggleGroupItem value="italic" variant="outline" aria-label="Italic">
							<TextItalic /> Italic
						</ToggleGroupItem>
						<ToggleGroupItem value="underline" variant="outline" aria-label="Underline">
							<TextUnderline /> Underline
						</ToggleGroupItem>
					</ToggleGroup>
				</div>
				<div className="flex flex-wrap gap-3">
					<ToggleGroup>
						<ToggleGroupItem value="sm" size="sm">Small</ToggleGroupItem>
						<ToggleGroupItem value="default" size="default">Default</ToggleGroupItem>
						<ToggleGroupItem value="lg" size="lg">Large</ToggleGroupItem>
					</ToggleGroup>
				</div>
				<div className="flex flex-wrap gap-3">
					<ToggleGroup disabled>
						<ToggleGroupItem value="bold" aria-label="Bold">
							<TextB /> Disabled
						</ToggleGroupItem>
					</ToggleGroup>
				</div>
			</div>
		</section>
	)
}
