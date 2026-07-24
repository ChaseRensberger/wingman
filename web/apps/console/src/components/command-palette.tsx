import { useEffect, useEffectEvent, useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { Command } from "cmdk";
import { PlusIcon } from "@phosphor-icons/react";

function isEditableTarget(target: EventTarget | null) {
	return target instanceof HTMLElement && (
		target.isContentEditable ||
		["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName)
	);
}

export function CommandPalette() {
	const navigate = useNavigate();
	const [open, setOpen] = useState(false);
	const toggle = useEffectEvent(() => setOpen((current) => !current));

	useEffect(() => {
		function handleKeyDown(event: KeyboardEvent) {
			if (event.key.toLowerCase() !== "k" || (!event.metaKey && !event.ctrlKey)) return;
			if (!open && isEditableTarget(event.target)) return;
			event.preventDefault();
			toggle();
		}

		document.addEventListener("keydown", handleKeyDown);
		return () => document.removeEventListener("keydown", handleKeyDown);
	}, [open]);

	function createSession() {
		setOpen(false);
		navigate({ to: "/sessions/$sessionId", params: { sessionId: "new" } });
	}

	return (
		<Command.Dialog open={open} onOpenChange={setOpen} label="Command menu">
			<Command.Input placeholder="Type a command..." />
			<Command.List>
				<Command.Empty>No commands found.</Command.Empty>
				<Command.Group heading="Actions">
					<Command.Item value="new session" onSelect={createSession}>
						<PlusIcon className="size-4" />
						<span>New session</span>
					</Command.Item>
				</Command.Group>
			</Command.List>
		</Command.Dialog>
	);
}
