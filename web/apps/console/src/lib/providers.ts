import type { Provider } from "@/lib/types";

export function isProviderSelectable(provider: Provider) {
  return provider.auth.configured || !provider.route.auth_enabled || provider.auth.source === "disabled";
}
