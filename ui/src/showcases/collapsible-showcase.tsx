import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/core/collapsible"

export function CollapsibleShowcase() {
  return (
    <section className="py-4 space-y-8">
      <h2 className="text-2xl font-semibold">Collapsible</h2>
      <Collapsible defaultOpen className="max-w-lg border rounded-lg px-4">
        <CollapsibleTrigger>Order #4189</CollapsibleTrigger>
        <CollapsibleContent>
          <div className="space-y-1 pb-1 text-muted-foreground">
            <div className="flex justify-between">
              <span>Status</span>
              <span>Shipped</span>
            </div>
            <div className="flex justify-between">
              <span>Estimated delivery</span>
              <span>May 5, 2026</span>
            </div>
            <div className="flex justify-between">
              <span>Tracking number</span>
              <span>1Z999AA10123456784</span>
            </div>
          </div>
        </CollapsibleContent>
      </Collapsible>
    </section>
  )
}
