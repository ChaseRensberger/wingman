import { Kbd } from "@/components/core/kbd"

export function KbdShowcase() {
  return (
    <section className="py-4 space-y-8">
      <h2 className="text-2xl font-semibold">Kbd</h2>
      <div className="space-y-4">
        <div className="flex flex-wrap gap-3 items-center">
          <Kbd>⌘</Kbd>
          <Kbd>K</Kbd>
          <Kbd>Enter</Kbd>
          <Kbd>Escape</Kbd>
          <Kbd>Tab</Kbd>
        </div>
        <div className="flex flex-wrap gap-1 items-center">
          <span className="text-sm text-muted-foreground">Open command palette:</span>
          <Kbd>⌘</Kbd>
          <span className="text-sm">+</span>
          <Kbd>K</Kbd>
        </div>
      </div>
    </section>
  )
}
