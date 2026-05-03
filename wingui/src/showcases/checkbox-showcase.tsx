import { Checkbox } from "@/components/core/checkbox"
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from "@/components/core/field"

export function CheckboxShowcase() {
  return (
    <section className="py-4 space-y-8">
      <h2 className="text-2xl font-semibold">Checkbox</h2>
      <FieldGroup>
        <Field orientation="horizontal">
          <Checkbox id="terms" />
          <FieldLabel htmlFor="terms">Accept terms and conditions</FieldLabel>
        </Field>
        <Field orientation="horizontal">
          <Checkbox id="notifications" defaultChecked />
          <FieldContent>
            <FieldLabel htmlFor="notifications">Enable notifications</FieldLabel>
            <FieldDescription>
              You can enable or disable notifications at any time.
            </FieldDescription>
          </FieldContent>
        </Field>
        <Field orientation="horizontal">
          <Checkbox id="disabled" disabled />
          <FieldLabel htmlFor="disabled">Disabled option</FieldLabel>
        </Field>
      </FieldGroup>
    </section>
  )
}
