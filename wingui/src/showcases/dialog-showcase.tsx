import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
	DialogTrigger,
	DialogClose,
} from "@/components/core/dialog"
import { Button } from "@/components/core/button"

export function DialogShowcase() {
	return (
		<section className="py-4 space-y-8">
			<h2 className="text-2xl font-semibold">Dialog</h2>
			<div className="flex flex-wrap gap-3">
				<Dialog>
					<DialogTrigger render={<Button variant="outline">Open Dialog</Button>} />
					<DialogContent>
						<DialogHeader>
							<DialogTitle>Edit Profile</DialogTitle>
							<DialogDescription>
								Make changes to your profile here. Click save when you're done.
							</DialogDescription>
						</DialogHeader>
						<DialogFooter>
							<DialogClose render={<Button variant="outline">Cancel</Button>} />
							<Button>Save changes</Button>
						</DialogFooter>
					</DialogContent>
				</Dialog>
				<Dialog>
					<DialogTrigger render={<Button variant="outline">No Close Button</Button>} />
					<DialogContent showCloseButton={false}>
						<DialogHeader>
							<DialogTitle>Confirm Action</DialogTitle>
							<DialogDescription>
								This dialog has no close button. Use the actions below to dismiss it.
							</DialogDescription>
						</DialogHeader>
						<DialogFooter>
							<DialogClose render={<Button variant="outline">Cancel</Button>} />
							<Button>Confirm</Button>
						</DialogFooter>
					</DialogContent>
				</Dialog>
			</div>
		</section>
	)
}
