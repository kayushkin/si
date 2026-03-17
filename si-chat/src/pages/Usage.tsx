import { useEffect, useState, useCallback } from 'react'
import styles from './Usage.module.css'

interface UsageStats {
  agent: string
  orchestrator: string
  model?: string
  messages: number
  input_tokens: number
  output_tokens: number
  total_tokens: number
  duration_ms: number
}

interface UsageResponse {
  day: UsageStats[]
  week: UsageStats[]
  month: UsageStats[]
}

type Period = 'day' | 'week' | 'month'

interface OrchestratorGroup {
  name: string
  label: string
  icon: string
  agents: UsageStats[]
  totals: { messages: number; input_tokens: number; output_tokens: number; total_tokens: number; duration_ms: number }
}

function formatTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return n.toString()
}

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  const s = ms / 1000
  if (s < 60) return `${s.toFixed(0)}s`
  const m = s / 60
  if (m < 60) return `${m.toFixed(1)}m`
  const h = m / 60
  return `${h.toFixed(1)}h`
}

const orchestratorMeta: Record<string, { label: string; icon: string; order: number }> = {
  openclaw:  { label: 'OpenClaw',  icon: '🦞', order: 0 },
  inber:     { label: 'Inber',     icon: '🌿', order: 1 },
}

function getOrchMeta(name: string) {
  return orchestratorMeta[name] || { label: name || 'Unknown', icon: '⚙️', order: 99 }
}

function groupByOrchestrator(stats: UsageStats[]): OrchestratorGroup[] {
  const groups = new Map<string, OrchestratorGroup>()

  for (const s of stats) {
    const key = s.orchestrator || '_unknown'
    let group = groups.get(key)
    if (!group) {
      const meta = getOrchMeta(key)
      group = {
        name: key,
        label: meta.label,
        icon: meta.icon,
        agents: [],
        totals: { messages: 0, input_tokens: 0, output_tokens: 0, total_tokens: 0, duration_ms: 0 },
      }
      groups.set(key, group)
    }
    group.agents.push(s)
    group.totals.messages += s.messages
    group.totals.input_tokens += s.input_tokens
    group.totals.output_tokens += s.output_tokens
    group.totals.total_tokens += s.total_tokens
    group.totals.duration_ms += s.duration_ms
  }

  return Array.from(groups.values())
    .sort((a, b) => getOrchMeta(a.name).order - getOrchMeta(b.name).order)
}

function TokenBar({ input, output, total }: { input: number; output: number; total: number }) {
  if (total === 0) return null
  const inPct = (input / total) * 100
  const outPct = (output / total) * 100
  return (
    <div className={styles.tokenBar}>
      <div className={styles.tokenBarIn} style={{ width: `${inPct}%` }} title={`Input: ${formatTokens(input)}`} />
      <div className={styles.tokenBarOut} style={{ width: `${outPct}%` }} title={`Output: ${formatTokens(output)}`} />
    </div>
  )
}

export default function Usage() {
  const [data, setData] = useState<UsageResponse | null>(null)
  const [period, setPeriod] = useState<Period>('day')
  const [loading, setLoading] = useState(true)

  const fetchUsage = useCallback(async () => {
    try {
      const resp = await fetch('/api/usage')
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
      const json = await resp.json()
      setData(json)
    } catch (err) {
      console.error('Failed to fetch usage:', err)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchUsage()
    const interval = setInterval(fetchUsage, 30000)
    return () => clearInterval(interval)
  }, [fetchUsage])

  const stats = data?.[period] || []
  const groups = groupByOrchestrator(stats)

  // Global totals
  const totals = stats.reduce(
    (acc, s) => ({
      messages: acc.messages + s.messages,
      input_tokens: acc.input_tokens + s.input_tokens,
      output_tokens: acc.output_tokens + s.output_tokens,
      total_tokens: acc.total_tokens + s.total_tokens,
      duration_ms: acc.duration_ms + s.duration_ms,
    }),
    { messages: 0, input_tokens: 0, output_tokens: 0, total_tokens: 0, duration_ms: 0 }
  )

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <h2>Token Usage</h2>
        <div className={styles.periodTabs}>
          {(['day', 'week', 'month'] as Period[]).map(p => (
            <button
              key={p}
              className={`${styles.tab} ${period === p ? styles.active : ''}`}
              onClick={() => setPeriod(p)}
            >
              {p === 'day' ? '24h' : p === 'week' ? '7d' : '30d'}
            </button>
          ))}
        </div>
      </div>

      {loading ? (
        <div className={styles.loading}>Loading...</div>
      ) : stats.length === 0 ? (
        <div className={styles.empty}>No usage data for this period</div>
      ) : (
        <>
          {/* Global summary */}
          <div className={styles.totalsRow}>
            <div className={styles.totalCard}>
              <span className={styles.totalLabel}>Messages</span>
              <span className={styles.totalValue}>{totals.messages}</span>
            </div>
            <div className={styles.totalCard}>
              <span className={styles.totalLabel}>Input</span>
              <span className={styles.totalValue}>{formatTokens(totals.input_tokens)}</span>
            </div>
            <div className={styles.totalCard}>
              <span className={styles.totalLabel}>Output</span>
              <span className={styles.totalValue}>{formatTokens(totals.output_tokens)}</span>
            </div>
            <div className={styles.totalCard}>
              <span className={styles.totalLabel}>Total</span>
              <span className={styles.totalValue}>{formatTokens(totals.total_tokens)}</span>
            </div>
            <div className={styles.totalCard}>
              <span className={styles.totalLabel}>Time</span>
              <span className={styles.totalValue}>{formatDuration(totals.duration_ms)}</span>
            </div>
          </div>

          {/* Per-orchestrator sections */}
          {groups.map(group => (
            <div key={group.name} className={styles.orchSection}>
              <div className={styles.orchHeader}>
                <div className={styles.orchTitle}>
                  <span className={styles.orchIcon}>{group.icon}</span>
                  <span className={styles.orchName}>{group.label}</span>
                </div>
                <div className={styles.orchSummary}>
                  <span>{group.totals.messages} msgs</span>
                  <span className={styles.orchTokens}>{formatTokens(group.totals.total_tokens)} tokens</span>
                  <span>{formatDuration(group.totals.duration_ms)}</span>
                </div>
              </div>
              <TokenBar input={group.totals.input_tokens} output={group.totals.output_tokens} total={group.totals.total_tokens} />
              {group.agents.length > 1 && (
                <div className={styles.agentList}>
                  {group.agents
                    .sort((a, b) => b.total_tokens - a.total_tokens)
                    .map((s, i) => (
                    <div key={i} className={styles.agentRow}>
                      <span className={styles.agentName}>{s.agent}</span>
                      <span className={styles.agentModel}>{s.model}</span>
                      <span>{s.messages} msgs</span>
                      <span>{formatTokens(s.input_tokens)} in</span>
                      <span>{formatTokens(s.output_tokens)} out</span>
                      <span className={styles.bold}>{formatTokens(s.total_tokens)}</span>
                      <span>{formatDuration(s.duration_ms)}</span>
                    </div>
                  ))}
                </div>
              )}
              {group.agents.length === 1 && (
                <div className={styles.agentSingle}>
                  <span className={styles.agentModel}>{group.agents[0].model}</span>
                  <span>{formatTokens(group.totals.input_tokens)} in · {formatTokens(group.totals.output_tokens)} out</span>
                </div>
              )}
            </div>
          ))}
        </>
      )}
    </div>
  )
}
