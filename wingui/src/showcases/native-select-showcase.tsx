import { NativeSelect } from "@/components/core/native-select"

export function NativeSelectShowcase() {
  return (
    <section className="py-4 space-y-8">
      <h2 className="text-2xl font-semibold">Native Select</h2>
      <div className="space-y-4 max-w-sm">
        <NativeSelect>
          <option value="">Select a framework</option>
          <option value="react">React</option>
          <option value="vue">Vue</option>
          <option value="angular">Angular</option>
          <option value="svelte">Svelte</option>
        </NativeSelect>
        <NativeSelect disabled>
          <option>Disabled</option>
        </NativeSelect>
      </div>
    </section>
  )
}
