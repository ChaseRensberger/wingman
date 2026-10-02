import { Link, Outlet, useRouterState } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import WingmanIcon from "@/assets/icon-128.png";
import { Button } from "@wingman/core/components/core/button";
import { Badge } from "@wingman/core/components/core/badge";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@wingman/core/components/core/sheet";
import { ListIcon } from "@phosphor-icons/react";
import { CommandPalette } from "@/components/command-palette";
import { DaemonConnectionBanner } from "@/components/daemon-connection";
import { useDaemonConnection } from "@/components/daemon-connection-context";
import { navItems, sessionsWorkspace } from "@/lib/navigation";
import { client as wingman } from "@/lib/client";
import { cn } from "@/lib/utils";
import { PwaLifecycle } from "@/components/pwa";

type Client = { id: string; name: string };

function useCurrentClient(hasConnected: boolean, revision: number) {
  const [client, setClient] = useState<Client>();

  useEffect(() => {
    if (!hasConnected) return;
    let cancelled = false;
    void wingman.current
      .client()
      .then((current) => {
        if (!cancelled) setClient(current);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [hasConnected, revision]);

  return client;
}

function CurrentClientBadge({
  client,
  variant,
}: {
  client?: Client;
  variant: "ghost" | "secondary";
}) {
  const label = client?.name ?? client?.id ?? "Client";
  return (
    <Badge
      className="h-8 px-2.5 sm:h-9"
      variant={variant}
      title={client ? `Current Wingman client: ${client.id}` : "Current Wingman client"}
    >
      Client: {label}
    </Badge>
  );
}

function NavLink({
  to,
  icon: Icon,
  label,
  className,
  onNavigate,
  workspace,
}: {
  to: string;
  icon: React.ComponentType<{ size?: number; className?: string }>;
  label: string;
  className?: string;
  onNavigate?: () => void;
  workspace?: string;
}) {
  const { location } = useRouterState();
  const isActive = location.pathname === to || location.pathname.startsWith(to + "/");

  return (
    <Button
      render={<Link to={to} search={to === "/sessions" ? { workspace } : {}} />}
      nativeButton={false}
      variant={isActive ? "default" : "outline"}
      size="lg"
      className={cn("gap-2 text-xs", !isActive && "text-muted-foreground", className)}
      onClick={onNavigate}
    >
      <Icon size={16} />
      {label}
    </Button>
  );
}

export default function App() {
  const { location } = useRouterState();
  const [navigationOpen, setNavigationOpen] = useState(false);
  const [lastWorkspace, setLastWorkspace] = useState<string>();
  const workspace = sessionsWorkspace(location.pathname, location.search.workspace, lastWorkspace);
  useEffect(() => {
    setLastWorkspace(workspace);
  }, [workspace]);
  const { revision, hasConnected } = useDaemonConnection();
  const client = useCurrentClient(hasConnected, revision);
  const isSessionDetail = /^\/sessions\/[^/]+$/.test(location.pathname);
  const activeNavItem = navItems.find(
    ({ to }) => location.pathname === to || location.pathname.startsWith(to + "/"),
  );

  return (
    <div
      className={cn(
        "console-shell flex flex-col",
        isSessionDetail ? "console-session" : "min-h-dvh",
      )}
    >
      <PwaLifecycle />
      <CommandPalette workspace={workspace} />
      <DaemonConnectionBanner />
      {!isSessionDetail && (
        <>
          <header className="flex items-center justify-between gap-4 border-b px-4 py-3 sm:hidden">
            <Link to="/" className="flex items-center gap-3">
              <img src={WingmanIcon} className="size-8" alt="Wingman logo" />
              <span className="text-sm font-medium">{activeNavItem?.label ?? "Wingman"}</span>
            </Link>
            <div className="flex items-center gap-2">
              <CurrentClientBadge client={client} variant="secondary" />
              <Sheet open={navigationOpen} onOpenChange={setNavigationOpen}>
                <SheetTrigger
                  render={
                    <Button variant="outline" size="icon" aria-label="Open navigation">
                      <ListIcon />
                    </Button>
                  }
                />
                <SheetContent className="w-full max-w-sm gap-6">
                  <SheetHeader>
                    <SheetTitle>Navigation</SheetTitle>
                  </SheetHeader>
                  <nav className="flex flex-col gap-2">
                    {navItems.map((item) => (
                      <NavLink
                        key={item.to}
                        {...item}
                        workspace={workspace}
                        className="w-full justify-start text-sm"
                        onNavigate={() => setNavigationOpen(false)}
                      />
                    ))}
                  </nav>
                </SheetContent>
              </Sheet>
            </div>
          </header>
          <header className="hidden items-center justify-between gap-4 border-b px-4 py-3 sm:flex">
            <div className="flex items-center gap-5">
              <Link to="/">
                <img src={WingmanIcon} className="size-8" alt="Wingman logo" />
              </Link>
              <nav className="flex items-center gap-3 text-xs text-muted-foreground">
                {navItems.map((item) => (
                  <NavLink key={item.to} {...item} workspace={workspace} />
                ))}
              </nav>
            </div>
            <CurrentClientBadge client={client} variant="ghost" />
          </header>
        </>
      )}
      <main className="flex-1 min-h-0">{hasConnected && <Outlet key={revision} />}</main>
    </div>
  );
}
