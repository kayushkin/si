import { useEffect, useState } from 'react'
import styles from './InberConfig.module.css'

interface AgentConfig {
  slug: string
  display_name: string
  emoji: string
  model: string
  max_turns: number
  max_input_tokens: number
  max_response_time: number
  thinking_budget: number
  context_budget: number
  enabled: boolean
  shelved: boolean
}

interface EditState {
  model: string
  max_turns: string
  max_input_tokens: string
  max_response_time: string
  thinking_budget: string
  context_budget: string
}

const MODELS = [
  'claude-opus-4-6',
  'claude-sonnet-4-6',
  'claude-sonnet-4-5',
  'claude-sonnet-4-5-20250929',
  'glm-5',
  'glm-4-flash',
]

export default function InberConfig() {
  const [agents, setAgents] = useState<AgentConfig[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [edits, setEdits] = useState<Record<string, EditState>>({})
  const [saving, setSaving] = useState<string | null>(null)
  const [saveMsg, setSaveMsg] = useState<Record<string, string>>({})

  const fetchAgents = async () => {
    try {
      const res = await fetch('/api/agents/config')
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      const data = await res.json()
      setAgents(data || [])
      setError(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to fetch')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchAgents()
    const interval = setInterval(fetchAgents, 30000)
    return () => clearInterval(interval)
  }, [])

  const getEdit = (slug: string, agent: AgentConfig): EditState => {
    if (edits[slug]) return edits[slug]
    return {
      model: agent.model,
      max_turns: String(agent.max_turns),
      max_input_tokens: String(agent.max_input_tokens),
      max_response_time: String(agent.max_response_time),
      thinking_budget: String(agent.thinking_budget),
      context_budget: String(agent.context_budget),
    }
  }

  const setEdit = (slug: string, field: keyof EditState, value: string) => {
    const agent = agents.find(a => a.slug === slug)
    if (!agent) return
    const current = getEdit(slug, agent)
    setEdits(prev => ({ ...prev, [slug]: { ...current, [field]: value } }))
  }

  const hasChanges = (slug: string, agent: AgentConfig): boolean => {
    const edit = edits[slug]
    if (!edit) return false
    return (
      edit.model !== agent.model ||
      edit.max_turns !== String(agent.max_turns) ||
      edit.max_input_tokens !== String(agent.max_input_tokens) ||
      edit.max_response_time !== String(agent.max_response_time) ||
      edit.thinking_budget !== String(agent.thinking_budget) ||
      edit.context_budget !== String(agent.context_budget)
    )
  }

  const save = async (slug: string, agent: AgentConfig) => {
    const edit = getEdit(slug, agent)
    setSaving(slug)
    setSaveMsg(prev => ({ ...prev, [slug]: '' }))
    try {
      const body: Record<string, unknown> = { slug }
      if (edit.model !== agent.model) body.model = edit.model
      if (edit.max_turns !== String(agent.max_turns)) body.max_turns = parseInt(edit.max_turns) || 0
      if (edit.max_input_tokens !== String(agent.max_input_tokens)) body.max_input_tokens = parseInt(edit.max_input_tokens) || 0
      if (edit.max_response_time !== String(agent.max_response_time)) body.max_response_time = parseInt(edit.max_response_time) || 0
      if (edit.thinking_budget !== String(agent.thinking_budget)) body.thinking_budget = parseInt(edit.thinking_budget) || 0
      if (edit.context_budget !== String(agent.context_budget)) body.context_budget = parseInt(edit.context_budget) || 0

      const res = await fetch('/api/agents/config', {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      if (!res.ok) {
        const data = await res.json()
        throw new Error(data.error || `HTTP ${res.status}`)
      }
      setSaveMsg(prev => ({ ...prev, [slug]: '✓' }))
      setEdits(prev => { const next = { ...prev }; delete next[slug]; return next })
      fetchAgents()
      setTimeout(() => setSaveMsg(prev => ({ ...prev, [slug]: '' })), 2000)
    } catch (e) {
      setSaveMsg(prev => ({ ...prev, [slug]: e instanceof Error ? e.message : 'Failed' }))
    } finally {
      setSaving(null)
    }
  }

  const reset = (slug: string) => {
    setEdits(prev => { const next = { ...prev }; delete next[slug]; return next })
  }

  const fmtTokens = (n: number) => {
    if (n >= 1000000) return `${(n / 1000000).toFixed(1)}M`
    if (n >= 1000) return `${Math.round(n / 1000)}K`
    return String(n)
  }

  if (loading) return <div className={styles.container}><p>Loading...</p></div>
  if (error) return <div className={styles.container}><p className={styles.error}>Error: {error}</p></div>

  const active = agents.filter(a => !a.shelved)
  const shelved = agents.filter(a => a.shelved)

  const renderAgent = (agent: AgentConfig) => {
    const edit = getEdit(agent.slug, agent)
    const changed = hasChanges(agent.slug, agent)
    const isSaving = saving === agent.slug
    const msg = saveMsg[agent.slug]

    return (
      <div key={agent.slug} className={`${styles.row} ${changed ? styles.rowChanged : ''}`}>
        <div className={styles.agentInfo}>
          <span className={styles.emoji}>{agent.emoji}</span>
          <span className={styles.name}>{agent.display_name || agent.slug}</span>
        </div>

        <div className={styles.fields}>
          <label className={styles.field}>
            <span className={styles.label}>Model</span>
            <select
              className={styles.select}
              value={edit.model}
              onChange={e => setEdit(agent.slug, 'model', e.target.value)}
            >
              {MODELS.map(m => <option key={m} value={m}>{m}</option>)}
              {!MODELS.includes(edit.model) && <option value={edit.model}>{edit.model}</option>}
            </select>
          </label>

          <label className={styles.field}>
            <span className={styles.label}>Max Turns</span>
            <input
              className={styles.input}
              type="number"
              value={edit.max_turns}
              onChange={e => setEdit(agent.slug, 'max_turns', e.target.value)}
              min={0}
            />
          </label>

          <label className={styles.field}>
            <span className={styles.label}>Max Tokens</span>
            <div className={styles.inputWithHint}>
              <input
                className={styles.input}
                type="number"
                value={edit.max_input_tokens}
                onChange={e => setEdit(agent.slug, 'max_input_tokens', e.target.value)}
                min={0}
                step={100000}
              />
              <span className={styles.hint}>{fmtTokens(parseInt(edit.max_input_tokens) || 0)}</span>
            </div>
          </label>

          <label className={styles.field}>
            <span className={styles.label}>Timeout (s)</span>
            <input
              className={styles.input}
              type="number"
              value={edit.max_response_time}
              onChange={e => setEdit(agent.slug, 'max_response_time', e.target.value)}
              min={0}
            />
          </label>

          <label className={styles.field}>
            <span className={styles.label}>Thinking</span>
            <input
              className={styles.input}
              type="number"
              value={edit.thinking_budget}
              onChange={e => setEdit(agent.slug, 'thinking_budget', e.target.value)}
              min={0}
            />
          </label>

          <label className={styles.field}>
            <span className={styles.label}>Context</span>
            <input
              className={styles.input}
              type="number"
              value={edit.context_budget}
              onChange={e => setEdit(agent.slug, 'context_budget', e.target.value)}
              min={0}
              step={1000}
            />
          </label>
        </div>

        <div className={styles.actions}>
          {changed && (
            <>
              <button
                className={styles.saveBtn}
                onClick={() => save(agent.slug, agent)}
                disabled={isSaving}
              >
                {isSaving ? '...' : 'Save'}
              </button>
              <button className={styles.resetBtn} onClick={() => reset(agent.slug)}>↩</button>
            </>
          )}
          {msg && <span className={msg === '✓' ? styles.successMsg : styles.errorMsg}>{msg}</span>}
        </div>
      </div>
    )
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <h2 className={styles.title}>🌿 Inber Config</h2>
        <span className={styles.count}>{active.length} agents</span>
      </div>

      <div className={styles.table}>
        {active.map(renderAgent)}
      </div>

      {shelved.length > 0 && (
        <>
          <h3 className={styles.sectionTitle}>Shelved ({shelved.length})</h3>
          <div className={`${styles.table} ${styles.shelved}`}>
            {shelved.map(renderAgent)}
          </div>
        </>
      )}
    </div>
  )
}
