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
				<div className="flex flex-col items-center gap-3 p-8 sm:p-12">
					<Link to="/">
						<img src={WingmanIcon} className="w-16 h-16" alt="Wingman logo" />
					</Link>
					<h1 className="text-xl font-medium uppercase tracking-widest text-foreground">
						404 - Page Not Found
					</h1>
				</div>
				<div className="border-t flex flex-col sm:flex-row">
					<Link
						to="/"
						className="flex-1 text-center py-3 px-4 uppercase text-foreground underline underline-offset-4 decoration-1 hover:text-muted-foreground transition-colors"
					>
						Home
					</Link>
					<Link
						to="/docs"
						className="flex-1 text-center py-3 px-4 uppercase text-foreground underline underline-offset-4 decoration-1 border-t sm:border-t-0 sm:border-l hover:text-muted-foreground transition-colors"
					>
						Docs
					</Link>
					<a
						href="https://github.com/chaserensberger/wingman"
						className="flex-1 text-center py-3 px-4 uppercase text-foreground underline underline-offset-4 decoration-1 border-t sm:border-t-0 sm:border-l hover:text-muted-foreground transition-colors"
					>
						GitHub
					</a>
				</div>
			</div>
		</main>
	)
}

export const Route = createRootRoute({
	component: RootLayout,
	notFoundComponent: NotFound,
})
