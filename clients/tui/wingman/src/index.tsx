import { createCliRenderer } from "@opentui/core";
import { createRoot } from "@opentui/react";
import { theme } from "./theme";
import { MessageInput } from "./components/message-input";

function App() {
	return (
		<box flexGrow={1} backgroundColor={theme.background}>
			<box flexGrow={1} justifyContent="center" alignItems="center" paddingLeft={2} paddingRight={2} gap={1}>
				<text fg={theme.text}>
					<strong>Wingman</strong>
				</text>
				<MessageInput
					placeholder='Ask anything... "Fix a TODO in the codebase"'
				/>
			</box>
			<box paddingTop={1} paddingBottom={1} paddingLeft={2} paddingRight={2} flexDirection="row" flexShrink={0}>
				<text fg={theme.textMuted}>{process.cwd()}</text>
				<box flexGrow={1} />
				<text fg={theme.textMuted}>v0.1.0</text>
			</box>
		</box>
	);
}

const renderer = await createCliRenderer();
createRoot(renderer).render(<App />);
