import { useEffect, useEffectEvent, useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { Command } from "cmdk";
import { CheckIcon, PlusIcon } from "@phosphor-icons/react";
import { useTheme } from "@wingman/core/components/theme-provider";
import { themes } from "@wingman/core/themes/registry";
import { navItems } from "@/lib/navigation";

function isEditableTarget(target: EventTarget | null) {
	return target instanceof HTMLElement && (
		target.isContentEditable ||
		["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName)
	);
}

export function CommandPalette() {
	const navigate = useNavigate();
	const { theme, colorMode, resolvedColorMode, setColorMode, setTheme } = useTheme();
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

	function navigateTo(to: (typeof navItems)[number]["to"]) {
		setOpen(false);
		navigate({ to });
	}

	function selectTheme(themeID: typeof theme.id, colorMode: "light" | "dark") {
		setTheme(themeID, colorMode);
		setOpen(false);
	}

	function selectSystemColorMode() {
		setColorMode("system");
		setOpen(false);
	}

	return (
		<Command.Dialog open={open} onOpenChange={setOpen} label="Command menu">
			<Command.Input placeholder="Type a command..." />
			<Command.List>
				<Command.Empty>No commands found.</Command.Empty>
				<Command.Group heading="Navigation">
					{navItems.map(({ to, icon: Icon, label }) => (
						<Command.Item key={to} value={label} onSelect={() => navigateTo(to)}>
							<Icon className="size-4" />
							<span>{label}</span>
						</Command.Item>
					))}
				</Command.Group>
				<Command.Group heading="Actions">
					<Command.Item value="new session" onSelect={createSession}>
						<PlusIcon className="size-4" />
						<span>New session</span>
					</Command.Item>
				</Command.Group>
				<Command.Group heading="Theme">
					{themes.flatMap((option) => option.modes.map((colorMode) => {
						const active = theme.id === option.id && resolvedColorMode === colorMode;
						return (
							<Command.Item key={`${option.id}-${colorMode}`} value={`${option.label} ${colorMode}`} onSelect={() => selectTheme(option.id, colorMode)}>
								<span>{option.label} {colorMode}</span>
								{active && <CheckIcon className="ml-auto size-4" weight="bold" />}
							</Command.Item>
						);
					}))}
					{theme.modes.length === 2 && (
						<Command.Item value="system preference" onSelect={selectSystemColorMode}>
							<span>System preference</span>
							{colorMode === "system" && <CheckIcon className="ml-auto size-4" weight="bold" />}
						</Command.Item>
					)}
				</Command.Group>
			</Command.List>
		</Command.Dialog>
	);
}
