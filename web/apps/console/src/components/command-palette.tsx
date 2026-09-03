import { useEffect, useEffectEvent, useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { Command } from "cmdk";
import { ArrowClockwiseIcon, CheckIcon, PlusIcon } from "@phosphor-icons/react";
import { useTheme } from "@wingman/core/components/theme-context";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@wingman/core/components/core/alert-dialog";
import { themes } from "@wingman/core/themes/registry";
import { navItems } from "@/lib/navigation";
import { client, restartService } from "@/lib/client";
import { useDaemonConnection } from "@/components/daemon-connection-context";
import { showErrorToast } from "@/lib/toast";

function isEditableTarget(target: EventTarget | null) {
  return (
    target instanceof HTMLElement &&
    (target.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName))
  );
}

export function CommandPalette() {
  const navigate = useNavigate();
  const { theme, colorMode, setColorMode, setTheme } = useTheme();
  const connection = useDaemonConnection();
  const [open, setOpen] = useState(false);
  const [confirmRestart, setConfirmRestart] = useState(false);
  const [restartAvailable, setRestartAvailable] = useState(false);
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

  useEffect(() => {
    if (!connection.hasConnected || connection.phase !== "live") return;
    let cancelled = false;
    void client.current
      .service()
      .then((service) => {
        if (!cancelled) setRestartAvailable(service.restart_available);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [connection.hasConnected, connection.phase, connection.revision]);

  function createSession() {
    setOpen(false);
    navigate({ to: "/sessions/$sessionId", params: { sessionId: "new" } });
  }

  function navigateTo(to: (typeof navItems)[number]["to"]) {
    setOpen(false);
    navigate({ to });
  }

  function selectTheme(themeID: typeof theme.id) {
    setTheme(themeID);
    setOpen(false);
  }

  function selectColorMode(colorMode: "light" | "dark" | "system") {
    setColorMode(colorMode);
    setOpen(false);
  }

  function selectRestart() {
    setOpen(false);
    setConfirmRestart(true);
  }

  async function confirmServiceRestart() {
    setConfirmRestart(false);
    try {
      await restartService();
    } catch (error) {
      showErrorToast(error, "Could not restart service");
    }
  }

  return (
    <>
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
            {restartAvailable && connection.phase === "live" && (
              <Command.Item value="restart service" onSelect={selectRestart}>
                <ArrowClockwiseIcon className="size-4" />
                <span>Restart service</span>
              </Command.Item>
            )}
          </Command.Group>
          <Command.Group heading="Settings">
            {themes.map((option) => (
              <Command.Item
                key={option.id}
                value={`change theme ${option.label}`}
                onSelect={() => selectTheme(option.id)}
              >
                <span>Change theme &gt; {option.label}</span>
                {theme.id === option.id && <CheckIcon className="ml-auto size-4" weight="bold" />}
              </Command.Item>
            ))}
            {[...theme.modes, "system" as const].map((mode) => (
              <Command.Item
                key={mode}
                value={`change color mode ${mode}`}
                onSelect={() => selectColorMode(mode)}
              >
                <span>
                  Change color mode &gt; {mode[0].toUpperCase()}
                  {mode.slice(1)}
                </span>
                {colorMode === mode && <CheckIcon className="ml-auto size-4" weight="bold" />}
              </Command.Item>
            ))}
          </Command.Group>
        </Command.List>
      </Command.Dialog>
      <AlertDialog open={confirmRestart} onOpenChange={setConfirmRestart}>
        <AlertDialogContent size="sm">
          <AlertDialogHeader>
            <AlertDialogTitle>Restart Wingman?</AlertDialogTitle>
            <AlertDialogDescription>
              Active runs will be aborted. Queued runs will resume when the service returns.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={confirmServiceRestart}>Restart service</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
