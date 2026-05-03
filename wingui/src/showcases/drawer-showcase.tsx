import { Button } from "@/components/core/button"
import {
	Drawer,
	DrawerClose,
	DrawerContent,
	DrawerDescription,
	DrawerFooter,
	DrawerHeader,
	DrawerTitle,
	DrawerTrigger,
} from "@/components/core/drawer"

export function DrawerShowcase() {
	return (
		<section className="py-4 space-y-8">
			<h2 className="text-2xl font-semibold">Drawer</h2>
			<Drawer>
				<DrawerTrigger render={<Button variant="outline">Open Drawer</Button>} />
				<DrawerContent>
					<DrawerHeader>
						<DrawerTitle>Are you absolutely sure?</DrawerTitle>
						<DrawerDescription>
							This action cannot be undone.
						</DrawerDescription>
					</DrawerHeader>
					<DrawerFooter>
						<Button>Submit</Button>
						<DrawerClose render={<Button variant="outline">Cancel</Button>} />
					</DrawerFooter>
				</DrawerContent>
			</Drawer>
		</section>
	)
}
