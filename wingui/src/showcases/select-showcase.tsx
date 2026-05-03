import {
	Select,
	SelectContent,
	SelectGroup,
	SelectItem,
	SelectLabel,
	SelectSeparator,
	SelectTrigger,
	SelectValue,
} from '@/components/core/select'

export function SelectShowcase() {
	return (
		<section className="py-4 space-y-8">
			<h2 className="text-2xl font-semibold">Select</h2>
			<div className="space-y-4">
				<div className="flex flex-wrap gap-4">
					<Select>
						<SelectTrigger className="w-48">
							<SelectValue placeholder="Select a fruit" />
						</SelectTrigger>
						<SelectContent>
							<SelectGroup>
								<SelectItem value="apple">Apple</SelectItem>
								<SelectItem value="banana">Banana</SelectItem>
								<SelectItem value="blueberry">Blueberry</SelectItem>
								<SelectItem value="grapes">Grapes</SelectItem>
								<SelectItem value="pineapple">Pineapple</SelectItem>
							</SelectGroup>
						</SelectContent>
					</Select>

					<Select disabled>
						<SelectTrigger className="w-48">
							<SelectValue placeholder="Disabled" />
						</SelectTrigger>
						<SelectContent>
							<SelectGroup>
								<SelectItem value="apple">Apple</SelectItem>
							</SelectGroup>
						</SelectContent>
					</Select>
				</div>

				<Select>
					<SelectTrigger className="w-48">
						<SelectValue placeholder="Select a timezone" />
					</SelectTrigger>
					<SelectContent>
						<SelectGroup>
							<SelectLabel>North America</SelectLabel>
							<SelectItem value="est">Eastern Time (ET)</SelectItem>
							<SelectItem value="cst">Central Time (CT)</SelectItem>
							<SelectItem value="mst">Mountain Time (MT)</SelectItem>
							<SelectItem value="pst">Pacific Time (PT)</SelectItem>
						</SelectGroup>
						<SelectSeparator />
						<SelectGroup>
							<SelectLabel>Europe</SelectLabel>
							<SelectItem value="gmt">Greenwich Mean Time (GMT)</SelectItem>
							<SelectItem value="cet">Central European Time (CET)</SelectItem>
						</SelectGroup>
					</SelectContent>
				</Select>
			</div>
		</section>
	)
}
