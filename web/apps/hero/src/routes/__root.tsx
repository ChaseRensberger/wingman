import { createRootRoute, Outlet } from '@tanstack/react-router'
import { ThemeProvider } from '@wingman/core/components/theme-provider'

const RootLayout = () => (
	<>
		<ThemeProvider defaultTheme='system' storageKey='wingman-ui-theme'>
			<Outlet />
		</ThemeProvider>
	</>
)

export const Route = createRootRoute({ component: RootLayout })
