import {
	NavigationMenu,
	NavigationMenuList,
	NavigationMenuItem,
	NavigationMenuTrigger,
	NavigationMenuContent,
	NavigationMenuLink,
} from "@/components/core/navigation-menu"

export function NavigationMenuShowcase() {
	return (
		<section className="py-4 space-y-8">
			<h2 className="text-2xl font-semibold">Navigation Menu</h2>
			<NavigationMenu>
				<NavigationMenuList>
					<NavigationMenuItem>
						<NavigationMenuTrigger>Getting started</NavigationMenuTrigger>
						<NavigationMenuContent>
							<div className="grid gap-3 p-4 w-80">
								<div className="space-y-1">
									<h4 className="text-sm font-medium">Introduction</h4>
									<p className="text-sm text-muted-foreground">Re-usable components built with Base UI and Tailwind CSS.</p>
								</div>
								<div className="space-y-1">
									<h4 className="text-sm font-medium">Installation</h4>
									<p className="text-sm text-muted-foreground">How to install and set up the component library.</p>
								</div>
							</div>
						</NavigationMenuContent>
					</NavigationMenuItem>
					<NavigationMenuItem>
						<NavigationMenuTrigger>Components</NavigationMenuTrigger>
						<NavigationMenuContent>
							<div className="grid grid-cols-2 gap-2 p-4 w-80">
								{["Button", "Input", "Select", "Checkbox", "Radio", "Switch"].map((item) => (
									<div key={item} className="rounded-md p-2 text-sm hover:bg-accent cursor-pointer">
										{item}
									</div>
								))}
							</div>
						</NavigationMenuContent>
					</NavigationMenuItem>
					<NavigationMenuItem>
						<NavigationMenuLink href="#" className="inline-flex h-9 items-center justify-center rounded-md px-4 py-2 text-sm font-medium hover:bg-accent hover:text-accent-foreground">
							Docs
						</NavigationMenuLink>
					</NavigationMenuItem>
				</NavigationMenuList>
			</NavigationMenu>
		</section>
	)
}
