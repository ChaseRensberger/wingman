import { createFileRoute } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { HardDrives, CheckCircle, XCircle, CaretDown, CaretRight } from '@phosphor-icons/react'
import { apiGet } from '../lib/api'
import type { ProviderMeta, ModelDTO, ProvidersAuthResponse } from '../lib/types'
import { Heading } from '../components/primitives/heading'
import { Badge } from '../components/primitives/badge'
import { Button } from '../components/primitives/button'
import {
  Table,
  TableHead,
  TableHeader,
  TableBody,
  TableRow,
  TableCell,
} from '../components/primitives/table'
import { Text } from '../components/primitives/text'

export const Route = createFileRoute('/providers')({
  component: ProvidersPage,
})

function ProvidersPage() {
  const providersQuery = useQuery({
    queryKey: ['providers'],
    queryFn: () => apiGet<ProviderMeta[]>('/provider'),
  })

  const authQuery = useQuery({
    queryKey: ['providers-auth'],
    queryFn: () => apiGet<ProvidersAuthResponse>('/provider/auth'),
  })

  return (
    <div className="space-y-6">
      <Heading level={1}>Providers</Heading>
      <Text>Registered model providers and their available models.</Text>

      {providersQuery.isLoading || authQuery.isLoading ? (
        <Text>Loading providers...</Text>
      ) : providersQuery.isError ? (
        <Text className="text-destructive">Error: {providersQuery.error?.message}</Text>
      ) : (
        <div className="space-y-4">
          {providersQuery.data?.map((provider) => (
            <ProviderCard
              key={provider.id}
              provider={provider}
              auth={authQuery.data?.providers?.[provider.id]}
            />
          ))}
        </div>
      )}
    </div>
  )
}

function ProviderCard({
  provider,
  auth,
}: {
  provider: ProviderMeta
  auth?: { type: string; configured: boolean }
}) {
  const [expanded, setExpanded] = useState(false)

  return (
    <div className="rounded-xl border border-border bg-background p-4 dark:border-border dark:bg-card">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <HardDrives className="size-5 text-muted-foreground" />
          <div>
            <div className="flex items-center gap-2">
              <span className="font-semibold text-foreground dark:text-foreground">{provider.name}</span>
              <Badge color="zinc">{provider.id}</Badge>
            </div>
            <div className="mt-1 flex items-center gap-2 text-sm">
              {auth?.configured ? (
                <span className="flex items-center gap-1 text-green-600 dark:text-green-400">
                  <CheckCircle className="size-4" /> Configured
                </span>
              ) : (
                <span className="flex items-center gap-1 text-muted-foreground">
                  <XCircle className="size-4" /> Not configured
                </span>
              )}
              <span className="text-muted-foreground">·</span>
              <span className="text-muted-foreground">{auth?.type || 'api_key'}</span>
            </div>
          </div>
        </div>
        <Button plain onClick={() => setExpanded(!expanded)}>
          {expanded ? <CaretDown className="size-4" /> : <CaretRight className="size-4" />}
          <span className="ml-1">Models</span>
        </Button>
      </div>

      {expanded && <ProviderModels providerId={provider.id} />}
    </div>
  )
}

function ProviderModels({ providerId }: { providerId: string }) {
  const modelsQuery = useQuery({
    queryKey: ['provider-models', providerId],
    queryFn: () => apiGet<Record<string, ModelDTO>>(`/provider/${providerId}/models`),
  })

  if (modelsQuery.isLoading) return <Text className="mt-4">Loading models...</Text>
  if (modelsQuery.isError) return <Text className="mt-4 text-destructive">Failed to load models</Text>

  const models = Object.values(modelsQuery.data || {})
  if (models.length === 0) return <Text className="mt-4">No models available</Text>

  return (
    <div className="mt-4">
      <Table>
        <TableHead>
          <TableRow>
            <TableHeader>Model</TableHeader>
            <TableHeader>Context</TableHeader>
            <TableHeader>Max Output</TableHeader>
            <TableHeader>Capabilities</TableHeader>
            <TableHeader className="text-right">Input $/Mtok</TableHeader>
            <TableHeader className="text-right">Output $/Mtok</TableHeader>
          </TableRow>
        </TableHead>
        <TableBody>
          {models.map((m) => (
            <TableRow key={m.id}>
              <TableCell>
                <span className="font-medium text-foreground dark:text-foreground">{m.id}</span>
              </TableCell>
              <TableCell>{m.context_window?.toLocaleString() ?? '—'}</TableCell>
              <TableCell>{m.max_output?.toLocaleString() ?? '—'}</TableCell>
              <TableCell>
                <div className="flex flex-wrap gap-1">
                  {m.tools && <Badge color="blue">tools</Badge>}
                  {m.images && <Badge color="purple">images</Badge>}
                  {m.reasoning && <Badge color="amber">reasoning</Badge>}
                  {m.structured_output && <Badge color="green">structured</Badge>}
                </div>
              </TableCell>
              <TableCell className="text-right">
                {m.input_cost_per_mtok ? `$${m.input_cost_per_mtok.toFixed(2)}` : '—'}
              </TableCell>
              <TableCell className="text-right">
                {m.output_cost_per_mtok ? `$${m.output_cost_per_mtok.toFixed(2)}` : '—'}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
