import { createContext, useContext, createSignal, type JSX } from "solid-js"
import type { RouteData } from "../types"

const RouteContext = createContext<{
  data: () => RouteData
  navigate: (to: RouteData) => void
}>()

export function RouteProvider(props: { children: JSX.Element }) {
  const [data, setData] = createSignal<RouteData>({ type: "home" })

  return (
    <RouteContext.Provider value={{ data, navigate: setData }}>
      {props.children}
    </RouteContext.Provider>
  )
}

export function useRoute() {
  const ctx = useContext(RouteContext)
  if (!ctx) throw new Error("useRoute must be used within RouteProvider")
  return ctx
}
