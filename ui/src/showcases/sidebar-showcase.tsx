import { useState } from "react"
import {
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
} from "@/components/core/sidebar"
import {
  HouseIcon,
  ChartBarIcon,
  GearIcon,
  UsersIcon,
  FileIcon,
  SidebarSimpleIcon,
} from "@phosphor-icons/react"

const navItems = [
  { icon: HouseIcon, label: "Dashboard", active: true },
  { icon: ChartBarIcon, label: "Analytics" },
  { icon: UsersIcon, label: "Users" },
  { icon: FileIcon, label: "Documents" },
]

export function SidebarShowcase() {
  const [open, setOpen] = useState(true)

  return (
    <section className="py-4 space-y-8">
      <h2 className="text-2xl font-semibold">Sidebar</h2>
      <div className="rounded-lg border overflow-hidden h-72">
        <SidebarProvider open={open} onOpenChange={setOpen}>
          <Sidebar>
            <SidebarHeader>
              <SidebarTrigger>
                <SidebarSimpleIcon className="size-4" />
              </SidebarTrigger>
              {open && <span className="font-semibold text-sm">WingUI</span>}
            </SidebarHeader>
            <SidebarContent>
              <SidebarGroup>
                <SidebarGroupLabel>Navigation</SidebarGroupLabel>
                {navItems.map((item) => (
                  <SidebarItem key={item.label} active={item.active}>
                    <SidebarItemIcon><item.icon /></SidebarItemIcon>
                    <SidebarItemLabel>{item.label}</SidebarItemLabel>
                  </SidebarItem>
                ))}
              </SidebarGroup>
            </SidebarContent>
            <SidebarFooter>
              <SidebarItem>
                <SidebarItemIcon><GearIcon /></SidebarItemIcon>
                <SidebarItemLabel>Settings</SidebarItemLabel>
              </SidebarItem>
            </SidebarFooter>
          </Sidebar>
          <SidebarInset>
            <div className="flex items-center justify-center h-full text-sm text-muted-foreground">
              Main content area
            </div>
          </SidebarInset>
        </SidebarProvider>
      </div>
    </section>
  )
}
