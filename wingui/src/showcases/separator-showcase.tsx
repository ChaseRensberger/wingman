import { Separator } from "@/components/core/separator"

export function SeparatorShowcase() {
  return (
    <section className="py-4 space-y-8">
      <h2 className="text-2xl font-semibold">Separator</h2>
      <div className="space-y-4">
        <div className="max-w-sm space-y-1">
          <p className="text-sm font-medium">WingUI</p>
          <p className="text-sm text-muted-foreground">A component library.</p>
          <Separator className="my-3" />
          <div className="flex h-4 items-center gap-3 text-sm">
            <span>Blog</span>
            <Separator orientation="vertical" />
            <span>Docs</span>
            <Separator orientation="vertical" />
            <span>Source</span>
          </div>
        </div>
      </div>
    </section>
  )
}
