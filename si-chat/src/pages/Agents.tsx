import { useEffect, useState } from 'react'
import styles from './Agents.module.css'

interface AgentEntry {
  name: string
  orchestrator: string
  description: string
  enabled: boolean
}

const KNOWN_ORCHESTRATORS = ['inber', 'openclaw', 'claude-code', 'codex']

export default function Agents() {
  const [agents, setAgents] = useState<AgentEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [collapsedSections, setCollapsedSections] = useState<Set<string>>(new Set())

  // New agent form
  const [showAdd, setShowAdd] = useState(false)
  const [newName, setNewName] = useState('')
  const [newOrch, setNewOrch] = useState('inber')
  const [newDesc, setNewDesc] = useState('')

  // Edit state (keyed by name:orchestrator)
  const [editing, setEditing] = useState<string | null>(null)
  const [editDesc, setEditDesc] = useState('')

  const fetchAgents = async () => {
    try {
      const res = await fetch('/api/agents')
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

  const addAgent = async () => {
    if (!newName.trim() || !newOrch.trim()) return
    try {
      const res = await fetch('/api/agents', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: newName.trim(), orchestrator: newOrch, description: newDesc.trim(), enabled: true }),
      })
      if (!res.ok) {
        const data = await res.json()
        setError(data.error || 'Failed to add')
        return
      }
      setNewName('')
      setNewDesc('')
      setShowAdd(false)
      fetchAgents()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to add')
    }
  }

  const updateAgent = async (agent: AgentEntry) => {
    try {
      await fetch('/api/agents', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(agent),
      })
      setEditing(null)
      fetchAgents()
    } catch { /* ignore */ }
  }

  const deleteAgent = async (name: string, orchestrator: string) => {
    if (!confirm(`Remove agent "${name}" from ${orchestrator}?`)) return
    try {
      await fetch(`/api/agents?name=${encodeURIComponent(name)}&orchestrator=${encodeURIComponent(orchestrator)}`, { method: 'DELETE' })
      fetchAgents()
    } catch { /* ignore */ }
  }

  const toggleEnabled = async (agent: AgentEntry) => {
    await updateAgent({ ...agent, enabled: !agent.enabled })
  }

  const toggleSection = (orch: string) => {
    setCollapsedSections(prev => {
      const next = new Set(prev)
      if (next.has(orch)) next.delete(orch)
      else next.add(orch)
      return next
    })
  }

  const editKey = (a: AgentEntry) => `${a.name}:${a.orchestrator}`

  const startEdit = (agent: AgentEntry) => {
    setEditing(editKey(agent))
    setEditDesc(agent.description)
  }

  const saveEdit = async (agent: AgentEntry) => {
    await updateAgent({ ...agent, description: editDesc })
  }

  if (loading) return <div className={styles.container}><p>Loading...</p></div>
  if (error) return <div className={styles.container}><p className={styles.errorMsg}>Error: {error}</p></div>

  // Group by orchestrator
  const grouped = agents.reduce<Record<string, AgentEntry[]>>((acc, a) => {
    const key = a.orchestrator
    if (!acc[key]) acc[key] = []
    acc[key].push(a)
    return acc
  }, {})

  const orchIcon = (orch: string) => {
    switch (orch) {
      case 'inber': return '🌿'
      case 'openclaw': return '🦀'
      case 'claude-code': return '💻'
      case 'codex': return '🤖'
      default: return '⚙️'
    }
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <h2 className={styles.title}>🎭 Agent Registry</h2>
        <div className={styles.headerRight}>
          <span className={styles.count}>{agents.length} agents</span>
          <button className={styles.addBtn} onClick={() => setShowAdd(!showAdd)}>
            {showAdd ? '✕ Cancel' : '+ Add Agent'}
          </button>
        </div>
      </div>

      {showAdd && (
        <div className={styles.addForm}>
          <input
            className={styles.input}
            placeholder="Agent name"
            value={newName}
            onChange={e => setNewName(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && addAgent()}
          />
          <select className={styles.select} value={newOrch} onChange={e => setNewOrch(e.target.value)}>
            {KNOWN_ORCHESTRATORS.map(o => (
              <option key={o} value={o}>{orchIcon(o)} {o}</option>
            ))}
          </select>
          <input
            className={styles.input}
            placeholder="Description (optional)"
            value={newDesc}
            onChange={e => setNewDesc(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && addAgent()}
          />
          <button className={styles.saveBtn} onClick={addAgent}>Add</button>
        </div>
      )}

      {Object.entries(grouped).sort().map(([orch, orchAgents]) => {
        const isCollapsed = collapsedSections.has(orch)
        return (
          <div key={orch} className={`${styles.section} ${isCollapsed ? styles.collapsed : ''}`}>
            <h3 className={`${styles.sectionTitle} ${isCollapsed ? styles.collapsed : ''}`} onClick={() => toggleSection(orch)}>
              {orchIcon(orch)} {orch}
              <span className={styles.sectionCount}>{orchAgents.length}</span>
            </h3>
            <div className={styles.grid}>
              {orchAgents.map(a => (
                <div key={`${a.name}:${a.orchestrator}`} className={`${styles.card} ${a.enabled ? styles.cardEnabled : styles.cardDisabled}`}>
                  {editing === editKey(a) ? (
                    <div className={styles.editForm}>
                      <div className={styles.editName}>{a.name}</div>
                      <input
                        className={styles.input}
                        placeholder="Description"
                        value={editDesc}
                        onChange={e => setEditDesc(e.target.value)}
                        onKeyDown={e => e.key === 'Enter' && saveEdit(a)}
                      />
                      <div className={styles.editActions}>
                        <button className={styles.saveBtn} onClick={() => saveEdit(a)}>Save</button>
                        <button className={styles.cancelBtn} onClick={() => setEditing(null)}>Cancel</button>
                      </div>
                    </div>
                  ) : (
                    <>
                      <div className={styles.cardHeader}>
                        <span className={styles.cardName}>{a.name}</span>
                        <button
                          className={`${styles.enabledBadge} ${a.enabled ? styles.badgeOn : styles.badgeOff}`}
                          onClick={() => toggleEnabled(a)}
                          title={a.enabled ? 'Click to disable' : 'Click to enable'}
                        >
                          {a.enabled ? 'ON' : 'OFF'}
                        </button>
                      </div>
                      {a.description && <p className={styles.cardDesc}>{a.description}</p>}
                      <div className={styles.cardActions}>
                        <button className={styles.editBtn} onClick={() => startEdit(a)} title="Edit">✏️</button>
                        <button className={styles.deleteBtn} onClick={() => deleteAgent(a.name, a.orchestrator)} title="Remove">🗑️</button>
                      </div>
                    </>
                  )}
                </div>
              ))}
            </div>
          </div>
        )
      })}

      {agents.length === 0 && (
        <div className={styles.empty}>
          <p>No agents registered. Click "Add Agent" to get started.</p>
        </div>
      )}
    </div>
  )
}
