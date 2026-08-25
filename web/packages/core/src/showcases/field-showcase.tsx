import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
  FieldSet,
  FieldLegend,
} from "#components/core/field";
import { Input } from "#components/core/input";

export function FieldShowcase() {
  return (
    <section className="py-4 space-y-8">
      <h2 className="text-2xl font-semibold">Field</h2>
      <FieldSet className="max-w-lg">
        <FieldLegend>Profile</FieldLegend>
        <FieldDescription>This appears on invoices and emails.</FieldDescription>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="name">Full name</FieldLabel>
            <Input
              id="name"
              autoComplete="off"
              placeholder="Evil Rabbit"
            />
            <FieldDescription>This appears on invoices and emails.</FieldDescription>
          </Field>
          <Field>
            <FieldLabel htmlFor="username">Username</FieldLabel>
            <Input
              id="username"
              autoComplete="off"
              aria-invalid
            />
            <FieldError>Choose another username.</FieldError>
          </Field>
        </FieldGroup>
      </FieldSet>
    </section>
  );
}
