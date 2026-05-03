import { Textarea } from "@/components/core/textarea"

export function TextareaShowcase() {
  return (
    <section className="py-4 space-y-8">
      <h2 className="text-2xl font-semibold">Textarea</h2>
      <div className="space-y-4 max-w-sm">
        <Textarea placeholder="Enter your message" />
        <Textarea placeholder="Disabled textarea" disabled />
        <Textarea placeholder="Invalid textarea" aria-invalid />
      </div>
    </section>
  )
}
