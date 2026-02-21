import { useRef } from "react";
import type { TextareaRenderable } from "@opentui/core";
import { theme } from "../theme";

export function MessageInput(props: {
	onSubmit?: (text: string) => void;
	placeholder?: string;
}) {
	const textareaRef = useRef<TextareaRenderable>(null);

	return (
		<box
			border={["left"] as const}
			borderColor={theme.primary}
			paddingX={2}
			paddingTop={1}
			backgroundColor={theme.backgroundElement}
			flexGrow={1}
			flexShrink={0}
			maxHeight={5}
			width="100%"
		>
			<textarea
				ref={textareaRef}
				placeholder={props.placeholder || "Ask anything..."}
				textColor={theme.text}
				cursorColor={theme.text}
				focusedBackgroundColor={theme.backgroundElement}
				focused
				keyBindings={[
					{ name: "return", action: "submit" as const },
					{ name: "return", meta: true, action: "newline" as const },
				]}
				onSubmit={() => {
					const ta = textareaRef.current;
					if (!ta || ta.isDestroyed) return;
					const text = ta.plainText.trim();
					if (!text) return;
					props.onSubmit?.(text);
					ta.clear();
				}}
			/>
		</box>
	);
}
