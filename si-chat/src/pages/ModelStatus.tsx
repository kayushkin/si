import { useEffect, useState } from 'react'
import styles from './ModelStatus.module.css'

// --- Model types ---

interface ModelHealth {
  model: string
  last_success_at: string
  last_success_ms: number
  avg_response_ms: number
  last_error_at: string
  last_error: string
  consecutive_errors: number
}

interface CredentialRef {
  id: string
  label: string
  auth_type: string
  api_key_masked?: string
  token_masked?: string
  priority: number
  enabled: boolean
  last_used_at: number
  error_count: number
  expires_at: number
}

interface ModelStatusEntry {
  id: string
  provider: string
  name: string
  max_tokens: number
  input_cost: number
  output_cost: number
  enabled: boolean
  priority: number
  health: ModelHealth | null
  credentials: CredentialRef[]
}

// --- Credential types ---

interface Credential {
  id: string
  provider: string
  label: string
  auth_type: string
  api_key_masked?: string
  token_masked?: string
  base_url?: string
  priority: number
  enabled: boolean
  last_used_at: number
  last_error: string
  last_error_at: number
  error_count: number
  expires_at: number
  created_at: string
}

// --- Helpers ---

function timeAgo(dateStr: string): string {
  if (!dateStr || dateStr === '0001-01-01T00:00:00Z') return 'never'
  const d = new Date(dateStr)
  if (d.getTime() === 0) return 'never'
  const seconds = Math.floor((Date.now() - d.getTime()) / 1000)
  if (seconds < 60) return `${seconds}s ago`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`
  return `${Math.floor(seconds / 86400)}d ago`
}

function timeAgoMs(unixMs: number): string {
  if (!unixMs || unixMs === 0) return 'never'
  const seconds = Math.floor((Date.now() - unixMs) / 1000)
  if (seconds < 0) return 'future?'
  if (seconds < 60) return `${seconds}s ago`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`
  return `${Math.floor(seconds / 86400)}d ago`
}

function healthStatus(h: ModelHealth | null): { icon: string; label: string; className: string } {
  if (!h || (!h.last_success_at && !h.last_error_at) ||
      (h.last_success_at === '0001-01-01T00:00:00Z' && h.last_error_at === '0001-01-01T00:00:00Z')) {
    return { icon: '❓', label: 'Unknown', className: styles.unknown }
  }

  const successTime = new Date(h.last_success_at).getTime()
  const errorTime = new Date(h.last_error_at).getTime()
  const thirtyMin = 30 * 60 * 1000
  const now = Date.now()

  if (successTime > 0 && (now - successTime) < thirtyMin && successTime > errorTime) {
    return { icon: '🟢', label: 'Healthy', className: styles.healthy }
  }
  if (errorTime > successTime && errorTime > 0) {
    return { icon: '🔴', label: 'Error', className: styles.error }
  }
  if (successTime > 0) {
    return { icon: '🟡', label: 'Stale', className: styles.stale }
  }
  return { icon: '❓', label: 'Unknown', className: styles.unknown }
}

function authTypeIcon(authType: string): string {
  switch (authType) {
    case 'oauth': return '🔒'
    case 'api_key': return '🔑'
    case 'token': return '🎫'
    default: return '❓'
  }
}

function credStatus(c: Credential): { icon: string; className: string } {
  if (c.error_count > 0) return { icon: '🔴', className: styles.error }
  if (!c.last_used_at || c.last_used_at === 0) return { icon: '⚪', className: '' }
  const thirtyMinAgo = Date.now() - 30 * 60 * 1000
  if (c.last_used_at > thirtyMinAgo) return { icon: '🟢', className: styles.healthy }
  return { icon: '🟡', className: styles.stale }
}

function expiresIn(unixMs: number): string | null {
  if (!unixMs || unixMs === 0) return null
  const seconds = Math.floor((unixMs - Date.now()) / 1000)
  if (seconds < 0) return 'expired'
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m left`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h left`
  return `${Math.floor(seconds / 86400)}d left`
}

// --- Component ---

export default function ModelStatus() {
  const [models, setModels] = useState<ModelStatusEntry[]>([])
  const [credentials, setCredentials] = useState<Credential[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [testing, setTesting] = useState<Record<string, boolean>>({})
  const [testResults, setTestResults] = useState<Record<string, { status: string; duration_ms: number; error?: string }>>({})

  const fetchModels = async () => {
    try {
      const res = await fetch('/api/models')
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      const data = await res.json()
      setModels(data || [])
      setError(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to fetch')
    } finally {
      setLoading(false)
    }
  }

  const fetchCredentials = async () => {
    try {
      const res = await fetch('/api/credentials')
      if (!res.ok) return
      const data = await res.json()
      setCredentials(data || [])
    } catch {
      // credentials section is optional — don't block the page
    }
  }

  const testModel = async (modelId: string) => {
    setTesting(prev => ({ ...prev, [modelId]: true }))
    setTestResults(prev => { const n = { ...prev }; delete n[modelId]; return n })
    try {
      const res = await fetch('/api/models/test', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ model: modelId }),
      })
      const data = await res.json()
      setTestResults(prev => ({ ...prev, [modelId]: data }))
      fetchModels()
    } catch {
      setTestResults(prev => ({ ...prev, [modelId]: { status: 'error', duration_ms: 0, error: 'Request failed' } }))
    } finally {
      setTesting(prev => ({ ...prev, [modelId]: false }))
    }
  }

  const toggleModel = async (modelId: string, enabled: boolean) => {
    try {
      await fetch('/api/models/toggle', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ model: modelId, enabled }),
      })
      fetchModels()
    } catch { /* ignore */ }
  }

  const toggleCredential = async (id: string, enabled: boolean) => {
    try {
      await fetch('/api/credentials/toggle', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id, enabled }),
      })
      fetchCredentials()
    } catch { /* ignore */ }
  }

  const testAllEnabled = async () => {
    const enabled = models.filter(m => m.enabled)
    for (const m of enabled) {
      await testModel(m.id)
    }
  }

  useEffect(() => {
    fetchModels()
    fetchCredentials()
    const interval = setInterval(() => { fetchModels(); fetchCredentials() }, 30000)
    return () => clearInterval(interval)
  }, [])

  if (loading) return <div className={styles.container}><p>Loading...</p></div>
  if (error) return <div className={styles.container}><p className={styles.errorMsg}>Error: {error}</p></div>

  const anyTesting = Object.values(testing).some(v => v)

  // Group credentials by provider
  const credGroups = credentials.reduce((acc, c) => {
    if (!acc[c.provider]) acc[c.provider] = []
    acc[c.provider].push(c)
    return acc
  }, {} as Record<string, Credential[]>)

  return (
    <div className={styles.container}>
      {/* --- Credentials Section --- */}
      {credentials.length > 0 && (
        <>
          <div className={styles.header}>
            <h2 className={styles.title}>🔑 Credentials</h2>
          </div>

          {Object.entries(credGroups).map(([provider, creds]) => (
            <div key={provider} className={styles.credGroup}>
              <h3 className={styles.credProvider}>{provider}</h3>
              <div className={styles.table}>
                <div className={styles.headerRow}>
                  <span className={styles.colStatus}>Status</span>
                  <span className={styles.colName}>Credential</span>
                  <span className={styles.colProvider}>Type</span>
                  <span className={styles.colPriority}>Pri</span>
                  <span className={styles.colLast}>Last Used</span>
                  <span className={styles.colError}>Errors</span>
                </div>

                {creds.map(c => {
                  const status = credStatus(c)
                  const expiry = expiresIn(c.expires_at)
                  const keyPreview = c.api_key_masked || c.token_masked || ''

                  return (
                    <div key={c.id} className={`${styles.row} ${!c.enabled ? styles.disabled : ''} ${status.className}`}>
                      <span className={styles.colStatus}>
                        <span className={styles.statusIcon}>{status.icon}</span>
                        <button
                          className={`${styles.enabledBadge} ${c.enabled ? styles.badgeOn : styles.badgeOff}`}
                          onClick={() => toggleCredential(c.id, !c.enabled)}
                          title={c.enabled ? 'Click to disable' : 'Click to enable'}
                        >
                          {c.enabled ? 'ON' : 'OFF'}
                        </button>
                      </span>
                      <span className={styles.colName}>
                        <strong>{c.id}</strong>
                        <span className={styles.modelId}>{c.label}</span>
                        {keyPreview && <span className={styles.modelId}>{keyPreview}</span>}
                        {expiry && (
                          <span className={expiry === 'expired' ? styles.testErr : styles.testOk}>
                            {expiry}
                          </span>
                        )}
                      </span>
                      <span className={styles.colProvider}>
                        {authTypeIcon(c.auth_type)} {c.auth_type}
                      </span>
                      <span className={styles.colPriority}>{c.priority}</span>
                      <span className={styles.colLast}>{timeAgoMs(c.last_used_at)}</span>
                      <span className={styles.colError}>
                        {c.error_count > 0
                          ? <span className={styles.errorText} title={c.last_error}>{c.error_count}× — {c.last_error?.substring(0, 30)}</span>
                          : '—'}
                      </span>
                    </div>
                  )
                })}
              </div>
            </div>
          ))}

          <div className={styles.divider} />
        </>
      )}

      {/* --- Models Section --- */}
      <div className={styles.header}>
        <h2 className={styles.title}>🤖 Models</h2>
        <button
          className={styles.testAllBtn}
          onClick={testAllEnabled}
          disabled={anyTesting}
        >
          {anyTesting ? '⏳ Testing...' : '🧪 Test All Enabled'}
        </button>
      </div>

      <div className={styles.table}>
        <div className={styles.headerRow}>
          <span className={styles.colStatus}>Status</span>
          <span className={styles.colName}>Model</span>
          <span className={styles.colProvider}>Provider</span>
          <span className={styles.colPriority}>Pri</span>
          <span className={styles.colCost}>Cost (in/out)</span>
          <span className={styles.colResponse}>Avg Response</span>
          <span className={styles.colLast}>Last Success</span>
          <span className={styles.colError}>Last Error</span>
          <span className={styles.colAction}>Test</span>
        </div>

        {models.map(m => {
          const hs = healthStatus(m.health)
          const isTesting = testing[m.id]
          const result = testResults[m.id]
          return (
            <div key={m.id} className={styles.modelGroup}>
              <div className={`${styles.row} ${!m.enabled ? styles.disabled : ''} ${hs.className}`}>
                <span className={styles.colStatus}>
                  <span className={styles.statusIcon}>{isTesting ? '⏳' : hs.icon}</span>
                  <button
                    className={`${styles.enabledBadge} ${m.enabled ? styles.badgeOn : styles.badgeOff}`}
                    onClick={() => toggleModel(m.id, !m.enabled)}
                    title={m.enabled ? 'Click to disable' : 'Click to enable'}
                  >
                    {m.enabled ? 'ON' : 'OFF'}
                  </button>
                </span>
                <span className={styles.colName}>
                  <strong>{m.name}</strong>
                  <span className={styles.modelId}>{m.id}</span>
                  {result && (
                    <span className={result.status === 'ok' ? styles.testOk : styles.testErr}>
                      {result.status === 'ok'
                        ? `✓ ${(result.duration_ms / 1000).toFixed(1)}s`
                        : `✗ ${result.error?.substring(0, 40)}`}
                    </span>
                  )}
                </span>
                <span className={styles.colProvider}>{m.provider}</span>
                <span className={styles.colPriority}>{m.priority}</span>
                <span className={styles.colCost}>${m.input_cost} / ${m.output_cost}</span>
                <span className={styles.colResponse}>
                  {m.health && m.health.avg_response_ms > 0
                    ? `${(m.health.avg_response_ms / 1000).toFixed(1)}s`
                    : '—'}
                </span>
                <span className={styles.colLast}>
                  {m.health ? timeAgo(m.health.last_success_at) : '—'}
                  {m.health && m.health.last_success_ms > 0 &&
                    <span className={styles.lastMs}> ({(m.health.last_success_ms / 1000).toFixed(1)}s)</span>
                  }
                </span>
                <span className={styles.colError}>
                  {m.health && m.health.last_error
                    ? <span className={styles.errorText} title={m.health.last_error}>
                        {m.health.last_error.substring(0, 30)}
                        {m.health.consecutive_errors > 1 && ` (×${m.health.consecutive_errors})`}
                      </span>
                    : '—'}
                </span>
                <span className={styles.colAction}>
                  <button
                    className={styles.testBtn}
                    onClick={() => testModel(m.id)}
                    disabled={isTesting}
                    title={`Test ${m.name}`}
                  >
                    {isTesting ? '⏳' : '🧪'}
                  </button>
                </span>
              </div>

              {/* Credential sub-rows */}
              {m.credentials && m.credentials.length > 0 && (
                <div className={styles.credSubRows}>
                  {m.credentials.map(c => {
                    const exp = expiresIn(c.expires_at)
                    return (
                      <div key={c.id} className={`${styles.credSubRow} ${!c.enabled ? styles.disabled : ''}`}>
                        <span className={styles.credSubIcon}>
                          {c.error_count > 0 ? '🔴' : c.enabled ? '↳' : '⏸'}
                        </span>
                        <span className={styles.credSubAuth}>
                          {authTypeIcon(c.auth_type)}
                        </span>
                        <span className={styles.credSubLabel}>
                          {c.label || c.id}
                          {c.api_key_masked && <span className={styles.modelId}> {c.api_key_masked}</span>}
                          {c.token_masked && <span className={styles.modelId}> {c.token_masked}</span>}
                        </span>
                        <span className={styles.credSubType}>{c.auth_type}</span>
                        <span className={styles.credSubPri}>pri {c.priority}</span>
                        {exp && (
                          <span className={exp === 'expired' ? styles.testErr : styles.credSubExpiry}>
                            {exp}
                          </span>
                        )}
                        {c.error_count > 0 && (
                          <span className={styles.credSubErr}>{c.error_count}× err</span>
                        )}
                        <span className={styles.credSubUsed}>{timeAgoMs(c.last_used_at)}</span>
                      </div>
                    )
                  })}
                </div>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}
