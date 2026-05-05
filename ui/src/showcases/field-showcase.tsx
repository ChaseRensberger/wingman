import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
  FieldSet,
  FieldLegend,
} from "@/components/core/field"

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
            <input
              id="name"
              autoComplete="off"
              placeholder="Evil Rabbit"
              className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors file:border-0 file:bg-transparent file:text-sm file:font-medium placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
            />
            <FieldDescription>This appears on invoices and emails.</FieldDescription>
          </Field>
          <Field>
            <FieldLabel htmlFor="username">Username</FieldLabel>
            <input
              id="username"
              autoComplete="off"
              aria-invalid
              className="flex h-9 w-full rounded-md border border-destructive bg-transparent px-3 py-1 text-sm shadow-sm transition-colors file:border-0 file:bg-transparent file:text-sm file:font-medium placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-destructive disabled:cursor-not-allowed disabled:opacity-50"
            />
            <FieldError>Choose another username.</FieldError>
          </Field>
        </FieldGroup>
      </FieldSet>
    </section>
  )
}
