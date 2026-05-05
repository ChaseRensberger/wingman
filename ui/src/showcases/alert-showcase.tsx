import { CheckCircle, WarningCircle, Info } from "@phosphor-icons/react"

import {
  Alert,
  AlertDescription,
  AlertTitle,
} from "@/components/core/alert"

export function AlertShowcase() {
  return (
    <section className="py-4 space-y-8">
      <h2 className="text-2xl font-semibold">Alert</h2>
      <div className="grid w-full max-w-xl items-start gap-4">
        <Alert>
          <CheckCircle />
          <AlertTitle>Success! Your changes have been saved</AlertTitle>
          <AlertDescription>
            This is an alert with icon, title and description.
          </AlertDescription>
        </Alert>
        <Alert>
          <Info />
          <AlertTitle>
            This Alert has a title and an icon. No description.
          </AlertTitle>
        </Alert>
        <Alert variant="destructive">
          <WarningCircle />
          <AlertTitle>Unable to process your payment.</AlertTitle>
          <AlertDescription>
            <p>Please verify your billing information and try again.</p>
          </AlertDescription>
        </Alert>
      </div>
    </section>
  )
}
