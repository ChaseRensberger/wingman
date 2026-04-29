import { createRootRoute, Outlet } from '@tanstack/react-router'
import { SidebarShell } from '../components/sidebar'
import { ThemeProvider } from '../components/theme-provider'

export const Route = createRootRoute({
  component: RootLayout,
})

function RootLayout() {
  return (
    <ThemeProvider defaultTheme="system" storageKey="wingman-ui-theme">
      <SidebarShell>
        <Outlet />
      </SidebarShell>
    </ThemeProvider>
  )
}
