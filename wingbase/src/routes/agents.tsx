import { createFileRoute } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { Plus, Trash, PencilSimple, Robot } from '@phosphor-icons/react'
import { apiGet, apiPost, apiPut, apiDelete } from '../lib/api'
import type { Agent, ProviderMeta, ModelDTO } from '../lib/types'
import { Heading } from '../components/primitives/heading'
import { Text } from '../components/primitives/text'
import { Button } from '../components/primitives/button'
import { Input } from '../components/primitives/input'
import { Textarea } from '../components/primitives/textarea'
import { Select } from '../components/primitives/select'
import { Badge } from '../components/primitives/badge'
import {
  Dialog,
  DialogTitle,
  DialogDescription,
  DialogBody,
  DialogActions,
} from '../components/primitives/dialog'
import { Field, Label, Description } from '../components/primitives/fieldset'

export const Route = createFileRoute('/agents')({
  component: AgentsPage,
})

interface AgentFormState {
  name: string
  instructions: string
  provider: string
  model: string
  tools: string
}

const EMPTY_FORM: AgentFormState = {
  name: '',
  instructions: '',
  provider: '',
  model: '',
  tools: '',
}

function AgentsPage() {
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState<Agent | null>(null)
  const [creating, setCreating] = useState(false)

  const agentsQuery = useQuery({
    queryKey: ['agents'],
    queryFn: () => apiGet<Agent[]>('/agents'),
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => apiDelete(`/agents/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['agents'] }),
  })

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <Heading level={1}>Agents</Heading>
        <Button onClick={() => setCreating(true)}>
          <Plus className="size-4" /> New agent
        </Button>
      </div>
      <Text>Configure agents that can be used in chat sessions.</Text>

      {agentsQuery.isLoading ? (
        <Text>Loading agents...</Text>
      ) : agentsQuery.isError ? (
        <Text className="text-destructive">Error: {agentsQuery.error?.message}</Text>
      ) : agentsQuery.data && agentsQuery.data.length > 0 ? (
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
          {agentsQuery.data.map((agent) => (
            <div
              key={agent.id}
              className="rounded-xl border border-border bg-card p-4"
            >
              <div className="flex items-start justify-between gap-2">
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <Robot className="size-4 text-muted-foreground" />
                    <span className="truncate font-semibold text-foreground">
                      {agent.name}
                    </span>
                  </div>
                  <div className="mt-1 font-mono text-xs text-muted-foreground">{agent.id}</div>
                </div>
                <div className="flex shrink-0 gap-1">
                  <button
                    type="button"
                    onClick={() => setEditing(agent)}
                    className="cursor-pointer rounded p-1.5 text-muted-foreground hover:bg-muted hover:text-foreground"
                  >
                    <PencilSimple className="size-3.5" />
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      if (confirm(`Delete agent "${agent.name}"?`)) {
                        deleteMutation.mutate(agent.id)
                      }
                    }}
                    className="cursor-pointer rounded p-1.5 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
                  >
                    <Trash className="size-3.5" />
                  </button>
                </div>
              </div>

              {agent.instructions && (
                <p className="mt-3 line-clamp-3 text-sm text-muted-foreground">
                  {agent.instructions}
                </p>
              )}

              <div className="mt-3 flex flex-wrap gap-1">
                {agent.provider && <Badge color="blue">{agent.provider}</Badge>}
                {agent.model && <Badge color="zinc">{agent.model}</Badge>}
                {agent.tools?.map((t) => (
                  <Badge key={t} color="purple">{t}</Badge>
                ))}
              </div>
            </div>
          ))}
        </div>
      ) : (
          <div className="rounded-xl border border-dashed border-border p-12 text-center">
          <Robot className="mx-auto size-10 text-subtle-foreground" />
          <Text className="mt-2">No agents yet. Create one to start chatting.</Text>
        </div>
      )}

      {(creating || editing) && (
        <AgentDialog
          agent={editing}
          onClose={() => {
            setCreating(false)
            setEditing(null)
          }}
        />
      )}
    </div>
  )
}

function AgentDialog({ agent, onClose }: { agent: Agent | null; onClose: () => void }) {
  const queryClient = useQueryClient()
  const [form, setForm] = useState<AgentFormState>(() =>
    agent
      ? {
          name: agent.name,
          instructions: agent.instructions ?? '',
          provider: agent.provider ?? '',
          model: agent.model ?? '',
          tools: agent.tools?.join(', ') ?? '',
        }
      : EMPTY_FORM
  )

  const providersQuery = useQuery({
    queryKey: ['providers'],
    queryFn: () => apiGet<ProviderMeta[]>('/provider'),
  })

  const modelsQuery = useQuery({
    queryKey: ['provider-models', form.provider],
    queryFn: () => apiGet<Record<string, ModelDTO>>(`/provider/${form.provider}/models`),
    enabled: !!form.provider,
  })

  // Reset model if it doesn't exist in the new provider's model list
  useEffect(() => {
    if (modelsQuery.data && form.model && !modelsQuery.data[form.model]) {
      setForm((f) => ({ ...f, model: '' }))
    }
  }, [modelsQuery.data, form.model])

  const saveMutation = useMutation({
    mutationFn: async () => {
      const payload = {
        name: form.name,
        instructions: form.instructions || undefined,
        provider: form.provider || undefined,
        model: form.model || undefined,
        tools: form.tools
          ? form.tools.split(',').map((t) => t.trim()).filter(Boolean)
          : undefined,
      }
      if (agent) {
        return apiPut<Agent>(`/agents/${agent.id}`, payload)
      }
      return apiPost<Agent>('/agents', payload)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['agents'] })
      onClose()
    },
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!form.name.trim()) return
    saveMutation.mutate()
  }

  return (
    <Dialog open onClose={onClose} size="lg">
      <DialogTitle>{agent ? 'Edit agent' : 'New agent'}</DialogTitle>
      <DialogDescription>
        Define an agent's identity, model, and tool access.
      </DialogDescription>

      <form onSubmit={handleSubmit}>
        <DialogBody className="space-y-5">
          <Field>
            <Label>Name</Label>
            <Input
              type="text"
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              placeholder="my-agent"
              required
            />
          </Field>

          <Field>
            <Label>Instructions</Label>
            <Description>System prompt that shapes the agent's behavior.</Description>
            <Textarea
              rows={5}
              value={form.instructions}
              onChange={(e) => setForm({ ...form, instructions: e.target.value })}
              placeholder="You are a helpful assistant..."
            />
          </Field>

          <div className="grid grid-cols-2 gap-4">
            <Field>
              <Label>Provider</Label>
              <Select
                value={form.provider}
                onChange={(e) => setForm({ ...form, provider: e.target.value, model: '' })}
              >
                <option value="">Select...</option>
                {providersQuery.data?.map((p) => (
                  <option key={p.id} value={p.id}>{p.name}</option>
                ))}
              </Select>
            </Field>

            <Field>
              <Label>Model</Label>
              <Select
                value={form.model}
                onChange={(e) => setForm({ ...form, model: e.target.value })}
                disabled={!form.provider || modelsQuery.isLoading}
              >
                <option value="">Select...</option>
                {modelsQuery.data &&
                  Object.values(modelsQuery.data).map((m) => (
                    <option key={m.id} value={m.id}>{m.id}</option>
                  ))}
              </Select>
            </Field>
          </div>

          <Field>
            <Label>Tools</Label>
            <Description>Comma-separated tool names (e.g. <code>read, write, bash</code>).</Description>
            <Input
              type="text"
              value={form.tools}
              onChange={(e) => setForm({ ...form, tools: e.target.value })}
              placeholder="read, write, bash"
            />
          </Field>

          {saveMutation.isError && (
            <div className="rounded-lg bg-destructive/10 p-2 text-sm text-destructive">
              {saveMutation.error?.message}
            </div>
          )}
        </DialogBody>

        <DialogActions>
          <Button plain type="button" onClick={onClose}>Cancel</Button>
          <Button type="submit" disabled={!form.name.trim() || saveMutation.isPending}>
            {saveMutation.isPending ? 'Saving...' : 'Save'}
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  )
}
