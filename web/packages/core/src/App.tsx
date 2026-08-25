import { ThemeToggle } from "./components/theme-toggle";
import { ThemePreviewSwitcherShowcase } from "./showcases/theme-preview-switcher-showcase";
import { AccordionShowcase } from "./showcases/accordion-showcase";
import { CollapsibleShowcase } from "./showcases/collapsible-showcase";
import { AlertShowcase } from "./showcases/alert-showcase";
import { AlertDialogShowcase } from "./showcases/alert-dialog-showcase";
import { DialogShowcase } from "./showcases/dialog-showcase";
import { AvatarShowcase } from "./showcases/avatar-showcase";
import { BadgeShowcase } from "./showcases/badge-showcase";
import { CardShowcase } from "./showcases/card-showcase";
import { ButtonShowcase } from "./showcases/button-showcase";
import { ButtonGroupShowcase } from "./showcases/button-group-showcase";
import { TypographyShowcase } from "./showcases/typography-showcase";
import { MarkdownShowcase } from "./showcases/markdown-showcase";
import { SpinnerShowcase } from "./showcases/spinner-showcase";
import { KbdShowcase } from "./showcases/kbd-showcase";
import { AspectRatioShowcase } from "./showcases/aspect-ratio-showcase";
import { EmptyShowcase } from "./showcases/empty-showcase";
import { ItemShowcase } from "./showcases/item-showcase";
import { CheckboxShowcase } from "./showcases/checkbox-showcase";
import { RadioGroupShowcase } from "./showcases/radio-group-showcase";
import { SwitchShowcase } from "./showcases/switch-showcase";
import { ToggleShowcase } from "./showcases/toggle-showcase";
import { ToggleGroupShowcase } from "./showcases/toggle-group-showcase";
import { DrawerShowcase } from "./showcases/drawer-showcase";
import { SheetShowcase } from "./showcases/sheet-showcase";
import { DropdownMenuShowcase } from "./showcases/dropdown-menu-showcase";
import { ContextMenuShowcase } from "./showcases/context-menu-showcase";
import { HoverCardShowcase } from "./showcases/hover-card-showcase";
import { ToastShowcase } from "./showcases/toast-showcase";
import { FieldShowcase } from "./showcases/field-showcase";
import { InputShowcase } from "./showcases/input-showcase";
import { InputGroupShowcase } from "./showcases/input-group-showcase";
import { TextareaShowcase } from "./showcases/textarea-showcase";
import { LabelShowcase } from "./showcases/label-showcase";
import { PopoverShowcase } from "./showcases/popover-showcase";
import { SeparatorShowcase } from "./showcases/separator-showcase";
import { TooltipShowcase } from "./showcases/tooltip-showcase";
import { ProgressShowcase } from "./showcases/progress-showcase";
import { SliderShowcase } from "./showcases/slider-showcase";
import { SkeletonShowcase } from "./showcases/skeleton-showcase";
import { SelectShowcase } from "./showcases/select-showcase";
import { ComboboxShowcase } from "./showcases/combobox-showcase";
import { NativeSelectShowcase } from "./showcases/native-select-showcase";
import { BreadcrumbShowcase } from "./showcases/breadcrumb-showcase";
import { PaginationShowcase } from "./showcases/pagination-showcase";
import { TableShowcase } from "./showcases/table-showcase";
import { DataTableShowcase } from "./showcases/data-table-showcase";
import { TabsShowcase } from "./showcases/tabs-showcase";
import { ScrollAreaShowcase } from "./showcases/scroll-area-showcase";
import { MenubarShowcase } from "./showcases/menubar-showcase";
import { NavigationMenuShowcase } from "./showcases/navigation-menu-showcase";
import { InputOTPShowcase } from "./showcases/input-otp-showcase";
import { SidebarShowcase } from "./showcases/sidebar-showcase";
import WingmanLogo from "#assets/WingmanBlue.png";
import { Button } from "./components/core/button";
import { GithubLogoIcon } from "@phosphor-icons/react";

export default function App() {
  return (
    <main className="min-h-screen bg-background text-foreground font-mono">
      <nav className="sticky-header-glass sticky top-0 z-50 flex items-center justify-between border-b p-4">
        <div className="flex items-center gap-2">
          <img src={WingmanLogo} className="h-8 w-8" />
          <span className="font-medium"> WingUI</span>
        </div>
        <div className="flex items-center gap-2">
          <ThemeToggle />
          <a href="https://github.com/wingman-actor/ui">
            <Button variant="outline" className="w-8 h-8">
              <GithubLogoIcon />
            </Button>
          </a>
        </div>
      </nav>
      <div className="mx-auto max-w-5xl divide-y divide-border px-8 py-4">
        <ButtonShowcase />
        <ButtonGroupShowcase />
        <TypographyShowcase />
        <ThemePreviewSwitcherShowcase />
        <MarkdownShowcase />
        <InputShowcase />
        <InputGroupShowcase />
        <TextareaShowcase />
        <SelectShowcase />
        <ComboboxShowcase />
        <InputOTPShowcase />
        <NativeSelectShowcase />
        <LabelShowcase />
        <FieldShowcase />
        <CheckboxShowcase />
        <RadioGroupShowcase />
        <SwitchShowcase />
        <ToggleShowcase />
        <ToggleGroupShowcase />
        <DropdownMenuShowcase />
        <MenubarShowcase />
        <NavigationMenuShowcase />
        <ContextMenuShowcase />
        <HoverCardShowcase />
        <BadgeShowcase />
        <CardShowcase />
        <AvatarShowcase />
        <ProgressShowcase />
        <SliderShowcase />
        <SkeletonShowcase />
        <SpinnerShowcase />
        <TooltipShowcase />
        <PopoverShowcase />
        <AlertShowcase />
        <AlertDialogShowcase />
        <DialogShowcase />
        <AccordionShowcase />
        <CollapsibleShowcase />
        <TabsShowcase />
        <DrawerShowcase />
        <SheetShowcase />
        <ScrollAreaShowcase />
        <ToastShowcase />
        <SeparatorShowcase />
        <KbdShowcase />
        <AspectRatioShowcase />
        <EmptyShowcase />
        <ItemShowcase />
        <BreadcrumbShowcase />
        <PaginationShowcase />
        <TableShowcase />
        <SidebarShowcase />
        <DataTableShowcase />
      </div>
    </main>
  );
}
