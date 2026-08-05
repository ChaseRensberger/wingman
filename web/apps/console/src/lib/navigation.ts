import { GearIcon, LightningIcon, SolarRoofIcon, StackIcon, WrenchIcon } from "@phosphor-icons/react";

export const navItems = [
	{ to: "/sessions", icon: StackIcon, label: "Sessions" },
	{ to: "/agents", icon: LightningIcon, label: "Agents" },
	{ to: "/tools", icon: WrenchIcon, label: "Tools" },
	{ to: "/providers", icon: SolarRoofIcon, label: "Providers" },
	{ to: "/settings", icon: GearIcon, label: "Settings" },
] as const;
