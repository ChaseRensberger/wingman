import { createRootRoute, Link, Outlet } from '@tanstack/react-router'
import { ThemeProvider } from '@wingman/core/components/theme-provider'
import { Analytics } from '@vercel/analytics/react'
import WingmanIcon from '../assets/WingmanBlue.png'

const RootLayout = () => (
	<>
		<Analytics />
		<ThemeProvider defaultTheme='system' storageKey='wingman-ui-theme'>
			<Outlet />
		</ThemeProvider>
	</>
)

function NotFound() {
	return (
		<main className="min-h-screen flex items-center justify-center p-8 font-mono">
			<div className="max-w-xl w-full border">
				<div className="flex flex-col items-center gap-4 p-8 sm:p-12">
					<Link to="/">
						<img src={WingmanIcon} className="w-16 h-16" alt="Wingman logo" />
					</Link>
					<h1 className="text-xl font-medium uppercase tracking-widest text-foreground">
						404 - Page Not Found
					</h1>
					<Link to="/" className='text-primary hover:underline hover:underline-offset-4'>
						Return Home
					</Link>
				</div>
			</div>
		</main>
	)
}

export const Route = createRootRoute({
	component: RootLayout,
	notFoundComponent: NotFound,
})
