import { useSession } from "../context/session";
import { MessageInput } from "../components/message-input";
import { theme } from "../theme";

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
								<text fg={theme.textMuted}>
									<strong>Tool</strong>
									{message.toolName ? `: ${message.toolName}` : ""}
								</text>
							) : (
								<>
									<text fg={message.role === "user" ? theme.primary : theme.text}>
										<strong>{message.role === "user" ? "You" : "WingCode"}</strong>
									</text>
									{message.role === "assistant" ? (
										<code
											filetype="markdown"
											drawUnstyledText={false}
											streaming={true}
											content={message.content || " "}
											fg={theme.text}
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
