import { createFileRoute } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState, useEffect, useRef } from 'react'
import { Plus, Trash, PaperPlaneRight, ChatCircle } from '@phosphor-icons/react'
import { apiGet, apiPost, apiDelete } from '../lib/api'
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
    mutationFn: (workDir?: string) => apiPost<Session>('/sessions', { work_dir: workDir }),
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
      <aside className="flex w-72 shrink-0 flex-col rounded-xl border border-zinc-200 bg-white dark:border-white/10 dark:bg-zinc-900">
        <div className="flex items-center justify-between border-b border-zinc-200 p-3 dark:border-white/10">
          <Heading level={2} className="!text-base">Sessions</Heading>
          <Button onClick={() => createMutation.mutate(undefined)} disabled={createMutation.isPending}>
            <Plus className="size-4" /> New
          </Button>
        </div>
        <div className="flex-1 overflow-y-auto">
          {sessionsQuery.isLoading ? (
            <div className="p-3"><Text>Loading...</Text></div>
          ) : sessionsQuery.data && sessionsQuery.data.length > 0 ? (
            <ul className="divide-y divide-zinc-100 dark:divide-white/5">
              {sessionsQuery.data.map((sess) => (
                <li key={sess.id}>
                  <button
                    type="button"
                    onClick={() => setSelectedId(sess.id)}
                    className={clsx(
                      'group flex w-full cursor-pointer items-start justify-between gap-2 px-3 py-2.5 text-left hover:bg-zinc-50 dark:hover:bg-white/5',
                      selectedId === sess.id && 'bg-zinc-100 dark:bg-white/10'
                    )}
                  >
                    <div className="min-w-0 flex-1">
                      <div className="truncate font-mono text-xs text-zinc-950 dark:text-white">
                        {sess.id.slice(0, 12)}
                      </div>
                      <div className="mt-0.5 truncate text-xs text-zinc-500 dark:text-zinc-400">
                        {sess.history?.length ?? 0} messages
                      </div>
                    </div>
                    <button
                      type="button"
                      onClick={(e) => {
                        e.stopPropagation()
                        deleteMutation.mutate(sess.id)
                      }}
                      className="invisible cursor-pointer rounded p-1 text-zinc-400 hover:bg-red-100 hover:text-red-600 group-hover:visible dark:hover:bg-red-500/20"
                    >
                      <Trash className="size-3.5" />
                    </button>
                  </button>
                </li>
              ))}
            </ul>
          ) : (
            <div className="p-6 text-center">
              <ChatCircle className="mx-auto size-8 text-zinc-300 dark:text-white/20" />
              <Text className="mt-2">No sessions yet</Text>
            </div>
          )}
        </div>
      </aside>

      <main className="flex flex-1 flex-col rounded-xl border border-zinc-200 bg-white dark:border-white/10 dark:bg-zinc-900">
        {selectedId ? (
          <SessionDetail sessionId={selectedId} />
        ) : (
          <div className="flex flex-1 items-center justify-center">
            <div className="text-center">
              <ChatCircle className="mx-auto size-12 text-zinc-300 dark:text-white/20" />
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

  const sessionQuery = useQuery({
    queryKey: ['session', sessionId],
    queryFn: () => apiGet<Session>(`/sessions/${sessionId}`),
  })

  const agentsQuery = useQuery({
    queryKey: ['agents'],
    queryFn: () => apiGet<Agent[]>('/agents'),
  })

  useEffect(() => {
    if (!agentId && agentsQuery.data && agentsQuery.data.length > 0) {
      setAgentId(agentsQuery.data[0].id)
    }
  }, [agentId, agentsQuery.data])

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
  if (sessionQuery.isError) return <div className="p-4"><Text className="text-red-600">Failed to load session</Text></div>

  const session = sessionQuery.data!
  const messages = session.history ?? []

  return (
    <>
      <div className="flex items-center justify-between border-b border-zinc-200 p-3 dark:border-white/10">
        <div>
          <div className="font-mono text-xs text-zinc-500 dark:text-zinc-400">{session.id}</div>
          {session.work_dir && (
            <div className="mt-0.5 font-mono text-xs text-zinc-400">{session.work_dir}</div>
          )}
        </div>
        <div className="w-56">
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
            <div className="rounded-lg bg-zinc-100 px-3 py-2 text-sm text-zinc-500 dark:bg-white/5 dark:text-zinc-400">
              Thinking...
            </div>
          </div>
        )}
        {sendMutation.isError && (
          <div className="rounded-lg bg-red-50 p-2 text-sm text-red-700 dark:bg-red-500/10 dark:text-red-400">
            {sendMutation.error?.message}
          </div>
        )}
      </div>

      <form onSubmit={handleSend} className="border-t border-zinc-200 p-3 dark:border-white/10">
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
            ? 'bg-blue-600 text-white'
            : isAssistant
              ? 'bg-zinc-100 text-zinc-950 dark:bg-white/5 dark:text-white'
              : 'bg-amber-50 text-amber-900 dark:bg-amber-500/10 dark:text-amber-300'
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
        <div className="mt-1 rounded border border-current/20 bg-black/5 p-2 font-mono text-xs dark:bg-white/5">
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
        <div className="mt-1 rounded border border-current/20 bg-black/5 p-2 font-mono text-xs dark:bg-white/5">
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
