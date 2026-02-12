import { createContext, useContext, createSignal, type JSX } from "solid-js"
import type { Message, ToolCall, StreamEvent, DoneEvent } from "../types"
import { api } from "../api"

interface SessionState {
  messages: () => Message[]
  isStreaming: () => boolean
  status: () => string
  error: () => string | null
  agentID: () => string | null
  sessionID: () => string | null
  totalTokens: () => number
  totalSteps: () => number
  setAgentID: (id: string) => void
  setSessionID: (id: string) => void
  sendMessage: (text: string) => Promise<void>
  abort: () => void
}

const SessionContext = createContext<SessionState>()

export function SessionProvider(props: { children: JSX.Element }) {
  const [messages, setMessages] = createSignal<Message[]>([])
  const [isStreaming, setIsStreaming] = createSignal(false)
  const [status, setStatus] = createSignal("Ready")
  const [error, setError] = createSignal<string | null>(null)
  const [agentID, setAgentID] = createSignal<string | null>(null)
  const [sessionID, setSessionID] = createSignal<string | null>(null)
  const [totalTokens, setTotalTokens] = createSignal(0)
  const [totalSteps, setTotalSteps] = createSignal(0)
  let abortController: AbortController | null = null

  function abort() {
    if (abortController) {
      abortController.abort()
      abortController = null
    }
  }

  async function sendMessage(text: string) {
    const sid = sessionID()
    const aid = agentID()
    if (!sid || !aid || !text.trim() || isStreaming()) return

    const userMessage: Message = { role: "user", content: text }
    setMessages(prev => [...prev, userMessage])
    setIsStreaming(true)
    setStatus("Thinking...")
    setError(null)

    const assistantMessage: Message = {
      role: "assistant",
      content: "",
      toolCalls: [],
    }
    setMessages(prev => [...prev, assistantMessage])

    const toolInputs = new Map<number, string>()
    const toolIds = new Map<number, string>()
    const toolNames = new Map<number, string>()

    abortController = new AbortController()

    try {
      for await (const { event, data } of api.streamMessage(sid, aid, text, abortController.signal)) {
        if (event === "text_delta" || data.Type === "text_delta") {
          if (data.Text) {
            setMessages(prev => {
              const updated = [...prev]
              const last = updated[updated.length - 1]
              if (last) last.content += data.Text
              return updated
            })
          }
        } else if (event === "content_block_start" || data.Type === "content_block_start") {
          if (data.ContentBlock?.Type === "tool_use") {
            const toolId = data.ContentBlock.ID || `tool_${data.Index}`
            const toolName = data.ContentBlock.Name || "unknown"
            toolIds.set(data.Index, toolId)
            toolNames.set(data.Index, toolName)
            toolInputs.set(data.Index, "")

            setStatus(`Using ${toolName}...`)
            setMessages(prev => {
              const updated = [...prev]
              const last = updated[updated.length - 1]
              if (last) {
                last.toolCalls = [
                  ...(last.toolCalls || []),
                  { id: toolId, name: toolName, input: "" },
                ]
              }
              return updated
            })
          }
        } else if (event === "input_json_delta" || data.Type === "input_json_delta") {
          if (data.InputJSON) {
            const existing = toolInputs.get(data.Index) || ""
            toolInputs.set(data.Index, existing + data.InputJSON)

            const toolId = toolIds.get(data.Index)
            if (toolId) {
              setMessages(prev => {
                const updated = [...prev]
                const last = updated[updated.length - 1]
                if (last?.toolCalls) {
                  const tool = last.toolCalls.find(t => t.id === toolId)
                  if (tool) tool.input = toolInputs.get(data.Index) || ""
                }
                return updated
              })
            }
          }
        } else if (event === "tool_result" || data.Type === "tool_result") {
          const toolId = toolIds.get(data.Index)
          if (toolId && data.Text) {
            setMessages(prev => {
              const updated = [...prev]
              const last = updated[updated.length - 1]
              if (last?.toolCalls) {
                const tool = last.toolCalls.find(t => t.id === toolId)
                if (tool) tool.result = data.Text
              }
              return updated
            })
          }
        } else if (event === "content_block_stop" || data.Type === "content_block_stop") {
          const toolId = toolIds.get(data.Index)
          if (toolId) {
            const toolName = toolNames.get(data.Index) || ""
            setStatus(`Running ${toolName}...`)
          }
        } else if (event === "message_delta" || data.Type === "message_delta") {
          if (data.StopReason === "tool_use") {
            setStatus("Running tools...")
          }
        } else if (event === "message_stop" || data.Type === "message_stop") {
          // pass
        } else if (event === "done") {
          const done = data as DoneEvent
          setTotalTokens(prev => prev + (done.usage?.InputTokens || 0) + (done.usage?.OutputTokens || 0))
          setTotalSteps(prev => prev + (done.steps || 0))
        } else if (event === "error" || data.Error) {
          setError(data.Error || data.Text || String(data))
        }
      }
    } catch (e) {
      if ((e as Error).name !== "AbortError") {
        setError(e instanceof Error ? e.message : "Request failed")
      }
    } finally {
      setIsStreaming(false)
      setStatus("Ready")
      abortController = null
    }
  }

  const state: SessionState = {
    messages,
    isStreaming,
    status,
    error,
    agentID,
    sessionID,
    totalTokens,
    totalSteps,
    setAgentID,
    setSessionID,
    sendMessage,
    abort,
  }

  return (
    <SessionContext.Provider value={state}>
      {props.children}
    </SessionContext.Provider>
  )
}

export function useSession() {
  const ctx = useContext(SessionContext)
  if (!ctx) throw new Error("useSession must be used within SessionProvider")
  return ctx
}
