import { NavigationMenu as NavigationMenuPrimitive } from "@base-ui/react/navigation-menu";
import { cn } from "#lib/utils";

function NavigationMenu({ className, ...props }: NavigationMenuPrimitive.Root.Props) {
  return (
    <NavigationMenuPrimitive.Root
      data-slot="navigation-menu"
      className={cn("relative flex items-center", className)}
      {...props}
    />
  );
}

function NavigationMenuList({ className, ...props }: NavigationMenuPrimitive.List.Props) {
  return (
    <NavigationMenuPrimitive.List
      data-slot="navigation-menu-list"
      className={cn("flex flex-1 list-none items-center justify-center gap-1", className)}
      {...props}
    />
  );
}

function NavigationMenuItem({ ...props }: NavigationMenuPrimitive.Item.Props) {
  return <NavigationMenuPrimitive.Item data-slot="navigation-menu-item" {...props} />;
}

function NavigationMenuTrigger({ className, ...props }: NavigationMenuPrimitive.Trigger.Props) {
  return (
    <NavigationMenuPrimitive.Trigger
      data-slot="navigation-menu-trigger"
      className={cn(
        "inline-flex h-9 items-center justify-center rounded-[var(--radius)] bg-background px-3 py-1 text-sm font-medium transition-[color,background-color,box-shadow] duration-150 hover:bg-accent hover:text-accent-foreground focus:bg-accent focus:text-accent-foreground focus:outline-none disabled:pointer-events-none disabled:opacity-50 data-active:bg-primary/15 data-popup-open:bg-primary/15",
        className,
      )}
      {...props}
    />
  );
}

function NavigationMenuContent({ className, ...props }: NavigationMenuPrimitive.Content.Props) {
  return (
    <NavigationMenuPrimitive.Portal>
      <NavigationMenuPrimitive.Positioner className="isolate z-50" sideOffset={4}>
        <NavigationMenuPrimitive.Viewport>
          <NavigationMenuPrimitive.Popup
            data-slot="navigation-menu-popup"
            className="w-auto min-w-48 rounded-[var(--radius)] border bg-popover p-1 shadow-sm"
          >
            <NavigationMenuPrimitive.Content
              data-slot="navigation-menu-content"
              className={cn("p-1", className)}
              {...props}
            />
          </NavigationMenuPrimitive.Popup>
        </NavigationMenuPrimitive.Viewport>
      </NavigationMenuPrimitive.Positioner>
    </NavigationMenuPrimitive.Portal>
  );
}

function NavigationMenuLink({ className, ...props }: NavigationMenuPrimitive.Link.Props) {
  return (
    <NavigationMenuPrimitive.Link
      data-slot="navigation-menu-link"
      className={cn("transition-colors hover:text-foreground/80", className)}
      {...props}
    />
  );
}

export {
  NavigationMenu,
  NavigationMenuList,
  NavigationMenuItem,
  NavigationMenuTrigger,
  NavigationMenuContent,
  NavigationMenuLink,
};
