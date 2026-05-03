import { Switch } from "@/components/core/switch"
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from "@/components/core/field"

export function SwitchShowcase() {
  return (
    <section className="py-4 space-y-8">
      <h2 className="text-2xl font-semibold">Switch</h2>
      <FieldGroup>
        <Field orientation="horizontal">
          <Switch id="airplane-mode" />
          <FieldLabel htmlFor="airplane-mode">Airplane Mode</FieldLabel>
        </Field>
        <Field orientation="horizontal">
          <Switch id="notifications" defaultChecked />
          <FieldContent>
            <FieldLabel htmlFor="notifications">Enable notifications</FieldLabel>
            <FieldDescription>
              Receive alerts when focus mode is enabled or disabled.
            </FieldDescription>
          </FieldContent>
        </Field>
        <Field orientation="horizontal">
          <Switch id="disabled-switch" disabled />
          <FieldLabel htmlFor="disabled-switch">Disabled</FieldLabel>
        </Field>
      </FieldGroup>
    </section>
  )
}
