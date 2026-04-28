import { createRootRoute, Outlet } from '@tanstack/react-router'
import { SidebarShell } from '../components/sidebar'

export const Route = createRootRoute({
  component: RootLayout,
})

function RootLayout() {
  return (
    <SidebarShell>
      <Outlet />
    </SidebarShell>
  )
}
