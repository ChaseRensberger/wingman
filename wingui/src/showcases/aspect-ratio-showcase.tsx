import { AspectRatio } from "@/components/core/aspect-ratio"

export function AspectRatioShowcase() {
  return (
    <section className="py-4 space-y-8">
      <h2 className="text-2xl font-semibold">Aspect Ratio</h2>
      <div className="space-y-4 max-w-sm">
        <div>
          <p className="text-sm text-muted-foreground mb-2">16:9</p>
          <AspectRatio ratio={16 / 9} className="rounded-lg overflow-hidden">
            <div className="w-full h-full bg-muted flex items-center justify-center text-muted-foreground text-sm">
              16 / 9
            </div>
          </AspectRatio>
        </div>
        <div>
          <p className="text-sm text-muted-foreground mb-2">1:1</p>
          <AspectRatio ratio={1} className="rounded-lg overflow-hidden">
            <div className="w-full h-full bg-muted flex items-center justify-center text-muted-foreground text-sm">
              1 / 1
            </div>
          </AspectRatio>
        </div>
      </div>
    </section>
  )
}
