import { FileTextIcon, GearIcon, LightningIcon, SolarRoofIcon, StackIcon, UsersIcon, WrenchIcon } from "@phosphor-icons/react";

export const navItems = [
	{ to: "/clients", icon: UsersIcon, label: "Clients" },
	{ to: "/sessions", icon: StackIcon, label: "Sessions" },
	{ to: "/agents", icon: LightningIcon, label: "Agents" },
	{ to: "/tools", icon: WrenchIcon, label: "Tools" },
	{ to: "/providers", icon: SolarRoofIcon, label: "Providers" },
	{ to: "/logs", icon: FileTextIcon, label: "Logs" },
	{ to: "/settings", icon: GearIcon, label: "Settings" },
] as const;
