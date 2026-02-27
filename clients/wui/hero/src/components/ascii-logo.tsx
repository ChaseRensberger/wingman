export function ASCIILOGO() {
	const logo = [
		"██╗    ██╗██╗███╗   ██╗ ██████╗ ███╗   ███╗ █████╗ ███╗   ██╗",
		"██║    ██║██║████╗  ██║██╔════╝ ████╗ ████║██╔══██╗████╗  ██║",
		"██║ █╗ ██║██║██╔██╗ ██║██║  ███╗██╔████╔██║███████║██╔██╗ ██║",
		"██║███╗██║██║██║╚██╗██║██║   ██║██║╚██╔╝██║██╔══██║██║╚██╗██║",
		"╚███╔███╔╝██║██║ ╚████║╚██████╔╝██║ ╚═╝ ██║██║  ██║██║ ╚████║",
		" ╚══╝╚══╝ ╚═╝╚═╝  ╚═══╝ ╚═════╝ ╚═╝     ╚═╝╚═╝  ╚═╝╚═╝  ╚═══╝",
	]

	return (
		<div className="mx-auto w-full max-w-full overflow-x-auto">
			<pre className="mx-auto w-fit whitespace-pre text-center font-mono text-[0.5rem] sm:text-[0.6rem] md:text-[0.7rem] lg:text-[0.875rem] leading-tight text-primary">
				{logo.join("\n")}
			</pre>
		</div>
	)
}
