import * as React from "react";
import { cn } from "#lib/utils";

interface SidebarContextValue {
  open: boolean;
  setOpen: (open: boolean) => void;
}

const SidebarContext = React.createContext<SidebarContextValue>({
  open: true,
  setOpen: () => {},
});

function useSidebar() {
  return React.useContext(SidebarContext);
}

function SidebarProvider({
  defaultOpen = true,
  open: controlledOpen,
  onOpenChange,
  children,
  className,
  ...props
}: React.ComponentProps<"div"> & {
  defaultOpen?: boolean;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
}) {
  const [internalOpen, setInternalOpen] = React.useState(defaultOpen);
  const open = controlledOpen ?? internalOpen;
  const setOpen = (value: boolean) => {
    setInternalOpen(value);
    onOpenChange?.(value);
  };

  return (
    <SidebarContext.Provider value={{ open, setOpen }}>
      <div
        data-slot="sidebar-provider"
        data-sidebar-open={open}
        className={cn("flex min-h-svh w-full", className)}
        {...props}
      >
        {children}
      </div>
    </SidebarContext.Provider>
  );
}

function Sidebar({ className, children, ...props }: React.ComponentProps<"aside">) {
  const { open } = useSidebar();
  return (
    <aside
      data-slot="sidebar"
      data-open={open}
      className={cn(
        "flex flex-col border-r bg-card text-card-foreground transition-[width] duration-150",
        open ? "w-64" : "w-14",
        className,
      )}
      {...props}
    >
      {children}
    </aside>
  );
}

function SidebarHeader({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="sidebar-header"
      className={cn("flex items-center gap-2 border-b px-3 py-3 h-14", className)}
      {...props}
    />
  );
}

function SidebarContent({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="sidebar-content"
      className={cn("flex flex-1 flex-col gap-1 overflow-y-auto p-2", className)}
      {...props}
    />
  );
}

function SidebarFooter({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="sidebar-footer"
      className={cn("flex items-center border-t px-3 py-3 h-14", className)}
      {...props}
    />
  );
}

function SidebarGroup({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div data-slot="sidebar-group" className={cn("flex flex-col gap-1", className)} {...props} />
  );
}

function SidebarGroupLabel({ className, ...props }: React.ComponentProps<"span">) {
  const { open } = useSidebar();
  return (
    <span
      data-slot="sidebar-group-label"
      className={cn(
        "px-2 py-1 text-xs font-medium uppercase tracking-widest text-muted-foreground transition-opacity",
        !open && "opacity-0",
        className,
      )}
      {...props}
    />
  );
}

function SidebarItem({
  className,
  active,
  ...props
}: React.ComponentProps<"div"> & { active?: boolean }) {
  return (
    <div
      data-slot="sidebar-item"
      data-active={active}
      className={cn(
        "flex select-none items-center gap-2.5 rounded-[var(--radius)] px-2.5 py-1.5 text-sm",
        active && "bg-primary/15 text-primary",
        className,
      )}
      {...props}
    />
  );
}

function SidebarItemIcon({ className, ...props }: React.ComponentProps<"span">) {
  return (
    <span
      data-slot="sidebar-item-icon"
      className={cn("shrink-0 [&_svg]:size-4", className)}
      {...props}
    />
  );
}

function SidebarItemLabel({ className, ...props }: React.ComponentProps<"span">) {
  const { open } = useSidebar();
  return (
    <span
      data-slot="sidebar-item-label"
      className={cn(
        "truncate transition-opacity",
        !open && "opacity-0 w-0 overflow-hidden",
        className,
      )}
      {...props}
    />
  );
}

function SidebarTrigger({ className, ...props }: React.ComponentProps<"button">) {
  const { open, setOpen } = useSidebar();
  return (
    <button
      data-slot="sidebar-trigger"
      onClick={() => setOpen(!open)}
      className={cn(
        "inline-flex items-center justify-center rounded-[var(--radius)] p-1.5 text-muted-foreground transition-[color,background-color,box-shadow] duration-150 hover:bg-accent hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50",
        className,
      )}
      aria-label={open ? "Collapse sidebar" : "Expand sidebar"}
      {...props}
    />
  );
}

function SidebarInset({ className, ...props }: React.ComponentProps<"main">) {
  return (
    <main
      data-slot="sidebar-inset"
      className={cn("flex flex-1 flex-col overflow-hidden", className)}
      {...props}
    />
  );
}

export {
  SidebarProvider,
  Sidebar,
  SidebarHeader,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarItem,
  SidebarItemIcon,
  SidebarItemLabel,
  SidebarTrigger,
  SidebarInset,
};
