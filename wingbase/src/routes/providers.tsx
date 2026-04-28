import { createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/providers')({
  component: ProvidersPage,
})

function ProvidersPage() {
  return null
}
