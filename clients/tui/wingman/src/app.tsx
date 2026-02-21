import { useTerminalDimensions } from "@opentui/react";
import { theme } from "./theme";
import { SessionView } from "./routes/session";

export function App() {
	const { width, height } = useTerminalDimensions();

	return (
		<box width={width} height={height} backgroundColor={theme.background}>
			<SessionView />
		</box>
	);
}
