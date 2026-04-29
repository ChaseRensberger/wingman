import { createFileRoute } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState, useEffect, useRef } from 'react'
import { Plus, Trash, PaperPlaneRight, ChatCircle, PencilSimple, Check, X } from '@phosphor-icons/react'
import { apiGet, apiPost, apiPut, apiDelete } from '../lib/api'

// The backend stores an empty string when no title was supplied; we
// also default-fill with this constant on create. Centralized so the
// list/detail views can render a consistent placeholder for legacy
// rows that predate the title column (default '' from the migration).
const UNTITLED_SESSION = 'Untitled session'

function displayTitle(sess: Pick<Session, 'title' | 'id'>): string {
  return sess.title?.trim() ? sess.title : UNTITLED_SESSION
}
import type {
  Session,
  Agent,
  MessageSessionResponse,
  Part,
  Message,
} from '../lib/types'
import { Heading } from '../components/primitives/heading'
import { Text } from '../components/primitives/text'
import { Button } from '../components/primitives/button'
import { Input } from '../components/primitives/input'
import { Badge } from '../components/primitives/badge'
import { Select } from '../components/primitives/select'
import clsx from 'clsx'

export const Route = createFileRoute('/sessions')({
  component: SessionsPage,
})

function SessionsPage() {
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const queryClient = useQueryClient()

  const sessionsQuery = useQuery({
    queryKey: ['sessions'],
    queryFn: () => apiGet<Session[]>('/sessions'),
  })

  const createMutation = useMutation({
    // Title is optional in the API; the server fills "New session" when
    // omitted. We send `undefined` here and let users rename via the
    // inline editor in the detail header — that keeps the "+ New"
    // affordance one-click while still surfacing titles everywhere.
    mutationFn: () => apiPost<Session>('/sessions', {}),
    onSuccess: (sess) => {
      queryClient.invalidateQueries({ queryKey: ['sessions'] })
      setSelectedId(sess.id)
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => apiDelete(`/sessions/${id}`),
    onSuccess: (_data, id) => {
      queryClient.invalidateQueries({ queryKey: ['sessions'] })
      if (selectedId === id) setSelectedId(null)
    },
  })

  return (
    <div className="flex h-[calc(100vh-8rem)] gap-4">
      <aside className="flex w-72 shrink-0 flex-col rounded-xl border border-border bg-card">
        <div className="flex items-center justify-between border-b border-border p-3">
          <Heading level={2} className="!text-base">Sessions</Heading>
          <Button onClick={() => createMutation.mutate()} disabled={createMutation.isPending}>
            <Plus className="size-4" /> New
          </Button>
        </div>
        <div className="flex-1 overflow-y-auto">
          {sessionsQuery.isLoading ? (
            <div className="p-3"><Text>Loading...</Text></div>
          ) : sessionsQuery.data && sessionsQuery.data.length > 0 ? (
            <ul className="divide-y divide-border">
              {sessionsQuery.data.map((sess) => (
                <li key={sess.id}>
                  <button
                    type="button"
                    onClick={() => setSelectedId(sess.id)}
                    className={clsx(
                      'group flex w-full cursor-pointer items-start justify-between gap-2 px-3 py-2.5 text-left hover:bg-accent',
                      selectedId === sess.id && 'bg-muted'
                    )}
                  >
                    <div className="min-w-0 flex-1">
                      <div className="truncate text-sm font-medium text-foreground">
                        {displayTitle(sess)}
                      </div>
                      <div className="mt-0.5 truncate font-mono text-xs text-muted-foreground">
                        {sess.id.slice(0, 12)} · {sess.history?.length ?? 0} msg
                      </div>
                    </div>
                    <button
                      type="button"
                      onClick={(e) => {
                        e.stopPropagation()
                        deleteMutation.mutate(sess.id)
                      }}
                      className="invisible cursor-pointer rounded p-1 text-muted-foreground hover:bg-destructive/10 hover:text-destructive group-hover:visible"
                    >
                      <Trash className="size-3.5" />
                    </button>
                  </button>
                </li>
              ))}
            </ul>
          ) : (
            <div className="p-6 text-center">
              <ChatCircle className="mx-auto size-8 text-subtle-foreground" />
              <Text className="mt-2">No sessions yet</Text>
            </div>
          )}
        </div>
      </aside>

      <main className="flex flex-1 flex-col rounded-xl border border-border bg-card">
        {selectedId ? (
          <SessionDetail sessionId={selectedId} />
        ) : (
          <div className="flex flex-1 items-center justify-center">
            <div className="text-center">
              <ChatCircle className="mx-auto size-12 text-subtle-foreground" />
              <Text className="mt-2">Select a session or create a new one</Text>
            </div>
          </div>
        )}
      </main>
    </div>
  )
}

function SessionDetail({ sessionId }: { sessionId: string }) {
  const queryClient = useQueryClient()
  const [input, setInput] = useState('')
  const [agentId, setAgentId] = useState<string>('')
  const scrollRef = useRef<HTMLDivElement>(null)

  // Inline-rename state lives here rather than in the header
  // component so React Query refetches don't reset the editor mid-typing.
  const [editingTitle, setEditingTitle] = useState(false)
  const [titleDraft, setTitleDraft] = useState('')

  const sessionQuery = useQuery({
    queryKey: ['session', sessionId],
    queryFn: () => apiGet<Session>(`/sessions/${sessionId}`),
  })

  const agentsQuery = useQuery({
    queryKey: ['agents'],
    queryFn: () => apiGet<Agent[]>('/agents'),
  })

  const renameMutation = useMutation({
    mutationFn: (title: string) =>
      apiPut<Session>(`/sessions/${sessionId}`, { title }),
    onSuccess: () => {
      // Invalidate both the detail and the list so the sidebar label
      // reflects the new title without a manual refresh.
      queryClient.invalidateQueries({ queryKey: ['session', sessionId] })
      queryClient.invalidateQueries({ queryKey: ['sessions'] })
      setEditingTitle(false)
    },
  })

  useEffect(() => {
    if (!agentId && agentsQuery.data && agentsQuery.data.length > 0) {
      setAgentId(agentsQuery.data[0].id)
    }
  }, [agentId, agentsQuery.data])

  // Reset rename state whenever we switch sessions; otherwise the
  // editor would stay open with stale text from the previous session.
  useEffect(() => {
    setEditingTitle(false)
    setTitleDraft('')
  }, [sessionId])

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight })
  }, [sessionQuery.data?.history?.length])

  const sendMutation = useMutation({
    mutationFn: (message: string) =>
      apiPost<MessageSessionResponse>(`/sessions/${sessionId}/message`, {
        agent_id: agentId,
        message,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['session', sessionId] })
      queryClient.invalidateQueries({ queryKey: ['sessions'] })
      setInput('')
    },
  })

  const handleSend = (e: React.FormEvent) => {
    e.preventDefault()
    const msg = input.trim()
    if (!msg || !agentId || sendMutation.isPending) return
    sendMutation.mutate(msg)
  }

  if (sessionQuery.isLoading) return <div className="p-4"><Text>Loading session...</Text></div>
  if (sessionQuery.isError) return <div className="p-4"><Text className="text-destructive">Failed to load session</Text></div>

  const session = sessionQuery.data!
  const messages = session.history ?? []

  const startRename = () => {
    setTitleDraft(session.title ?? '')
    setEditingTitle(true)
  }

  const commitRename = () => {
    const next = titleDraft.trim()
    // No-op if unchanged; avoids a wasted PUT and the resulting
    // updated_at bump that would re-sort the sidebar for nothing.
    if (next === (session.title ?? '')) {
      setEditingTitle(false)
      return
    }
    renameMutation.mutate(next)
  }

  return (
    <>
      <div className="flex items-center justify-between gap-3 border-b border-border p-3">
        <div className="min-w-0 flex-1">
          {editingTitle ? (
            <form
              onSubmit={(e) => {
                e.preventDefault()
                commitRename()
              }}
              className="flex items-center gap-1"
            >
              <Input
                autoFocus
                type="text"
                value={titleDraft}
                onChange={(e) => setTitleDraft(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Escape') setEditingTitle(false)
                }}
                disabled={renameMutation.isPending}
                placeholder={UNTITLED_SESSION}
              />
              <Button type="submit" disabled={renameMutation.isPending}>
                <Check className="size-4" />
              </Button>
              <Button
                type="button"
                onClick={() => setEditingTitle(false)}
                disabled={renameMutation.isPending}
              >
                <X className="size-4" />
              </Button>
            </form>
          ) : (
            <div className="flex items-center gap-2">
              <Heading level={2} className="!text-base !truncate">
                {displayTitle(session)}
              </Heading>
              <button
                type="button"
                onClick={startRename}
                className="cursor-pointer rounded p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
                aria-label="Rename session"
              >
                <PencilSimple className="size-3.5" />
              </button>
            </div>
          )}
          <div className="mt-0.5 truncate font-mono text-xs text-muted-foreground">
            {session.id}
            {session.work_dir ? ` · ${session.work_dir}` : ''}
          </div>
        </div>
        <div className="w-56 shrink-0">
          <Select value={agentId} onChange={(e) => setAgentId(e.target.value)}>
            <option value="">Select agent...</option>
            {agentsQuery.data?.map((a) => (
              <option key={a.id} value={a.id}>{a.name}</option>
            ))}
          </Select>
        </div>
      </div>

      <div ref={scrollRef} className="flex-1 space-y-4 overflow-y-auto p-4">
        {messages.length === 0 ? (
          <div className="flex h-full items-center justify-center">
            <Text>No messages yet. Start the conversation below.</Text>
          </div>
        ) : (
          messages.map((msg, i) => <MessageBubble key={i} message={msg} />)
        )}
        {sendMutation.isPending && (
          <div className="flex justify-start">
            <div className="rounded-lg bg-muted px-3 py-2 text-sm text-muted-foreground">
              Thinking...
            </div>
          </div>
        )}
        {sendMutation.isError && (
          <div className="rounded-lg bg-destructive/10 p-2 text-sm text-destructive">
            {sendMutation.error?.message}
          </div>
        )}
      </div>

      <form onSubmit={handleSend} className="border-t border-border p-3">
        <div className="flex gap-2">
          <Input
            type="text"
            placeholder={agentId ? 'Type a message...' : 'Select an agent first'}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            disabled={!agentId || sendMutation.isPending}
          />
          <Button type="submit" disabled={!input.trim() || !agentId || sendMutation.isPending}>
            <PaperPlaneRight className="size-4" />
          </Button>
        </div>
      </form>
    </>
  )
}

function MessageBubble({ message }: { message: Message }) {
  const isUser = message.role === 'user'
  const isAssistant = message.role === 'assistant'

  return (
    <div className={clsx('flex', isUser ? 'justify-end' : 'justify-start')}>
      <div
        className={clsx(
          'max-w-[80%] rounded-lg px-3 py-2 text-sm',
          isUser
            ? 'bg-primary text-primary-foreground'
            : isAssistant
              ? 'bg-muted text-foreground'
              : 'bg-amber-500/10 text-amber-600'
        )}
      >
        <div className="mb-1 flex items-center gap-1.5">
          <Badge color={isUser ? 'blue' : isAssistant ? 'zinc' : 'amber'}>
            {message.role}
          </Badge>
          {message.origin && (
            <span className="text-xs opacity-70">
              {message.origin.provider}/{message.origin.model_id}
            </span>
          )}
        </div>
        {message.content?.map((part, i) => <PartView key={i} part={part} />)}
      </div>
    </div>
  )
}

function PartView({ part }: { part: Part }) {
  switch (part.type) {
    case 'text':
      return <div className="whitespace-pre-wrap break-words">{String(part.text ?? '')}</div>
    case 'tool_call':
      return (
        <div className="mt-1 rounded border border-current/20 bg-overlay p-2 font-mono text-xs">
          <div className="font-semibold">→ {String(part.name ?? 'tool')}</div>
          {part.input != null && (
            <pre className="mt-1 overflow-x-auto whitespace-pre-wrap">
              {JSON.stringify(part.input, null, 2)}
            </pre>
          )}
        </div>
      )
    case 'tool_result':
      return (
        <div className="mt-1 rounded border border-current/20 bg-overlay p-2 font-mono text-xs">
          <div className="font-semibold">← result</div>
          <pre className="mt-1 overflow-x-auto whitespace-pre-wrap">
            {typeof part.output === 'string' ? part.output : JSON.stringify(part.output, null, 2)}
          </pre>
        </div>
      )
    case 'reasoning':
      return (
        <div className="mt-1 italic opacity-70">
          {String(part.text ?? '')}
        </div>
      )
    default:
      return (
        <div className="mt-1 font-mono text-xs opacity-60">
          [{part.type}]
        </div>
      )
  }
}
