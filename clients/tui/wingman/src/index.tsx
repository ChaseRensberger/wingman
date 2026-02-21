import { createCliRenderer } from "@opentui/core";
import { createRoot } from "@opentui/react";
import { theme } from "./theme";
import { MessageInput } from "./components/message-input";

function App() {
	return (
		<box flexGrow={1} backgroundColor={theme.background}>
			<box flexGrow={1} justifyContent="center" alignItems="center">
				<text fg={theme.text}>
					<strong>Wingman</strong>
				</text>
			</box>
			<box>
				<box paddingX={2} flexDirection="column" gap={2}>
					<MessageInput
						placeholder='Ask for anything...'
					/>
					<box flexDirection="row" justifyContent="space-between">
						<text fg={theme.textMuted}>{process.cwd()}</text>
						<box flexGrow={1} />
						<text fg={theme.textMuted}>v0.1.0</text>
					</box>
				</box>
			</box>
		</box>
	);
}

const renderer = await createCliRenderer();
createRoot(renderer).render(<App />);
