import { createRootRoute, Outlet } from '@tanstack/react-router'
import { SidebarProvider, AppSidebar, SidebarShell } from '../components/sidebar'

export const Route = createRootRoute({
  component: RootLayout,
})

function RootLayout() {
  return (
    <SidebarProvider>
      <SidebarShell sidebar={<AppSidebar />}>
        <Outlet />
      </SidebarShell>
    </SidebarProvider>
  )
}
