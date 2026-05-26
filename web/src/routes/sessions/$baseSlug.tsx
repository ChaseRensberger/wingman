import { Outlet, createFileRoute } from "@tanstack/react-router";

export const Route = createFileRoute("/sessions/$baseSlug")({
	component: BaseLayout,
});

function BaseLayout() {
	return <Outlet />;
}
