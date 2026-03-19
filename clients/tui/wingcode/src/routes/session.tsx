import { SyntaxStyle } from "@opentui/core";
import { useSession } from "../context/session";
import { MessageInput } from "../components/message-input";
import { theme } from "../theme";

const syntaxStyle = SyntaxStyle.create();
const MAX_TOOL_OUTPUT_LINES = 30;

function truncateOutput(output: string): { text: string; truncated: boolean } {
	const lines = output.split("\n");
	if (lines.length <= MAX_TOOL_OUTPUT_LINES) {
		return { text: output, truncated: false };
	}
	return {
		text: lines.slice(0, MAX_TOOL_OUTPUT_LINES).join("\n"),
		truncated: true,
	};
}

export function SessionView() {
	const session = useSession();

	return (
		<box flexDirection="column" flexGrow={1} paddingLeft={2} paddingRight={2} paddingTop={1} paddingBottom={1} gap={1}>
			<scrollbox flexGrow={1}>
				{session.messages.length === 0 ? (
					<box justifyContent="center" alignItems="center" flexGrow={1}>
						<text fg={theme.textMuted}>Send a message to get started...</text>
					</box>
				) : (
					session.messages.map((message, index) => (
						<box key={index} paddingBottom={1} flexDirection="column">
						{message.role === "tool" ? (
							<>
								<text fg={message.status === "running" ? theme.primary : theme.textMuted}>
									<strong>Tool</strong>
									{message.toolName ? `: ${message.toolName}` : ""}
									{message.status === "running" ? " (running...)" : ""}
								</text>
								{message.output && message.status === "done" ? (() => {
									const { text, truncated } = truncateOutput(message.output);
									return (
										<box flexDirection="column" paddingLeft={2}>
											<text fg={theme.textMuted}>{text}</text>
											{truncated ? (
												<text fg={theme.textMuted}>{"... (output truncated)"}</text>
											) : null}
										</box>
									);
								})() : null}
							</>
							) : (
								<>
									<text fg={message.role === "user" ? theme.primary : theme.text}>
										<strong>{message.role === "user" ? "You" : "WingCode"}</strong>
									</text>
									{message.role === "assistant" ? (
									<markdown
										syntaxStyle={syntaxStyle}
										streaming={true}
										content={message.content || " "}
									/>
									) : (
										<text fg={theme.text}>{message.content || " "}</text>
									)}
								</>
							)}
						</box>
					))
				)}
			</scrollbox>

			{session.error ? (
				<box>
					<text fg={theme.error}>{session.error}</text>
				</box>
			) : null}

			<MessageInput
				placeholder="Ask anything..."
				onSubmit={(text) => {
					session.sendMessage(text);
				}}
			/>
			<box flexDirection="row" justifyContent="space-between">
				<text fg={theme.textMuted}>{process.cwd()}</text>
				<text fg={theme.textMuted}>{session.status}</text>
			</box>
		</box>
	);
}
