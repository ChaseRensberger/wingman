import { RadioGroup, RadioGroupItem } from "@/components/core/radio-group"
import { Field, FieldLabel } from "@/components/core/field"

export function RadioGroupShowcase() {
  return (
    <section className="py-4 space-y-8">
      <h2 className="text-2xl font-semibold">Radio Group</h2>
      <RadioGroup defaultValue="default">
        <Field orientation="horizontal">
          <RadioGroupItem value="default" id="default" />
          <FieldLabel htmlFor="default">Default</FieldLabel>
        </Field>
        <Field orientation="horizontal">
          <RadioGroupItem value="comfortable" id="comfortable" />
          <FieldLabel htmlFor="comfortable">Comfortable</FieldLabel>
        </Field>
        <Field orientation="horizontal">
          <RadioGroupItem value="compact" id="compact" />
          <FieldLabel htmlFor="compact">Compact</FieldLabel>
        </Field>
        <Field orientation="horizontal">
          <RadioGroupItem value="disabled" id="disabled" disabled />
          <FieldLabel htmlFor="disabled">Disabled</FieldLabel>
        </Field>
      </RadioGroup>
    </section>
  )
}
