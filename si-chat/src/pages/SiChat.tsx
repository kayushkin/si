import React, { useEffect, useRef, useState, useCallback } from 'react'
import styles from './SiChat.module.css'

interface ToolEvent {
  tool: string
  input?: string
  output?: string
  error?: boolean
}

interface MessageMeta {
  inputTokens: number
  outputTokens: number
  cacheReadTokens: number
  cacheCreationTokens: number
  toolCalls: number
  cost: number
  durationMs: number
  model: string
  turn: number
  tools?: ToolEvent[]
}

interface Message {
  id: string
  role: 'user' | 'assistant' | 'system' | 'error'
  content: string
  timestamp: string
  agent?: string
  orchestrator?: string
  targetAgent?: string // the agent this message is for/from (not the human author)
  meta?: MessageMeta
  thinking?: string // accumulated thinking/reasoning text
  error?: {
    error: string
    code?: string
  }
}

interface AgentInfo {
  id: string
  name: string
  orchestrator: string
  description: string
  emoji: string
  enabled: boolean
}

interface SessionInfo {
  key: string
  agent: string
  status: 'idle' | 'running' | 'completed' | 'error'
  spawn_depth: number
  parent_key?: string
  children?: string[]
  created_at: string
  last_active: string
  messages: number
  active_request?: {
    id: number
    input_text: string
    status: string
    started_at: string
  }
}

interface GatewayEvent {
  kind: 'spawn_started' | 'spawn_completed' | 'session_active' | 'session_idle'
  session_key: string
  agent: string
  parent_key?: string
  task?: string
  status?: string
  summary?: string
  tokens?: { input: number; output: number; cost: number }
  duration_ms?: number
  error?: string
  timestamp: string
}

interface SpawnCard {
  sessionKey: string
  agent: string
  parentKey?: string
  task?: string
  status: 'running' | 'completed' | 'error'
  summary?: string
  tokens?: { input: number; output: number; cost: number }
  durationMs?: number
  error?: string
  timestamp: string
}

// Fallback agents shown when registry is unreachable.
const FALLBACK_AGENTS: AgentInfo[] = [
  { id: 'claxon', name: 'claxon', orchestrator: 'inber', description: 'Main orchestrator', emoji: '🦀', enabled: true },
]

const ORCH_ICON: Record<string, string> = {
  'inber': '🌿',
  'openclaw': '🦀',
  'claude-code': '💻',
  'codex': '🤖',
}

// Map celtic agent names to their project/domain for clarity
const AGENT_PROJECT: Record<string, string> = {
  'main': 'claxon',
  'brigid': 'kayushkin.com',
  'fionn': 'inber',
  'oisin': 'si / bus',
  'goibniu': 'forge',
  'manannan': 'downloadstack',
  'ogma': 'logstack',
  'scathach': 'claxon-android',
  'bran': 'orchestrator (shelved)',
}

const AGENT_EMOJI: Record<string, string> = {
  'main': '🦀',
  'claxon': '🦀',
  'brigid': '🔥',
  'fionn': '🦌',
  'oisin': '📨',
  'goibniu': '🔨',
  'manannan': '🌊',
  'ogma': '📜',
  'scathach': '⚔️',
  'bran': '🚢',
  'keyboard': '⌨️',
}

// Default/main agent per orchestrator
const MAIN_AGENTS: Record<string, string> = {
  'openclaw': 'main',
  'inber': 'claxon',
}

// Intermediate tool/thinking event from logstack — collected separately and attached to parent messages
interface LogToolEvent {
  streamId: string
  stream: 'tool_call' | 'tool_result' | 'thinking'
  tool?: ToolEvent
  thinkingText?: string
}

// Parse logstack entry → Message (or tool event for later attachment)
function logEntryToMessage(entry: any): { message?: Message; toolEvent?: LogToolEvent } {
  const content = typeof entry.content === 'object' ? entry.content : null
  if (!content?.text && !content?.meta?.tools) return {}

  const stream = content.stream || ''

  // Skip deltas — they're intermediate streaming chunks
  if (stream === 'delta') return {}

  // Collect tool_call / tool_result events for attachment to parent messages
  if ((stream === 'tool_call' || stream === 'tool_result') && content.stream_id) {
    const toolData = content.meta?.tools?.[0]
    if (toolData) {
      return {
        toolEvent: {
          streamId: content.stream_id,
          stream: stream as 'tool_call' | 'tool_result',
          tool: stream === 'tool_call'
            ? { tool: toolData.tool, input: toolData.input }
            : { tool: toolData.tool, output: toolData.output, error: toolData.error },
        }
      }
    }
    return {}
  }

  // Collect thinking events
  if (stream === 'thinking' && content.stream_id) {
    return {
      toolEvent: {
        streamId: content.stream_id,
        stream: 'thinking',
        thinkingText: content.text || '',
      }
    }
  }

  const isInbound = entry.type === 'inbound'
  const targetAgent = isInbound
    ? (content.agent || entry.agent)
    : (content.author || entry.agent)

  const text = content.text || ''

  // Filter out messages that are just raw status text like "API CALL" with no real content
  const trimmed = text.trim().toUpperCase()
  if (!isInbound && (trimmed === 'API CALL' || trimmed === 'API_CALL' || trimmed === 'TOOL CALL' || trimmed === 'TOOL_CALL')) {
    return {}
  }

  // Skip assistant messages with empty content and no tools/thinking
  const meta = parseMeta(content.meta || entry.metadata)
  if (!isInbound && !text.trim() && !meta?.tools?.length && !content.thinking) {
    return {}
  }

  return {
    message: {
      id: content.stream_id || entry.id || crypto.randomUUID(),
      role: isInbound ? 'user' : 'assistant',
      content: text,
      timestamp: entry.timestamp,
      agent: content.author || entry.agent,
      orchestrator: content.orchestrator || '',
      targetAgent,
      thinking: content.thinking || undefined,
      meta,
    }
  }
}

// Parse meta from Si backend MessageMeta format
function parseMeta(raw: any): MessageMeta | undefined {
  if (!raw) return undefined
  const meta: MessageMeta = {
    inputTokens: raw.input_tokens || 0,
    outputTokens: raw.output_tokens || 0,
    cacheReadTokens: raw.cache_read_tokens || 0,
    cacheCreationTokens: raw.cache_creation_tokens || 0,
    toolCalls: raw.tool_calls || 0,
    cost: raw.cost || 0,
    durationMs: raw.duration_ms || 0,
    model: raw.model || '',
    turn: raw.turn || 0,
    tools: raw.tools || undefined,
  }
  if (meta.inputTokens || meta.outputTokens || meta.toolCalls || meta.cost || meta.durationMs || meta.model || meta.turn || meta.tools?.length) {
    return meta
  }
  return undefined
}

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`
  const mins = Math.floor(ms / 60000)
  const secs = ((ms % 60000) / 1000).toFixed(0)
  return `${mins}m ${secs}s`
}

function formatCost(cost: number): string {
  if (cost < 0.001) return `$${cost.toFixed(6)}`
  if (cost < 0.01) return `$${cost.toFixed(4)}`
  return `$${cost.toFixed(3)}`
}

function formatTokens(n: number): string {
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`
  return `${n}`
}

function formatElapsedTime(ms: number): string {
  if (ms < 1000) return `${Math.floor(ms / 100) / 10}s`
  const seconds = Math.floor(ms / 1000)
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  const remainingSeconds = seconds % 60
  return `${minutes}m ${remainingSeconds}s`
}

// Utility functions kept for future use
// function tokensPerSecond(meta: MessageMeta) { ... }
// function cacheHitRate(meta: MessageMeta) { ... }
// function shortModel(model: string) { ... }

// Composite key for agent+orchestrator selection.
function agentKey(name: string, orchestrator: string) { return `${name}:${orchestrator}` }

export default function SiChat() {
  const [selected, setSelected] = useState(agentKey('claxon', 'inber'))
  const [agents, setAgents] = useState<AgentInfo[]>(FALLBACK_AGENTS)
  const [allMessages, setAllMessages] = useState<Message[]>([])
  const [connected, setConnected] = useState(false)
  const [input, setInput] = useState('')
  const [loading, setLoading] = useState(false)
  const [expandedMeta, setExpandedMeta] = useState<Set<string>>(new Set())
  const [expandedTools, setExpandedTools] = useState<Set<string>>(new Set())
  const [expandedThinking, setExpandedThinking] = useState<Set<string>>(new Set())
  const [headerCollapsed, setHeaderCollapsed] = useState(false)
  const [sessions, setSessions] = useState<SessionInfo[]>([])
  const [sessionsOpen, setSessionsOpen] = useState(false)
  const [responseStatus, setResponseStatus] = useState<string | null>(null) // null | 'sent' | 'received' | 'api_call'
  const [activeStreams, setActiveStreams] = useState<Set<string>>(new Set())
  const [viewingSession, setViewingSession] = useState<string | null>(null) // session key if viewing a specific session
  const [spawnCards, setSpawnCards] = useState<Map<string, SpawnCard>>(new Map())
  const [unreadCounts, setUnreadCounts] = useState<Map<string, number>>(new Map())
  const [inactiveAgentsOpen, setInactiveAgentsOpen] = useState(false)
  const [recentlyCompleted, setRecentlyCompleted] = useState<Set<string>>(new Set())
  
  // New request tracking state
  const [requestStartTime, setRequestStartTime] = useState<number | null>(null)
  const [requestElapsedTime, setRequestElapsedTime] = useState<number>(0)

  const selectedRef = useRef(agentKey('claxon', 'inber'))
  const seenIds = useRef<Set<string>>(new Set())
  const wsRef = useRef<WebSocket | null>(null)
  const messagesEndRef = useRef<HTMLDivElement>(null)
  const messagesContainerRef = useRef<HTMLDivElement>(null)
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const userScrolledUpRef = useRef(false)
  const timerRef = useRef<ReturnType<typeof setInterval> | undefined>(undefined)

  // Fetch agents from registry
  useEffect(() => {
    const fetchAgents = async () => {
      try {
        const res = await fetch('/api/agents')
        if (!res.ok) return
        const data: { name: string; orchestrator: string; description: string; enabled: boolean }[] = await res.json()
        if (!data || data.length === 0) return

        const mapped: AgentInfo[] = data
          .filter(a => a.enabled)
          .map(a => ({
            id: a.name,
            name: AGENT_PROJECT[a.name] ? `${a.name} (${AGENT_PROJECT[a.name]})` : a.name,
            orchestrator: a.orchestrator,
            description: a.description || AGENT_PROJECT[a.name] || a.orchestrator,
            emoji: AGENT_EMOJI[a.name] || ORCH_ICON[a.orchestrator] || '⚙️',
            enabled: a.enabled,
          }))

        // Always include claxon (default inber agent) even if not in registry
        if (!mapped.find(a => a.id === 'claxon')) {
          mapped.unshift({ id: 'claxon', name: 'claxon', orchestrator: 'inber', description: 'Main orchestrator', emoji: '🦀', enabled: true })
        }

        setAgents(mapped)
      } catch { /* use fallback */ }
    }
    fetchAgents()
  }, [])

  // Fetch history from logstack on mount and agent change
  const fetchHistory = useCallback(async (agentId: string) => {
    setLoading(true)
    const [agentName] = agentId.split(':')
    // Logstack stores OpenClaw main agent as 'claxon' (session dir name)
    const logstackAgent = agentName === 'main' ? 'claxon' : agentName
    try {
      // Fetch from both si (bus traffic) and openclaw (session logs) sources
      const [siResp, ocResp] = await Promise.all([
        fetch(`/api/logs?source=si&agent=${logstackAgent}&limit=200`),
        fetch(`/api/logs?source=openclaw&agent=${logstackAgent}&limit=200`),
      ])
      const siData = siResp.ok ? await siResp.json() : { logs: [] }
      const ocData = ocResp.ok ? await ocResp.json() : { logs: [] }
      const data = { logs: [...(siData.logs || []), ...(ocData.logs || [])] }

      const logs = data.logs || []
      const msgs: Message[] = []
      const toolEvents: LogToolEvent[] = []
      for (const entry of logs) {
        const result = logEntryToMessage(entry)
        if (result.message) {
          msgs.push(result.message)
          seenIds.current.add(result.message.id)
        }
        if (result.toolEvent) {
          toolEvents.push(result.toolEvent)
        }
      }

      // Attach collected tool/thinking events to their parent messages (by stream_id)
      for (const te of toolEvents) {
        const parent = msgs.find(m => m.id === te.streamId)
        if (!parent) continue
        if (te.stream === 'thinking' && te.thinkingText) {
          parent.thinking = (parent.thinking || '') + te.thinkingText
        } else if (te.tool) {
          if (!parent.meta) parent.meta = {} as MessageMeta
          if (!parent.meta.tools) parent.meta.tools = []
          if (te.stream === 'tool_call') {
            parent.meta.tools.push({ tool: te.tool.tool, input: te.tool.input })
          } else {
            // Match result to the last pending call for this tool
            const tools = parent.meta.tools
            let matched = false
            for (let i = tools.length - 1; i >= 0; i--) {
              if (tools[i].tool === te.tool.tool && !tools[i].output) {
                tools[i] = { ...tools[i], output: te.tool.output, error: te.tool.error }
                matched = true
                break
              }
            }
            if (!matched) {
              tools.push(te.tool)
            }
          }
          parent.meta.toolCalls = parent.meta.tools.length
        }
      }

      // Sort by timestamp ascending
      msgs.sort((a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime())

      // Deduplicate — messages exist in both si and openclaw sources with
      // slightly different timestamps. Use role + content + rounded time (5min window).
      const seen = new Set<string>()
      const deduped = msgs.filter(m => {
        const timeSlot = Math.floor(new Date(m.timestamp).getTime() / 300000) // 5-min bucket
        const key = `${m.role}:${timeSlot}:${m.content.slice(0, 150)}`
        if (seen.has(key)) return false
        seen.add(key)
        return true
      })

      setAllMessages(deduped)
    } catch (err) {
      console.error('Failed to fetch history:', err)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchHistory(selected)
  }, [fetchHistory, selected])

  // Fetch active sessions periodically.
  const fetchSessions = useCallback(async () => {
    try {
      const res = await fetch('/api/gateway/sessions?requests=true')
      if (!res.ok) return
      const data = await res.json()
      if (Array.isArray(data)) setSessions(data)
    } catch { /* gateway may not be available */ }
  }, [])

  useEffect(() => {
    fetchSessions()
    const interval = setInterval(fetchSessions, 5000)
    return () => clearInterval(interval)
  }, [fetchSessions])

  // Send a message to a specific session (for steering/interrupting).
  const sendToSession = useCallback(async (sessionKey: string, message: string) => {
    try {
      await fetch(`/api/gateway/sessions/${encodeURIComponent(sessionKey)}/inject`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ message }),
      })
    } catch (err) {
      console.error('Failed to inject into session:', err)
    }
  }, [])

  const scrollToBottom = useCallback((force = false) => {
    if (!force && userScrolledUpRef.current) return
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [])

  // Track user scroll position to determine if they've scrolled up
  useEffect(() => {
    const container = messagesContainerRef.current
    if (!container) return
    const handleScroll = () => {
      const threshold = 150 // px from bottom
      const atBottom = container.scrollHeight - container.scrollTop - container.clientHeight < threshold
      userScrolledUpRef.current = !atBottom
    }
    container.addEventListener('scroll', handleScroll, { passive: true })
    return () => container.removeEventListener('scroll', handleScroll)
  }, [])

  // Auto-scroll on new messages (respects user scroll position)
  useEffect(() => {
    scrollToBottom()
  }, [allMessages, scrollToBottom])

  // Force-scroll on agent change
  useEffect(() => {
    userScrolledUpRef.current = false
    scrollToBottom(true)
  }, [selected, scrollToBottom])

  // Connect to WebSocket for live messages
  const connect = useCallback(() => {
    if (wsRef.current?.readyState === WebSocket.OPEN) return

    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
    const ws = new WebSocket(`${proto}//${location.host}/ws`)
    wsRef.current = ws

    ws.onopen = () => setConnected(true)

    ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data)

        // Handle gateway events (spawn lifecycle) — arrive as outbound with channel="gateway".
        if (msg.type === 'event' && msg.message?.channel === 'gateway' && msg.message?.text) {
          let gw: GatewayEvent
          try { gw = JSON.parse(msg.message.text) } catch { return }
          // Update spawn cards for inline display.
          setSpawnCards(prev => {
            const next = new Map(prev)
            if (gw.kind === 'spawn_started') {
              next.set(gw.session_key, {
                sessionKey: gw.session_key,
                agent: gw.agent,
                parentKey: gw.parent_key,
                task: gw.task,
                status: 'running',
                timestamp: gw.timestamp,
              })
            } else if (gw.kind === 'spawn_completed') {
              const existing = next.get(gw.session_key)
              if (existing) {
                next.set(gw.session_key, {
                  ...existing,
                  status: gw.status === 'success' ? 'completed' : 'error',
                  summary: gw.summary,
                  tokens: gw.tokens,
                  durationMs: gw.duration_ms,
                  error: gw.error,
                })
              }
            }
            return next
          })
          // Update sessions state reactively.
          setSessions(prev => {
            const updated = [...prev]
            const idx = updated.findIndex(s => s.key === gw.session_key)
            if (idx >= 0) {
              if (gw.kind === 'session_active') updated[idx] = { ...updated[idx], status: 'running' }
              else if (gw.kind === 'session_idle') updated[idx] = { ...updated[idx], status: 'idle' }
              else if (gw.kind === 'spawn_completed') updated[idx] = { ...updated[idx], status: gw.status === 'success' ? 'completed' : 'error' }
            } else if (gw.kind === 'spawn_started') {
              updated.push({
                key: gw.session_key,
                agent: gw.agent,
                status: 'running',
                spawn_depth: 1,
                parent_key: gw.parent_key,
                created_at: gw.timestamp,
                last_active: gw.timestamp,
                messages: 0,
              })
            }
            return updated
          })
          return
        }

        if (msg.type === 'event' && msg.message) {
          const data = msg.message

          // Handle status events (granular progress indicators)
          if (data.stream === 'status') {
            setResponseStatus(data.text || 'received')
            return
          }
        }

        // Handle new inber status events (Fionn's new format)
        if (msg.type === 'status') {
          if (msg.status === 'processing') {
            setResponseStatus('processing')
          }
          return
        }

        // Handle new inber error events (Fionn's new format)  
        if (msg.type === 'error') {
          const errorInfo = {
            error: msg.error || 'An error occurred',
            code: msg.code
          }
          setResponseStatus(null)
          setRequestStartTime(null)

          // Add error message to conversation
          const [selAgent] = selectedRef.current.split(':')
          const errorMsg: Message = {
            id: Date.now().toString(),
            role: 'error',
            content: errorInfo.error,
            timestamp: new Date().toISOString(),
            agent: 'system',
            orchestrator: '',
            targetAgent: selAgent,
            error: errorInfo,
          }
          setAllMessages(prev => [...prev, errorMsg])
          return
        }

        if (msg.type === 'event' && msg.message) {
          const data = msg.message
          const isInbound = msg.event_type === 'inbound'

          // Clear status on any real streaming event
          if (data.stream && data.stream_id && data.stream !== 'status') {
            setResponseStatus(null)
            if (data.stream !== 'done') {
              setActiveStreams(prev => {
                const had = prev.has(data.stream_id)
                if (!had) {
                  // New stream starting — snap to bottom
                  userScrolledUpRef.current = false
                  setTimeout(() => messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' }), 50)
                }
                return new Set(prev).add(data.stream_id)
              })
            }
          }

          // Handle streaming deltas.
          if (data.stream === 'delta' && data.stream_id) {
            const streamId = data.stream_id
            setAllMessages(prev => {
              const existing = prev.find(m => m.id === streamId)
              if (existing) {
                // Append delta text to existing streaming message.
                return prev.map(m =>
                  m.id === streamId
                    ? { ...m, content: m.content + (data.text || '') }
                    : m
                )
              } else {
                // Create new streaming message placeholder.
                const targetAgent = data.author || data.agent
                return [...prev, {
                  id: streamId,
                  role: 'assistant' as const,
                  content: data.text || '',
                  timestamp: data.timestamp ? new Date(data.timestamp).toISOString() : new Date().toISOString(),
                  agent: data.agent || data.author,
                  orchestrator: data.orchestrator || '',
                  targetAgent,
                }]
              }
            })
            return
          }

          // Handle streaming tool calls and results — accumulate on the streaming message.
          if ((data.stream === 'tool_call' || data.stream === 'tool_result') && data.stream_id) {
            const streamId = data.stream_id
            const toolEvent = data.meta?.tools?.[0]
            if (toolEvent) {
              // Auto-expand tools during streaming
              setExpandedTools(prev => new Set(prev).add(streamId))
              setAllMessages(prev => {
                const existing = prev.find(m => m.id === streamId)
                if (existing) {
                  const existingTools = existing.meta?.tools || []
                  let updatedTools: ToolEvent[]
                  if (data.stream === 'tool_call') {
                    // Add new pending tool call
                    updatedTools = [...existingTools, { tool: toolEvent.tool, input: toolEvent.input }]
                  } else {
                    // Match result to the last pending call for this tool
                    updatedTools = [...existingTools]
                    for (let i = updatedTools.length - 1; i >= 0; i--) {
                      if (updatedTools[i].tool === toolEvent.tool && !updatedTools[i].output) {
                        updatedTools[i] = { ...updatedTools[i], output: toolEvent.output, error: toolEvent.error }
                        break
                      }
                    }
                  }
                  return prev.map(m =>
                    m.id === streamId
                      ? { ...m, meta: { ...(m.meta || {} as MessageMeta), tools: updatedTools, toolCalls: updatedTools.length } }
                      : m
                  )
                } else {
                  // Create placeholder with tool
                  const targetAgent = data.author || data.agent
                  const tool: ToolEvent = data.stream === 'tool_call'
                    ? { tool: toolEvent.tool, input: toolEvent.input }
                    : { tool: toolEvent.tool, output: toolEvent.output, error: toolEvent.error }
                  return [...prev, {
                    id: streamId,
                    role: 'assistant' as const,
                    content: '',
                    timestamp: new Date().toISOString(),
                    agent: data.agent || data.author,
                    orchestrator: data.orchestrator || '',
                    targetAgent,
                    meta: { toolCalls: 1, tools: [tool] } as MessageMeta,
                  }]
                }
              })
            }
            return
          }

          // Handle streaming thinking — accumulate on the streaming message.
          if (data.stream === 'thinking' && data.stream_id) {
            const streamId = data.stream_id
            setExpandedThinking(prev => new Set(prev).add(streamId))
            setAllMessages(prev => {
              const existing = prev.find(m => m.id === streamId)
              if (existing) {
                return prev.map(m =>
                  m.id === streamId
                    ? { ...m, thinking: (m.thinking || '') + (data.text || '') }
                    : m
                )
              } else {
                const targetAgent = data.author || data.agent
                return [...prev, {
                  id: streamId,
                  role: 'assistant' as const,
                  content: '',
                  timestamp: new Date().toISOString(),
                  agent: data.agent || data.author,
                  orchestrator: data.orchestrator || '',
                  targetAgent,
                  thinking: data.text || '',
                }]
              }
            })
            return
          }

          // Handle stream "done" — replace the streaming placeholder with final message.
          if (data.stream === 'done' && data.stream_id) {
            const streamId = data.stream_id
            setActiveStreams(prev => { const next = new Set(prev); next.delete(streamId); return next })
            // Clear request tracking when we get a complete response
            setRequestStartTime(null)
            setResponseStatus(null)
            const targetAgent = data.author || data.agent
            // Increment unread if this message is for a different agent
            if (targetAgent) {
              const msgOrch = data.orchestrator || ''
              // Find matching agent to build the key
              const matchKey = agents.find(a => a.id === targetAgent && (!msgOrch || a.orchestrator === msgOrch))
              const tKey = matchKey ? agentKey(matchKey.id, matchKey.orchestrator) : agentKey(targetAgent, msgOrch)
              if (tKey !== selectedRef.current) {
                setUnreadCounts(prev => { const next = new Map(prev); next.set(tKey, (next.get(tKey) || 0) + 1); return next })
              }
            }
            setAllMessages(prev => {
              const idx = prev.findIndex(m => m.id === streamId)
              const finalMsg: Message = {
                id: streamId,
                role: 'assistant',
                content: data.text || '',
                timestamp: data.timestamp ? new Date(data.timestamp).toISOString() : new Date().toISOString(),
                agent: data.agent || data.author,
                orchestrator: data.orchestrator || '',
                targetAgent,
                meta: parseMeta(data.meta),
              }
              if (idx >= 0) {
                // Replace streaming placeholder with final, preserving accumulated thinking and tools.
                const accumulated = prev[idx]
                if (accumulated.thinking) {
                  finalMsg.thinking = accumulated.thinking
                }
                if (accumulated.meta?.tools?.length) {
                  finalMsg.meta = {
                    ...(finalMsg.meta || {} as MessageMeta),
                    tools: accumulated.meta.tools,
                    toolCalls: accumulated.meta.tools.length,
                  }
                }
                return prev.map((m, i) => i === idx ? finalMsg : m)
              }
              // No placeholder found — just add it.
              if (seenIds.current.has(finalMsg.id)) return prev
              seenIds.current.add(finalMsg.id)
              return [...prev, finalMsg]
            })
            return
          }

          // Clear status and request tracking on any non-streamed assistant response
          if (!isInbound) {
            setResponseStatus(null)
            setRequestStartTime(null)
          }

          // Normal non-streamed message.
          const targetAgent = isInbound
            ? (data.agent || data.author)
            : (data.author || data.agent)

          // Filter out raw status text like "API CALL"
          const rawText = (data.text || '').trim().toUpperCase()
          if (!isInbound && (rawText === 'API CALL' || rawText === 'API_CALL' || rawText === 'TOOL CALL' || rawText === 'TOOL_CALL' || rawText === '')) {
            return
          }

          const newMsg: Message = {
            id: data.id || Date.now().toString(),
            role: isInbound ? 'user' : 'assistant',
            content: data.text || '',
            timestamp: data.timestamp ? new Date(data.timestamp).toISOString() : new Date().toISOString(),
            agent: data.agent || data.author,
            orchestrator: data.orchestrator || '',
            targetAgent,
            meta: parseMeta(data.meta),
          }

          // Deduplicate (logstack fetch + live WS might overlap)
          if (seenIds.current.has(newMsg.id)) return
          seenIds.current.add(newMsg.id)

          // Increment unread if this message is for a different agent
          if (targetAgent) {
            const msgOrch = data.orchestrator || ''
            const matchKey = agents.find(a => a.id === targetAgent && (!msgOrch || a.orchestrator === msgOrch))
            const tKey = matchKey ? agentKey(matchKey.id, matchKey.orchestrator) : agentKey(targetAgent, msgOrch)
            if (tKey !== selectedRef.current) {
              setUnreadCounts(prev => { const next = new Map(prev); next.set(tKey, (next.get(tKey) || 0) + 1); return next })
            }
          }

          setAllMessages(prev => [...prev, newMsg])
        }
      } catch (err) {
        console.error('Failed to parse WebSocket message:', err)
      }
    }

    ws.onclose = () => {
      setConnected(false)
      reconnectTimeoutRef.current = setTimeout(() => connect(), 3000)
    }

    ws.onerror = (error) => {
      console.error('WebSocket error:', error)
    }

    return () => { ws.close() }
  }, [])

  useEffect(() => {
    connect()
    return () => {
      if (reconnectTimeoutRef.current) clearTimeout(reconnectTimeoutRef.current)
      wsRef.current?.close()
    }
  }, [connect])

  selectedRef.current = selected

  const send = useCallback(() => {
    if (!input.trim() || !connected || !wsRef.current) return

    // Start request tracking
    const now = Date.now()
    setRequestStartTime(now)
    setRequestElapsedTime(0)

    // Don't do optimistic update - let WebSocket broadcast drive all message updates
    // This prevents duplicate messages (optimistic + broadcast)
    const [selAgent, selOrch] = selected.split(':')
    wsRef.current.send(JSON.stringify({
      text: input,
      author: 'slava',
      channel: 'websocket',
      agent: selAgent,
      orchestrator: selOrch,
    }))

    setInput('')
    setResponseStatus('sent')
  }, [input, connected, selected])

  const handleKeyPress = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      send()
    }
  }

  const toggleMeta = (msgId: string) => {
    setExpandedMeta(prev => {
      const next = new Set(prev)
      if (next.has(msgId)) next.delete(msgId)
      else next.add(msgId)
      return next
    })
  }

  const toggleThinking = (msgId: string) => {
    setExpandedThinking(prev => {
      const next = new Set(prev)
      if (next.has(msgId)) next.delete(msgId)
      else next.add(msgId)
      return next
    })
  }

  const toggleTools = (msgId: string) => {
    setExpandedTools(prev => {
      const next = new Set(prev)
      if (next.has(msgId)) next.delete(msgId)
      else next.add(msgId)
      return next
    })
  }

  // Format tool input/output for display
  const formatToolDisplay = (tool: ToolEvent): { label: string; detail: string; isFileEdit: boolean; path?: string; content?: string } => {
    const isFileEdit = ['write_file', 'edit_file', 'read_file'].includes(tool.tool)

    if (isFileEdit && tool.input) {
      try {
        const parsed = JSON.parse(tool.input)
        const path = parsed.path || parsed.file || ''
        const content = parsed.content || parsed.new_text || ''
        return {
          label: tool.tool.replace('_', ' '),
          detail: path,
          isFileEdit: true,
          path,
          content,
        }
      } catch {
        // Fall through to default
      }
    }

    // Generic tool display
    let detail = ''
    if (tool.input) {
      try {
        const parsed = JSON.parse(tool.input)
        const keys = Object.keys(parsed).slice(0, 2)
        detail = keys.map(k => `${k}=${JSON.stringify(parsed[k]).slice(0, 30)}`).join(', ')
      } catch {
        detail = tool.input.slice(0, 50)
      }
    }

    return { label: tool.tool, detail, isFileEdit: false }
  }

  const currentAgentInfo = agents.find(a => agentKey(a.id, a.orchestrator) === selected) || agents[0]
  const [selAgent] = selected.split(':')

  // Filter messages for the selected agent.
  // Match on targetAgent name. If orchestrator data is available, filter on that too.
  // Apply the same name mapping used for logstack queries (e.g. main↔claxon).
  const AGENT_NAME_ALIASES: Record<string, string[]> = {
    'main': ['main', 'claxon'],
    'claxon': ['claxon', 'main'],
  }
  const selAgentNames = AGENT_NAME_ALIASES[selAgent] || [selAgent]

  const currentMessages = allMessages.filter(msg => {
    if (!msg.targetAgent) return false
    if (!selAgentNames.includes(msg.targetAgent)) return false
    // If the message has orchestrator info, check it matches
    if (msg.orchestrator && currentAgentInfo?.orchestrator && msg.orchestrator !== currentAgentInfo.orchestrator) {
      return false
    }
    return true
  })

  // Group agents by orchestrator for the switcher
  const orchGroups = agents.reduce<Record<string, AgentInfo[]>>((acc, a) => {
    if (!acc[a.orchestrator]) acc[a.orchestrator] = []
    acc[a.orchestrator].push(a)
    return acc
  }, {})

  // Sort orchestrators: known ones first, then alphabetical
  const ORCH_ORDER: Record<string, number> = { openclaw: 0, inber: 1 }
  const sortedOrchs = Object.keys(orchGroups).sort((a, b) =>
    (ORCH_ORDER[a] ?? 50) - (ORCH_ORDER[b] ?? 50)
  )

  const selectedOrch = currentAgentInfo?.orchestrator || ''

  const selectAgent = (a: AgentInfo) => {
    const key = agentKey(a.id, a.orchestrator)
    setSelected(key)
    selectedRef.current = key
    setUnreadCounts(prev => { const next = new Map(prev); next.delete(key); return next })
  }

  const selectOrchestrator = (orch: string) => {
    const orchAgents = orchGroups[orch] || []
    const mainId = MAIN_AGENTS[orch] || orchAgents[0]?.id
    const defaultAgent = orchAgents.find(a => a.id === mainId) || orchAgents[0]
    if (defaultAgent) selectAgent(defaultAgent)
  }

  const jumpToAgent = (agentName: string) => {
    const target = agents.find(a => a.id === agentName)
    if (target) selectAgent(target)
  }

  // Spawn cards sorted by timestamp for inline display
  const sortedSpawnCards = Array.from(spawnCards.values())
    .sort((a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime())

  // Active sub-agents for the sidebar
  const activeSubAgents = Array.from(spawnCards.values())
    .filter(s => s.status === 'running')
    .sort((a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime())

  // Determine agent activity status from sessions + spawnCards
  const getAgentStatus = (agentId: string): 'active' | 'completed' | 'idle' => {
    // Check sessions for running status
    const sess = sessions.find(s => s.agent === agentId && s.status === 'running')
    if (sess) return 'active'
    // Check spawn cards
    const spawns = Array.from(spawnCards.values()).filter(s => s.agent === agentId)
    if (spawns.some(s => s.status === 'running')) return 'active'
    if (recentlyCompleted.has(agentId)) return 'completed'
    return 'idle'
  }

  // Timer effect for request tracking
  useEffect(() => {
    if (requestStartTime) {
      timerRef.current = setInterval(() => {
        const elapsed = Date.now() - requestStartTime
        setRequestElapsedTime(elapsed)

        // Auto-timeout after 5 minutes (300,000ms) to match inber's timeout
        if (elapsed >= 300000) {
          const [selAgent] = selectedRef.current.split(':')
          const timeoutMsg: Message = {
            id: Date.now().toString(),
            role: 'error',
            content: 'Request timed out after 5 minutes. The agent may still be processing your request, but no response was received within the expected time.',
            timestamp: new Date().toISOString(),
            agent: 'system',
            orchestrator: '',
            targetAgent: selAgent,
            error: {
              error: 'Request timeout',
              code: 'TIMEOUT'
            },
          }
          
          setAllMessages(prev => [...prev, timeoutMsg])
          setRequestStartTime(null)
          setResponseStatus(null)
          
          if (timerRef.current) {
            clearInterval(timerRef.current)
            timerRef.current = undefined
          }
        }
      }, 100) // Update every 100ms for smooth timer

      return () => {
        if (timerRef.current) {
          clearInterval(timerRef.current)
          timerRef.current = undefined
        }
      }
    } else {
      if (timerRef.current) {
        clearInterval(timerRef.current)
        timerRef.current = undefined
      }
    }
  }, [requestStartTime])

  // Track recently completed agents (fade checkmark after 5s)
  useEffect(() => {
    const completedAgents = new Set<string>()
    for (const sc of spawnCards.values()) {
      if (sc.status === 'completed' || sc.status === 'error') {
        completedAgents.add(sc.agent)
      }
    }
    for (const sess of sessions) {
      if (sess.status === 'completed') {
        completedAgents.add(sess.agent)
      }
    }
    // Only add newly completed ones
    const newlyCompleted = new Set<string>()
    completedAgents.forEach(a => {
      if (!recentlyCompleted.has(a)) newlyCompleted.add(a)
    })
    if (newlyCompleted.size > 0) {
      setRecentlyCompleted(prev => new Set([...prev, ...newlyCompleted]))
      // Clear after 5 seconds
      const timeout = setTimeout(() => {
        setRecentlyCompleted(prev => {
          const next = new Set(prev)
          newlyCompleted.forEach(a => next.delete(a))
          return next
        })
      }, 5000)
      return () => clearTimeout(timeout)
    }
  }, [spawnCards, sessions])

  // Split agents for current orchestrator into main, active, and inactive
  const currentOrchAgents = orchGroups[selectedOrch] || []
  const mainAgentId = MAIN_AGENTS[selectedOrch] || currentOrchAgents[0]?.id
  const mainAgent = currentOrchAgents.find(a => a.id === mainAgentId)
  const otherAgents = currentOrchAgents.filter(a => a.id !== mainAgentId)
  const activeAgents = otherAgents.filter(a => getAgentStatus(a.id) === 'active' || getAgentStatus(a.id) === 'completed')
  const inactiveAgents = otherAgents.filter(a => getAgentStatus(a.id) === 'idle')

  // Also check active streams for agent activity
  const isAgentStreaming = (agentId: string): boolean => {
    return allMessages.some(m => m.targetAgent === agentId && activeStreams.has(m.id))
  }

  return (
    <div className={styles.container}>
      {/* Compact mobile header - tap to expand */}
      <div
        className={styles.mobileHeader}
        onClick={() => setHeaderCollapsed(!headerCollapsed)}
      >
        <div className={styles.mobileHeaderLeft}>
          <span className={styles.mobileAvatar}>{currentAgentInfo?.emoji}</span>
          <span className={styles.mobileAgentName}>{currentAgentInfo?.name}</span>
          <span className={styles.mobileOrchestrator}>{currentAgentInfo?.orchestrator}</span>
        </div>
        <div className={styles.mobileHeaderRight}>
          <div className={`${styles.connectionStatus} ${connected ? styles.connected : styles.disconnected}`}>
            <span className={styles.statusDot} />
          </div>
          <span className={styles.expandIcon}>{headerCollapsed ? '▾' : '▸'}</span>
        </div>
      </div>

      {/* Expandable section: orchestrator tabs + agent sub-nav */}
      <div className={`${styles.expandableSection} ${headerCollapsed ? styles.collapsed : ''}`}>
        {/* Primary orchestrator tabs */}
        <nav className={styles.orchNav}>
          {sortedOrchs.map(orch => (
            <button
              key={orch}
              className={`${styles.orchTab} ${orch === selectedOrch ? styles.orchTabActive : ''}`}
              onClick={() => selectOrchestrator(orch)}
            >
              <span className={styles.orchTabIcon}>{ORCH_ICON[orch] || '⚙️'}</span>
              <span className={styles.orchTabName}>{orch}</span>
            </button>
          ))}
          <div className={styles.orchNavSpacer} />
          <div className={`${styles.connectionStatus} ${connected ? styles.connected : styles.disconnected}`}>
            <span className={styles.statusDot} />
            {connected ? 'Connected' : 'Connecting...'}
          </div>
        </nav>

        {/* Agent sub-nav: main agent prominent, active agents visible, inactive collapsed */}
        {currentOrchAgents.length > 1 && (
          <nav className={styles.agentNav}>
            {/* Main orchestrator agent — always prominent */}
            {mainAgent && (() => {
              const key = agentKey(mainAgent.id, mainAgent.orchestrator)
              const unread = unreadCounts.get(key) || 0
              const status = getAgentStatus(mainAgent.id)
              const streaming = isAgentStreaming(mainAgent.id)
              return (
                <button
                  key={key}
                  className={`${styles.agentTab} ${styles.agentTabMain} ${key === selected ? styles.agentTabActive : ''} ${unread > 0 ? styles.agentTabUnread : ''}`}
                  onClick={() => selectAgent(mainAgent)}
                  title={mainAgent.description}
                >
                  <span className={styles.agentTabEmoji}>{mainAgent.emoji}</span>
                  <span className={styles.agentTabName}>{mainAgent.name}</span>
                  {(status === 'active' || streaming) && <span className={styles.agentStatusDot} />}
                  {status === 'completed' && <span className={styles.agentStatusCheck}>✓</span>}
                  {unread > 0 && (
                    <span className={styles.unreadBadge}>{unread > 99 ? '99+' : unread}</span>
                  )}
                </button>
              )
            })()}

            {/* Active agents — shown prominently with status indicators */}
            {activeAgents.map(a => {
              const key = agentKey(a.id, a.orchestrator)
              const unread = unreadCounts.get(key) || 0
              const status = getAgentStatus(a.id)
              const streaming = isAgentStreaming(a.id)
              return (
                <button
                  key={key}
                  className={`${styles.agentTab} ${key === selected ? styles.agentTabActive : ''} ${unread > 0 ? styles.agentTabUnread : ''}`}
                  onClick={() => selectAgent(a)}
                  title={a.description}
                >
                  <span className={styles.agentTabName}>{a.name}</span>
                  {(status === 'active' || streaming) && <span className={styles.agentStatusDot} />}
                  {status === 'completed' && <span className={styles.agentStatusCheck}>✓</span>}
                  {unread > 0 && (
                    <span className={styles.unreadBadge}>{unread > 99 ? '99+' : unread}</span>
                  )}
                </button>
              )
            })}

            {/* Inactive agents — collapsed */}
            {inactiveAgents.length > 0 && (
              <div className={styles.inactiveAgentsWrapper}>
                <button
                  className={styles.inactiveAgentsToggle}
                  onClick={() => setInactiveAgentsOpen(!inactiveAgentsOpen)}
                >
                  <span>Other ({inactiveAgents.length})</span>
                  <span className={styles.inactiveAgentsArrow}>{inactiveAgentsOpen ? '▾' : '▸'}</span>
                </button>
                {inactiveAgentsOpen && (
                  <div className={styles.inactiveAgentsDropdown}>
                    {inactiveAgents.map(a => {
                      const key = agentKey(a.id, a.orchestrator)
                      const unread = unreadCounts.get(key) || 0
                      return (
                        <button
                          key={key}
                          className={`${styles.inactiveAgentItem} ${key === selected ? styles.agentTabActive : ''}`}
                          onClick={() => { selectAgent(a); setInactiveAgentsOpen(false) }}
                          title={a.description}
                        >
                          <span>{a.name}</span>
                          {unread > 0 && (
                            <span className={styles.unreadBadge}>{unread > 99 ? '99+' : unread}</span>
                          )}
                        </button>
                      )
                    })}
                  </div>
                )}
              </div>
            )}
          </nav>
        )}
      </div>

      {/* Sessions sidebar */}
      {sessions.length > 0 && (
        <div className={styles.sessionsBar}>
          <button
            className={styles.sessionsToggle}
            onClick={() => setSessionsOpen(!sessionsOpen)}
          >
            <span>📡 Sessions ({sessions.filter(s => s.status === 'running').length} active)</span>
            <span>{sessionsOpen ? '▾' : '▸'}</span>
          </button>

          {sessionsOpen && (
            <div className={styles.sessionsList}>
              {sessions
                .sort((a, b) => {
                  // Running first, then by last_active desc.
                  if (a.status === 'running' && b.status !== 'running') return -1
                  if (b.status === 'running' && a.status !== 'running') return 1
                  return new Date(b.last_active).getTime() - new Date(a.last_active).getTime()
                })
                .map(sess => {
                  const isViewing = viewingSession === sess.key
                  const statusIcon = sess.status === 'running' ? '🟢' : sess.status === 'error' ? '🔴' : sess.status === 'completed' ? '✅' : '⚪'
                  const depth = sess.spawn_depth || 0

                  return (
                    <div
                      key={sess.key}
                      className={`${styles.sessionItem} ${isViewing ? styles.sessionItemActive : ''}`}
                      style={{ paddingLeft: `${0.5 + depth * 1}rem` }}
                    >
                      <div className={styles.sessionItemHeader}>
                        <span className={styles.sessionStatus}>{statusIcon}</span>
                        <span className={styles.sessionAgent}>{sess.agent}</span>
                        <span className={styles.sessionMsgCount}>{sess.messages} msgs</span>
                      </div>
                      {sess.active_request && (
                        <div className={styles.sessionTask}>
                          {typeof sess.active_request.input_text === 'string'
                            ? sess.active_request.input_text.slice(0, 60)
                            : '...'}
                        </div>
                      )}
                      <div className={styles.sessionActions}>
                        <button
                          className={styles.sessionViewBtn}
                          onClick={() => setViewingSession(isViewing ? null : sess.key)}
                          title={isViewing ? 'Back to agent view' : 'View session'}
                        >
                          {isViewing ? '← Back' : '👁️'}
                        </button>
                        {sess.status === 'running' && (
                          <button
                            className={styles.sessionSteerBtn}
                            onClick={() => {
                              const msg = prompt(`Message to ${sess.agent}:`)
                              if (msg) sendToSession(sess.key, msg)
                            }}
                            title="Send message to this session"
                          >
                            💬
                          </button>
                        )}
                      </div>
                    </div>
                  )
                })}
            </div>
          )}
        </div>
      )}

      <div className={styles.chatArea}>
      {/* Floating sub-agent activity panel */}
      {activeSubAgents.length > 0 && (
        <div className={styles.subAgentPanel}>
          <div className={styles.subAgentPanelHeader}>
            <span className={styles.subAgentPanelTitle}>🔄 Active Sub-agents</span>
            <span className={styles.subAgentPanelCount}>{activeSubAgents.length}</span>
          </div>
          {activeSubAgents.map(sc => (
            <div key={sc.sessionKey} className={styles.subAgentItem}>
              <div className={styles.subAgentItemHeader}>
                <span className={styles.subAgentDot} />
                <span className={styles.subAgentName}>{sc.agent}</span>
              </div>
              <div className={styles.subAgentTask}>
                {sc.task ? sc.task.slice(0, 50) + (sc.task.length > 50 ? '…' : '') : 'Working...'}
              </div>
              <button
                className={styles.subAgentJump}
                onClick={() => jumpToAgent(sc.agent)}
              >
                View →
              </button>
            </div>
          ))}
        </div>
      )}

      <div className={styles.messages} ref={messagesContainerRef}>
        {loading ? (
          <div className={styles.empty}>
            <p className={styles.emptyText}>Loading history...</p>
          </div>
        ) : currentMessages.length === 0 && sortedSpawnCards.length === 0 ? (
          <div className={styles.empty}>
            <div className={styles.emptyEmoji}>{currentAgentInfo?.emoji}</div>
            <p className={styles.emptyTitle}>Chat with {currentAgentInfo?.name}</p>
            <p className={styles.emptyText}>Send a message to start the conversation</p>
          </div>
        ) : (
          <>
            {/* Build an interleaved timeline of messages + spawn cards */}
            {[
              ...currentMessages.map(m => ({ kind: 'message' as const, data: m, ts: new Date(m.timestamp).getTime() })),
              ...sortedSpawnCards.map(sc => ({ kind: 'spawn' as const, data: sc, ts: new Date(sc.timestamp).getTime() })),
            ]
              .sort((a, b) => a.ts - b.ts)
              .map((item) => {
                if (item.kind === 'spawn') {
                  const sc = item.data as SpawnCard
                  return (
                    <div key={`spawn-${sc.sessionKey}`} className={`${styles.spawnCard} ${sc.status === 'running' ? styles.spawnCardRunning : sc.status === 'error' ? styles.spawnCardError : styles.spawnCardDone}`}>
                      <div className={styles.spawnCardHeader}>
                        <span className={styles.spawnCardStatus}>
                          {sc.status === 'running' && <span className={styles.spawnSpinner}>⟳</span>}
                          {sc.status === 'completed' && '✅'}
                          {sc.status === 'error' && '❌'}
                        </span>
                        <span className={styles.spawnCardLabel}>Sub-agent</span>
                        <span className={styles.spawnCardAgent}>{sc.agent}</span>
                        <span className={styles.spawnCardTime}>
                          {new Date(sc.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
                        </span>
                      </div>
                      {sc.task && (
                        <div className={styles.spawnCardTask}>{sc.task.length > 120 ? sc.task.slice(0, 120) + '…' : sc.task}</div>
                      )}
                      {sc.status === 'completed' && sc.summary && (
                        <div className={styles.spawnCardSummary}>
                          <span className={styles.spawnCardSummaryLabel}>Result: </span>
                          {sc.summary.length > 200 ? sc.summary.slice(0, 200) + '…' : sc.summary}
                        </div>
                      )}
                      {sc.error && (
                        <div className={styles.spawnCardErrorText}>⚠ {sc.error}</div>
                      )}
                      <div className={styles.spawnCardFooter}>
                        {sc.durationMs != null && (
                          <span className={styles.spawnCardMeta}>⏱ {formatDuration(sc.durationMs)}</span>
                        )}
                        {sc.tokens && sc.tokens.cost > 0 && (
                          <span className={styles.spawnCardMeta}>
                            {formatTokens(sc.tokens.input + sc.tokens.output)} tok · {formatCost(sc.tokens.cost)}
                          </span>
                        )}
                        <button
                          className={styles.spawnCardJump}
                          onClick={() => jumpToAgent(sc.agent)}
                          title={`View ${sc.agent}'s conversation`}
                        >
                          View chat →
                        </button>
                      </div>
                    </div>
                  )
                }

                const msg = item.data as Message
                return (
              <div key={msg.id} className={`${styles.message} ${styles[msg.role]}`}>
                {msg.role === 'assistant' && (
                  <div className={styles.messageHeader}>
                    <span className={styles.messageAuthor}>{msg.agent || currentAgentInfo?.name}</span>
                    {msg.meta?.model && (
                      <span className={styles.messageModel}>{msg.meta.model}</span>
                    )}
                  </div>
                )}
                {msg.role === 'error' && (
                  <div className={styles.errorHeader}>
                    <span className={styles.errorIcon}>⚠️</span>
                    <span className={styles.errorLabel}>Error</span>
                    {msg.error?.code && (
                      <span className={styles.errorCode}>{msg.error.code}</span>
                    )}
                  </div>
                )}
                {/* Split layout: text left, tools right during streaming */}
                {activeStreams.has(msg.id) && msg.meta?.tools?.length ? (
                  <div className={styles.streamingSplit}>
                    <div className={styles.streamingText}>
                      {msg.content ? msg.content.split('\n').map((line, i) => (
                        <p key={i}>{line || '\u00A0'}</p>
                      )) : <span className={styles.typingIndicator}>…</span>}
                    </div>
                    <div className={styles.streamingTools}>
                      <div className={styles.streamingToolsHeader}>🔧 Tools</div>
                      {msg.meta.tools.map((tool, idx) => {
                        const display = formatToolDisplay(tool)
                        return (
                          <div key={idx} className={`${styles.toolItem} ${tool.error ? styles.toolError : ''} ${!tool.output ? styles.toolRunning : ''}`}>
                            <div className={styles.toolHeader}>
                              <span className={styles.toolName}>{display.label}</span>
                              {!tool.output && !tool.error && <span className={styles.toolSpinner}>⟳</span>}
                            </div>
                            {display.detail && <div className={styles.toolDetail}>{display.detail}</div>}
                            {tool.output && (
                              <div className={styles.toolOutput}>
                                <span className={styles.toolOutputLabel}>→</span>
                                <span className={styles.toolOutputText}>{tool.output}</span>
                              </div>
                            )}
                          </div>
                        )
                      })}
                    </div>
                  </div>
                ) : (
                  <div className={styles.messageContent}>
                    {msg.content.split('\n').map((line, i) => (
                      <p key={i}>{line || '\u00A0'}</p>
                    ))}
                  </div>
                )}
                <div className={styles.messageFooter}>
                  <span className={styles.messageTime}>
                    {new Date(msg.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
                  </span>
                  {msg.meta && (
                    <button
                      className={styles.metaToggle}
                      onClick={() => toggleMeta(msg.id)}
                      title="Toggle message details"
                    >
                      <span className={styles.metaSummary}>
                        {msg.meta.durationMs > 0 && (
                          <span className={styles.metaChipDuration}>⏱ {formatDuration(msg.meta.durationMs)}</span>
                        )}
                        {(msg.meta.inputTokens > 0 || msg.meta.outputTokens > 0) && (
                          <span className={styles.metaChipTokens}>
                            {formatTokens(msg.meta.inputTokens + msg.meta.outputTokens)} tok
                          </span>
                        )}
                        {msg.meta.cacheReadTokens > 0 && (
                          <span className={styles.metaChipCache}>
                            ⚡ {Math.round(msg.meta.cacheReadTokens / (msg.meta.inputTokens || 1) * 100)}%
                          </span>
                        )}
                        {msg.meta.toolCalls > 0 && (
                          <span className={styles.metaChipTools}>🔧 {msg.meta.toolCalls}</span>
                        )}
                        {msg.meta.cost > 0 && (
                          <span className={styles.metaChipCost}>{formatCost(msg.meta.cost)}</span>
                        )}
                      </span>
                      <span className={styles.metaExpandIcon}>
                        {expandedMeta.has(msg.id) ? '▾' : '▸'}
                      </span>
                    </button>
                  )}
                </div>

                {msg.meta && expandedMeta.has(msg.id) && (
                  <div className={styles.metaPanel}>
                    <div className={styles.metaGrid}>
                      {msg.meta.model && (
                        <div className={styles.metaCell}>
                          <span className={styles.metaLabel}>Model</span>
                          <span className={styles.metaValueModel}>{msg.meta.model}</span>
                        </div>
                      )}
                      {msg.meta.durationMs > 0 && (
                        <div className={styles.metaCell}>
                          <span className={styles.metaLabel}>Duration</span>
                          <span className={styles.metaValueDuration}>{formatDuration(msg.meta.durationMs)}</span>
                        </div>
                      )}
                      {msg.meta.inputTokens > 0 && (
                        <div className={styles.metaCell}>
                          <span className={styles.metaLabel}>Input</span>
                          <span className={styles.metaValueTokens}>{formatTokens(msg.meta.inputTokens)}</span>
                        </div>
                      )}
                      {msg.meta.outputTokens > 0 && (
                        <div className={styles.metaCell}>
                          <span className={styles.metaLabel}>Output</span>
                          <span className={styles.metaValueTokens}>{formatTokens(msg.meta.outputTokens)}</span>
                        </div>
                      )}
                      {(msg.meta.cacheReadTokens > 0 || msg.meta.cacheCreationTokens > 0) && (
                        <div className={styles.metaCell}>
                          <span className={styles.metaLabel}>Cache</span>
                          <span className={styles.metaValueCache}>
                            {formatTokens(msg.meta.cacheReadTokens)} read
                            {msg.meta.cacheCreationTokens > 0 && ` / ${formatTokens(msg.meta.cacheCreationTokens)} new`}
                          </span>
                          {msg.meta.inputTokens > 0 && (
                            <div className={styles.cacheBar}>
                              <div
                                className={styles.cacheBarFill}
                                style={{ width: `${Math.min(100, Math.round(msg.meta.cacheReadTokens / msg.meta.inputTokens * 100))}%` }}
                              />
                            </div>
                          )}
                        </div>
                      )}
                      {msg.meta.toolCalls > 0 && (
                        <div className={styles.metaCell}>
                          <span className={styles.metaLabel}>Tools</span>
                          <span className={styles.metaValueTools}>{msg.meta.toolCalls} calls</span>
                        </div>
                      )}
                      {msg.meta.cost > 0 && (
                        <div className={styles.metaCell}>
                          <span className={styles.metaLabel}>Cost</span>
                          <span className={styles.metaValueCost}>{formatCost(msg.meta.cost)}</span>
                        </div>
                      )}
                      {msg.meta.turn > 0 && (
                        <div className={styles.metaRow}>
                          <span className={styles.metaLabel}>Turn</span>
                          <span className={styles.metaValue}>#{msg.meta.turn}</span>
                        </div>
                      )}
                    </div>
                  </div>
                )}

                {/* Collapsible thinking section */}
                {msg.thinking && (
                  <div className={styles.thinkingSection}>
                    <button
                      className={styles.thinkingToggle}
                      onClick={() => toggleThinking(msg.id)}
                    >
                      <span className={styles.thinkingToggleIcon}>
                        {expandedThinking.has(msg.id) ? '▾' : '▸'}
                      </span>
                      <span className={styles.thinkingToggleLabel}>
                        💭 thinking
                      </span>
                    </button>

                    {expandedThinking.has(msg.id) && (
                      <div className={styles.thinkingContent}>
                        {msg.thinking.split('\n').map((line, i) => (
                          <p key={i}>{line || '\u00A0'}</p>
                        ))}
                      </div>
                    )}
                  </div>
                )}

                {/* Collapsible tool calls section (hidden during streaming — shown in vertical split instead) */}
                {msg.meta?.tools && msg.meta.tools.length > 0 && !activeStreams.has(msg.id) && (
                  <div className={styles.toolsSection}>
                    <button
                      className={styles.toolsToggle}
                      onClick={() => toggleTools(msg.id)}
                    >
                      <span className={styles.toolsToggleIcon}>
                        {expandedTools.has(msg.id) ? '▾' : '▸'}
                      </span>
                      <span className={styles.toolsToggleLabel}>
                        🔧 {msg.meta.tools.length} tool{msg.meta.tools.length > 1 ? 's' : ''}
                      </span>
                    </button>

                    {expandedTools.has(msg.id) && (
                      <div className={styles.toolsList}>
                        {msg.meta.tools.map((tool, idx) => {
                          const display = formatToolDisplay(tool)
                          return (
                            <div key={idx} className={`${styles.toolItem} ${tool.error ? styles.toolError : ''}`}>
                              <div className={styles.toolHeader}>
                                <span className={styles.toolName}>{display.label}</span>
                                {display.detail && (
                                  <span className={styles.toolDetail}>{display.detail}</span>
                                )}
                                {tool.error && <span className={styles.toolErrorBadge}>error</span>}
                              </div>

                              {/* Show output for all tools */}
                              {tool.output && (
                                <div className={styles.toolOutput}>
                                  <span className={styles.toolOutputLabel}>→</span>
                                  <span className={styles.toolOutputText}>{tool.output}</span>
                                </div>
                              )}
                            </div>
                          )
                        })}
                      </div>
                    )}
                  </div>
                )}
              </div>
                )
              })}
            {(responseStatus || requestStartTime) && (
              <div className={`${styles.message} ${styles.assistant}`}>
                <div className={styles.statusIndicator}>
                  <span className={styles.statusDotPulse} />
                  <span className={styles.statusText}>
                    {responseStatus === 'sent' && 'Sending to bus…'}
                    {responseStatus === 'received' && `${currentAgentInfo?.emoji || '⚙️'} ${currentAgentInfo?.name || 'Agent'} received — loading context…`}
                    {responseStatus === 'api_call' && `${currentAgentInfo?.emoji || '⚙️'} ${currentAgentInfo?.name || 'Agent'} calling API — awaiting response…`}
                    {responseStatus === 'processing' && `${currentAgentInfo?.emoji || '⚙️'} ${currentAgentInfo?.name || 'Agent'} processing request…`}
                  </span>
                  {requestStartTime && (
                    <span className={`${styles.requestTimer} ${requestElapsedTime > 60000 ? styles.requestTimerWarning : ''} ${requestElapsedTime > 300000 ? styles.requestTimerError : ''}`}>
                      ⏱ {formatElapsedTime(requestElapsedTime)}
                      {requestElapsedTime > 60000 && requestElapsedTime < 300000 && ' (slow response)'}
                      {requestElapsedTime >= 300000 && ' (timeout warning)'}
                    </span>
                  )}
                </div>
              </div>
            )}
            <div ref={messagesEndRef} />
          </>
        )}
      </div>
      </div>
      <div className={styles.inputArea}>
        <textarea
          className={styles.input}
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={handleKeyPress}
          placeholder={`Message ${currentAgentInfo?.name} (${currentAgentInfo?.orchestrator})...`}
          rows={2}
          disabled={!connected}
        />
        <button
          onClick={send}
          className={styles.sendButton}
          disabled={!connected || !input.trim()}
        >
          Send
        </button>
      </div>
    </div>
  )
}
