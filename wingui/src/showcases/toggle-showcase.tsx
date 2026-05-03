import { TextB, TextItalic, TextUnderline } from "@phosphor-icons/react"
import { Toggle } from "@/components/core/toggle"

export function ToggleShowcase() {
	return (
		<section className="py-4 space-y-8">
			<h2 className="text-2xl font-semibold">Toggle</h2>
			<div className="space-y-4">
				<div className="flex flex-wrap gap-3">
					<Toggle aria-label="Bold">
						<TextB />
					</Toggle>
					<Toggle aria-label="Italic">
						<TextItalic />
					</Toggle>
					<Toggle aria-label="Underline">
						<TextUnderline />
					</Toggle>
				</div>
				<div className="flex flex-wrap gap-3">
					<Toggle variant="outline" aria-label="Bold">
						<TextB /> Bold
					</Toggle>
					<Toggle variant="outline" aria-label="Italic">
						<TextItalic /> Italic
					</Toggle>
				</div>
				<div className="flex flex-wrap gap-3">
					<Toggle size="sm" aria-label="Bold">Small</Toggle>
					<Toggle size="default" aria-label="Bold">Default</Toggle>
					<Toggle size="lg" aria-label="Bold">Large</Toggle>
				</div>
				<div className="flex flex-wrap gap-3">
					<Toggle disabled aria-label="Bold">
						<TextB /> Disabled
					</Toggle>
				</div>
			</div>
		</section>
	)
}
