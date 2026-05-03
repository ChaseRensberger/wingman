import { Button } from '@/components/core/button'
import {
	Popover,
	PopoverContent,
	PopoverDescription,
	PopoverHeader,
	PopoverTitle,
	PopoverTrigger,
} from '@/components/core/popover'

export function PopoverShowcase() {
	return (
		<section className="py-4 space-y-8">
			<h2 className="text-2xl font-semibold">Popover</h2>
			<div className="space-y-4">
				<Popover>
					<PopoverTrigger render={<Button variant="outline" />}>
						Open Popover
					</PopoverTrigger>
					<PopoverContent>
						<PopoverHeader>
							<PopoverTitle>Dimensions</PopoverTitle>
							<PopoverDescription>
								Set the dimensions for the layer.
							</PopoverDescription>
						</PopoverHeader>
					</PopoverContent>
				</Popover>
			</div>
		</section>
	)
}
