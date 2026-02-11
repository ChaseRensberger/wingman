import { createRootRoute, Outlet } from '@tanstack/react-router'
import { ThemeProvider } from '@wingman/core/components/theme-provider'
import { Analytics } from '@vercel/analytics/next'

const RootLayout = () => (
	<>
		<Analytics />
		<ThemeProvider defaultTheme='system' storageKey='wingman-ui-theme'>
			<Outlet />
		</ThemeProvider>
	</>
)

export const Route = createRootRoute({ component: RootLayout })
