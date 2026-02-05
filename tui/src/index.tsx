import { createCliRenderer } from "@opentui/core";
import { createRoot } from "@opentui/react";
import { useState } from "react";

function App() {
	const [value, setValue] = useState("");

	return (
		<box flexDirection="column" flexGrow={1} padding={1} backgroundColor="#000000">
			<box height={4} backgroundColor="#428f84" />
			<box flexGrow={1} backgroundColor="#42cf84" />
			{/* Input Box*/}
			<box height={4} backgroundColor="#2a2a2a" padding={1}>
				<input
					value={value}
					onChange={setValue}
					placeholder="Type a message..."
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
