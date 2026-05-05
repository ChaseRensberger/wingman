import {
	DropdownMenu,
	DropdownMenuTrigger,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuGroup,
	DropdownMenuLabel,
	DropdownMenuSeparator,
	DropdownMenuShortcut,
	DropdownMenuSub,
	DropdownMenuSubTrigger,
	DropdownMenuSubContent,
	DropdownMenuPortal
} from "@/components/core/dropdown-menu"
import { Button } from "@/components/core/button"

export function DropdownMenuShowcase() {
	return (
		<section className="py-4 space-y-8">
			<h2 className="text-2xl font-semibold">Dropdown Menu</h2>
			<DropdownMenu>
				<DropdownMenuTrigger render={<Button variant="outline">Open</Button>} />
				<DropdownMenuContent className="w-40" align="start">
					<DropdownMenuGroup>
						<DropdownMenuLabel>My Account</DropdownMenuLabel>
						<DropdownMenuItem>
							Profile
							<DropdownMenuShortcut>⇧⌘P</DropdownMenuShortcut>
						</DropdownMenuItem>
						<DropdownMenuItem>
							Billing
							<DropdownMenuShortcut>⌘B</DropdownMenuShortcut>
						</DropdownMenuItem>
						<DropdownMenuItem>
							Settings
							<DropdownMenuShortcut>⌘S</DropdownMenuShortcut>
						</DropdownMenuItem>
					</DropdownMenuGroup>
					<DropdownMenuSeparator />
					<DropdownMenuGroup>
						<DropdownMenuItem>Team</DropdownMenuItem>
						<DropdownMenuSub>
							<DropdownMenuSubTrigger>Invite users</DropdownMenuSubTrigger>
							<DropdownMenuPortal>
								<DropdownMenuSubContent>
									<DropdownMenuItem>Email</DropdownMenuItem>
									<DropdownMenuItem>Message</DropdownMenuItem>
									<DropdownMenuSeparator />
									<DropdownMenuItem>More...</DropdownMenuItem>
								</DropdownMenuSubContent>
							</DropdownMenuPortal>
						</DropdownMenuSub>
						<DropdownMenuItem>
							New Team
							<DropdownMenuShortcut>⌘+T</DropdownMenuShortcut>
						</DropdownMenuItem>
					</DropdownMenuGroup>
					<DropdownMenuSeparator />
					<DropdownMenuGroup>
						<DropdownMenuItem>GitHub</DropdownMenuItem>
						<DropdownMenuItem>Support</DropdownMenuItem>
						<DropdownMenuItem disabled>API</DropdownMenuItem>
					</DropdownMenuGroup>
					<DropdownMenuSeparator />
					<DropdownMenuGroup>
						<DropdownMenuItem>
							Log out
							<DropdownMenuShortcut>⇧⌘Q</DropdownMenuShortcut>
						</DropdownMenuItem>
					</DropdownMenuGroup>
				</DropdownMenuContent>
			</DropdownMenu>
		</section >
	)
}
