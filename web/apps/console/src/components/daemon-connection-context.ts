import { createContext, useContext } from "react";

import type { DaemonConnectionPhase } from "@/lib/connection";

export type DaemonConnection = {
  phase: DaemonConnectionPhase;
  revision: number;
  hasConnected: boolean;
  failure?: string;
};

export const DaemonConnectionContext = createContext<DaemonConnection>({
  phase: "connecting",
  revision: 0,
  hasConnected: false,
});

export function useDaemonConnection(): DaemonConnection {
  return useContext(DaemonConnectionContext);
}
