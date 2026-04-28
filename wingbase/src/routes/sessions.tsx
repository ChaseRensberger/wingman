import { createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/sessions')({
  component: SessionsPage,
})

function SessionsPage() {
  return null
}
