import { createFileRoute } from '@tanstack/react-router'
import WingmanIcon from "../../assets/WingmanBlue.png";

export const Route = createFileRoute('/docs/')({
	component: RouteComponent,
})

function RouteComponent() {
	return <Docs />
}

function Docs() {

	return (
		<div className="min-h-screen flex flex-col">
			{/* Header */}
			<div className="sticky top-0 bg-background flex items-center justify-between px-6 py-2 w-full border-b">
				<img src={WingmanIcon} className="w-12 h-12" />
			</div>
			{/* Sidebar */}
			<div className='flex-1 flex'>
				<nav className='p-4 border-r w-48'>
					<ul>
						<li>Intro</li>
						<li>Config</li>
						<li>Providers</li>
					</ul>
				</nav>
				{/* Main Content */}
				<main className='flex-1 p-4'>
					<h1>Doc Content</h1>
				</main>
			</div>
		</div>
	)

}
