import {
  Sheet,
  SheetTrigger,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
  SheetFooter,
} from "@/components/core/sheet"
import { Button } from "@/components/core/button"

export function SheetShowcase() {
  return (
    <section className="py-4 space-y-8">
      <h2 className="text-2xl font-semibold">Sheet</h2>
      <div className="flex flex-wrap gap-3">
        {(["left", "right", "top", "bottom"] as const).map((side) => (
          <Sheet key={side}>
            <SheetTrigger render={<Button variant="outline">{side.charAt(0).toUpperCase() + side.slice(1)}</Button>} />
            <SheetContent side={side}>
              <SheetHeader>
                <SheetTitle>Sheet ({side})</SheetTitle>
                <SheetDescription>
                  A panel that slides in from the {side}.
                </SheetDescription>
              </SheetHeader>
              <p className="text-sm text-muted-foreground">
                Add your content here. Forms, navigation, or any other content.
              </p>
              <SheetFooter>
                <Button variant="outline">Cancel</Button>
                <Button>Save changes</Button>
              </SheetFooter>
            </SheetContent>
          </Sheet>
        ))}
      </div>
    </section>
  )
}
