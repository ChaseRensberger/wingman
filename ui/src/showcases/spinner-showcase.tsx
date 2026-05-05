import { Spinner } from "@/components/core/spinner"

export function SpinnerShowcase() {
  return (
    <section className="py-4 space-y-8">
      <h2 className="text-2xl font-semibold">Spinner</h2>
      <div className="flex flex-wrap gap-4 items-center">
        <Spinner size="sm" />
        <Spinner size="default" />
        <Spinner size="lg" />
        <Spinner size="xl" />
      </div>
    </section>
  )
}
