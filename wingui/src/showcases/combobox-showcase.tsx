import { useState } from "react"
import {
	Combobox,
	ComboboxInputGroup,
	ComboboxInput,
	ComboboxTrigger,
	ComboboxPopup,
	ComboboxItem,
	ComboboxItemIndicator,
	ComboboxEmpty,
} from "@/components/core/combobox"
import { CaretUpDownIcon } from "@phosphor-icons/react"

const frameworks = [
	{ value: "react", label: "React" },
	{ value: "vue", label: "Vue" },
	{ value: "angular", label: "Angular" },
	{ value: "svelte", label: "Svelte" },
	{ value: "solid", label: "Solid" },
	{ value: "qwik", label: "Qwik" },
]

export function ComboboxShowcase() {
	const [value, setValue] = useState<string | null>("")

	return (
		<section className="py-4 space-y-8">
			<h2 className="text-2xl font-semibold">Combobox</h2>
			<div className="max-w-xs">
				<Combobox value={value} onValueChange={setValue}>
					<ComboboxInputGroup>
						<ComboboxInput placeholder="Search framework..." />
						<ComboboxTrigger>
							<CaretUpDownIcon className="size-4" />
						</ComboboxTrigger>
					</ComboboxInputGroup>
					<ComboboxPopup>
						{frameworks.map((fw) => (
							<ComboboxItem key={fw.value} value={fw.value}>
								<ComboboxItemIndicator />
								{fw.label}
							</ComboboxItem>
						))}
						<ComboboxEmpty>No framework found.</ComboboxEmpty>
					</ComboboxPopup>
				</Combobox>
			</div>
		</section>
	)
}
