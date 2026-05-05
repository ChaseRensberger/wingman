import { Label } from "@/components/core/label"
import { Checkbox } from "@/components/core/checkbox"
import { Input } from "@/components/core/input"

export function LabelShowcase() {
  return (
    <section className="py-4 space-y-8">
      <h2 className="text-2xl font-semibold">Label</h2>
      <div className="space-y-4">
        <div className="flex items-center gap-2">
          <Checkbox id="terms" />
          <Label htmlFor="terms">Accept terms and conditions</Label>
        </div>
        <div className="flex flex-col gap-1.5 max-w-xs">
          <Label htmlFor="email">Email address</Label>
          <Input id="email" type="email" placeholder="you@example.com" />
        </div>
      </div>
    </section>
  )
}
