import {
	Avatar,
	AvatarFallback,
	AvatarImage,
} from "@/components/core/avatar"
import WingmanBlue from "@/assets/WingmanBlue.png"

export function AvatarShowcase() {
	return (
		<section className="py-4 space-y-8">
			<h2 className="text-2xl font-semibold">Avatar</h2>
			<div className="flex items-center gap-4">
				<Avatar>
					<AvatarImage src={WingmanBlue} alt="Wingman" />
					<AvatarFallback>WM</AvatarFallback>
				</Avatar>
				<Avatar>
					<AvatarFallback>WM</AvatarFallback>
				</Avatar>
			</div>
		</section>
	)
}
