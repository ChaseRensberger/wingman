import { createRootRoute, Outlet, Link, useMatches } from "@tanstack/react-router";
import { ThemeProvider } from "@wingman/core/components/theme-provider";
import {
	Sidebar,
	SidebarContent,
	SidebarGroup,
	SidebarGroupContent,
	SidebarHeader,
	SidebarMenu,
	SidebarMenuButton,
	SidebarMenuItem,
	SidebarProvider,
	SidebarInset,
	SidebarTrigger,
} from "@wingman/core/components/primitives/sidebar";
import { Separator } from "@wingman/core/components/primitives/separator";
import { Bot } from "lucide-react";
import WingmanIcon from "@wingman/core/assets/WingmanBlue.png";

const NAV_ITEMS = [
	{ label: "Agents", to: "/agents" as const, icon: Bot },
];

function AppSidebar() {
	const matches = useMatches();
	const currentPath = matches[matches.length - 1]?.pathname ?? "";

	return (
		<Sidebar>
			<SidebarHeader className="p-4">
				<Link to="/" className="flex items-center gap-2">
					<img src={WingmanIcon} className="w-8 h-8" alt="Wingman" />
					<span className="font-semibold text-sm tracking-widest text-primary">WINGMAN</span>
				</Link>
			</SidebarHeader>
			<Separator />
			<SidebarContent>
				<SidebarGroup>
					<SidebarGroupContent>
						<SidebarMenu>
							{NAV_ITEMS.map((item) => (
								<SidebarMenuItem key={item.to}>
									<SidebarMenuButton asChild isActive={currentPath.startsWith(item.to)} tooltip={item.label}>
										<Link to={item.to}>
											<item.icon className="size-4" />
											<span>{item.label}</span>
										</Link>
									</SidebarMenuButton>
								</SidebarMenuItem>
							))}
						</SidebarMenu>
					</SidebarGroupContent>
				</SidebarGroup>
			</SidebarContent>
		</Sidebar>
	);
}

function RootLayout() {
	return (
		<ThemeProvider defaultTheme="system" storageKey="wingman-admin-theme">
			<SidebarProvider>
				<AppSidebar />
				<SidebarInset>
					<header className="flex h-12 items-center gap-2 border-b px-4 md:hidden">
						<SidebarTrigger />
						<Separator orientation="vertical" className="h-4" />
						<img src={WingmanIcon} className="w-6 h-6" alt="Wingman" />
						<span className="font-semibold text-xs tracking-widest text-primary">WINGMAN</span>
					</header>
					<main className="flex-1 p-4 md:p-6">
						<Outlet />
					</main>
				</SidebarInset>
			</SidebarProvider>
		</ThemeProvider>
	);
}

export const Route = createRootRoute({ component: RootLayout });
