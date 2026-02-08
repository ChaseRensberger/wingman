import { createFileRoute, Outlet, Link, useParams } from "@tanstack/react-router";
import WingmanIcon from "../assets/WingmanBlue.png";
import { getGroupedDocs } from "@/lib/docs";
import { Menu, X } from "lucide-react";
import { Button } from "@wingman/core/components/primitives/button";
import {
	Sidebar,
	SidebarContent,
	SidebarGroup,
	SidebarGroupContent,
	SidebarGroupLabel,
	SidebarMenu,
	SidebarMenuButton,
	SidebarMenuItem,
	SidebarProvider,
	useSidebar,
} from "@wingman/core/components/primitives/sidebar";

export const Route = createFileRoute("/docs")({
	component: DocsLayout,
});

function DocsSidebar() {
	const params = useParams({ strict: false });
	const slug = (params as { slug?: string }).slug;
	const groups = getGroupedDocs();

	return (
		<Sidebar variant="sidebar" collapsible="none" className="border-r bg-background">
			<SidebarContent className="p-4">
				{groups.map((group) => (
					<SidebarGroup key={group.name} className="p-0">
						{group.name !== "Uncategorized" && (
							<SidebarGroupLabel className="font-semibold text-sm text-muted-foreground mb-2 px-0 h-auto">
								{group.name}
							</SidebarGroupLabel>
						)}
						<SidebarGroupContent>
							<SidebarMenu className="gap-1">
								{group.docs.map((doc) => (
									<SidebarMenuItem key={doc.slug}>
										<SidebarMenuButton asChild isActive={slug === doc.slug} className="h-auto py-1 px-2">
											<Link to="/docs/$slug" params={{ slug: doc.slug }}>
												{doc.title}
											</Link>
										</SidebarMenuButton>
									</SidebarMenuItem>
								))}
							</SidebarMenu>
						</SidebarGroupContent>
					</SidebarGroup>
				))}
			</SidebarContent>
		</Sidebar>
	);
}

function MobileSidebarToggle() {
	const { openMobile, setOpenMobile } = useSidebar();

	return (
		<Button
			variant="ghost"
			className="md:hidden"
			onClick={() => setOpenMobile(!openMobile)}
		>
			{openMobile ? <X className="w-6 h-6" /> : <Menu className="w-6 h-6" />}
		</Button>
	);
}

function DocsLayout() {
	return (
		<SidebarProvider>
			<div className="min-h-screen flex flex-col">
				{/* Header */}
				<div className="sticky top-0 bg-background flex items-center justify-between px-6 py-2 w-full border-b z-10">
					<Link to="/">
						<img src={WingmanIcon} className="w-12 h-12" />
					</Link>
					<MobileSidebarToggle />
				</div>
				{/* Sidebar + Content */}
				<div className="flex-1 flex">
					<DocsSidebar />
					{/* Main Content */}
					<main className="flex-1 p-8">
						<Outlet />
					</main>
				</div>
			</div>
		</SidebarProvider>
	);
}
