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
} from "@wingman/core/components/primitives/sidebar";
import { Bot, KeyRound, MessageSquare, Settings } from "lucide-react";
import WingmanIcon from "@wingman/core/assets/WingmanBlue.png";
import { Separator } from "@wingman/core/components/primitives/separator";

const NAV_ITEMS = [
	{ label: "Agents", to: "/agents" as const, icon: Bot },
	{ label: "Sessions", to: "/sessions" as const, icon: MessageSquare },
	{ label: "Auth", to: "/auth" as const, icon: KeyRound },
	{ label: "Settings", to: "/settings" as const, icon: Settings },
];

function AppSidebar() {
	const matches = useMatches();
	const currentPath = matches[matches.length - 1]?.pathname ?? "";

	return (
		<Sidebar variant="sidebar" collapsible="none" className="border-r bg-background sticky top-0">
			<SidebarHeader>
				<div className="flex items-center gap-2">
					<img src={WingmanIcon} className="w-12 h-12" />
					<span className="text-xl font-semibold text-primary">Admin</span>
				</div>
			</SidebarHeader>
			<Separator />
			<SidebarContent className="p-4">
				<SidebarGroup className="p-0">
					<SidebarGroupContent>
						<SidebarMenu className="gap-1">
							{NAV_ITEMS.map((item) => (
								<SidebarMenuItem key={item.to}>
									<SidebarMenuButton asChild isActive={currentPath.startsWith(item.to)} className="h-auto p-2">
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
				<div className="flex-1 flex">
					<AppSidebar />
					<main className="flex-1">
						<Outlet />
					</main>
				</div>
			</SidebarProvider>
		</ThemeProvider>
	);
}

export const Route = createRootRoute({ component: RootLayout });
