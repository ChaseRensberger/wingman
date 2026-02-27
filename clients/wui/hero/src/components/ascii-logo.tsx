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
			<pre className="mx-auto w-fit whitespace-pre text-center font-mono text-[0.7rem] leading-tight text-primary md:text-[0.875rem]">
				{logo.join("\n")}
			</pre>
		</div>
	)
}
