import { createCliRenderer } from "@opentui/core";
import { createRoot } from "@opentui/react";
import { useState } from "react";
import { logo } from "./logo";

function App() {
	const [value, setValue] = useState("");

	return (
		<box flexDirection="column" flexGrow={1} padding={1}>
			<box height={4} backgroundColor="#2a2a2a" />
			<box flexGrow={1} justifyContent="center" alignItems="center" borderColor="#FFFFFF">
				<text fg="#0088ff">{logo}</text>
			</box>
			<box height={4} backgroundColor="#2a2a2a" padding={1}>
				<input
					value={value}
					onChange={setValue}
					placeholder="Ask anything..."
					focused
					flexGrow={1}
					textColor="#ffffff"
					backgroundColor="#2a2a2a"
					focusedBackgroundColor="#2a2a2a"
				/>
			</box>
		</box>
	);
}

const renderer = await createCliRenderer();
createRoot(renderer).render(<App />);
