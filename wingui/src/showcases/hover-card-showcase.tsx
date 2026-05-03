import { HoverCard, HoverCardTrigger, HoverCardContent } from "@/components/core/hover-card"

export function HoverCardShowcase() {
	return (
		<section className="py-4 space-y-8">
			<h2 className="text-2xl font-semibold">Hover Card</h2>
			<HoverCard>
				<HoverCardTrigger
					render={<span className="cursor-pointer underline underline-offset-4 text-primary" />}
				>
					@wingman
				</HoverCardTrigger>
				<HoverCardContent>
					<div className="space-y-2">
						<h4 className="text-sm font-semibold">@wingman</h4>
						<p className="text-sm text-muted-foreground">
							Building the next generation of AI-powered developer tools.
						</p>
						<p className="text-xs text-muted-foreground">Joined January 2024</p>
					</div>
				</HoverCardContent>
			</HoverCard>
		</section>
	)
}
