import { useEffect, useState, useCallback } from 'react'
import styles from './Forge.module.css'

const SERVICE_REPOS = new Set(['bus', 'si', 'kayushkin.com', 'logstack', 'inber'])

interface EnvironmentRepo {
  name: string
  worktree_path: string
  branch: string
  commit: string
  commit_message: string
  dirty: boolean
  dirty_files: number
  ahead: number
  behind: number
}

interface EnvironmentService {
  name: string
  status: string
  health: string
  ports: string
}

interface Environment {
  id: number
  name: string
  base_port: number
  status: string
  change: string
  agents: string[]
  services: EnvironmentService[]
  repos: EnvironmentRepo[]
}

interface Deploy {
  id: number
  slot: number
  agent: string
  action: string
  detail: string
  timestamp: number
}

function deriveEnvStatus(env: Environment): { label: string; className: string } {
  const hasRunning = env.services?.some(s => s.status === 'running')
  if (hasRunning) return { label: 'running', className: 'running' }
  if (env.status === 'active') return { label: 'claimed', className: 'active' }
  return { label: 'idle', className: 'idle' }
}

export default function Forge() {
  const [environments, setEnvironments] = useState<Environment[]>([])
  const [deploys, setDeploys] = useState<Deploy[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [lastRefresh, setLastRefresh] = useState<Date | null>(null)
  const [expandedRepos, setExpandedRepos] = useState<Set<number>>(new Set())
  const [deploying, setDeploying] = useState<Set<number>>(new Set())

  const fetchData = useCallback(async () => {
    try {
      const [envRes, deploysRes] = await Promise.all([
        fetch('/api/forge/environments'),
        fetch('/api/forge/deploys'),
      ])
      if (!envRes.ok) throw new Error(`Forge: HTTP ${envRes.status}`)
      const envData = await envRes.json()
      setEnvironments(envData || [])

      if (deploysRes.ok) {
        const deploysData = await deploysRes.json()
        setDeploys(deploysData || [])
      }

      setError(null)
      setLastRefresh(new Date())
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to fetch')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchData()
    const interval = setInterval(fetchData, 5000)
    return () => clearInterval(interval)
  }, [fetchData])

  const toggleRepos = (envId: number) => {
    setExpandedRepos(prev => {
      const next = new Set(prev)
      if (next.has(envId)) next.delete(envId)
      else next.add(envId)
      return next
    })
  }

  const triggerDeploy = async (envId: number) => {
    setDeploying(prev => new Set(prev).add(envId))
    try {
      const res = await fetch('/api/forge/deploy', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ environment_id: envId, target: 'dev' }),
      })
      if (!res.ok) {
        const data = await res.json()
        alert(data.error || 'Deploy failed')
      }
    } catch (e) {
      alert(e instanceof Error ? e.message : 'Deploy failed')
    } finally {
      setTimeout(() => {
        setDeploying(prev => {
          const next = new Set(prev)
          next.delete(envId)
          return next
        })
      }, 2000)
    }
  }

  const formatTime = (ts: number) => {
    const date = new Date(ts * 1000)
    const now = new Date()
    const diff = now.getTime() - date.getTime()
    if (diff < 60000) return 'just now'
    if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`
    if (diff < 86400000) return `${Math.floor(diff / 3600000)}h ago`
    return date.toLocaleDateString()
  }

  const getChangedRepos = (env: Environment) =>
    env.repos.filter(r => r.branch !== 'main' || r.dirty || r.ahead > 0)

  if (loading) {
    return <div className={styles.loading}>Loading environments...</div>
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <h1 className={styles.title}>Forge Environments</h1>
        <div className={styles.statusBar}>
          <span className={styles.lastRefresh}>
            {lastRefresh ? `Last refresh: ${lastRefresh.toLocaleTimeString()}` : ''}
          </span>
          {error && <span className={styles.error}>{error}</span>}
        </div>
      </div>

      {/* Summary */}
      <div className={styles.summaryBar}>
        <div className={styles.summaryItem}>
          <span className={styles.summaryLabel}>Environments</span>
          <span className={styles.summaryValue}>{environments.length}</span>
        </div>
        <div className={styles.summaryItem}>
          <span className={styles.summaryLabel}>Running</span>
          <span className={styles.summaryValue}>
            {environments.filter(e => e.services?.some(s => s.status === 'running')).length}
          </span>
        </div>
        <div className={styles.summaryItem}>
          <span className={styles.summaryLabel}>With Changes</span>
          <span className={styles.summaryValue}>
            {environments.filter(e => getChangedRepos(e).length > 0).length}
          </span>
        </div>
      </div>

      {/* Environment Cards */}
      <div className={styles.environments}>
        {environments.map(env => {
          const changedRepos = getChangedRepos(env)
          const isExpanded = expandedRepos.has(env.id)
          const isDeploying = deploying.has(env.id)
          const derived = deriveEnvStatus(env)
          const envNum = env.name.replace(/\D/g, '')
          const devUrl = envNum ? `https://${envNum}.dev.kayushkin.com` : null
          const serviceRepos = env.repos.filter(r => SERVICE_REPOS.has(r.name))
          const depRepos = env.repos.filter(r => !SERVICE_REPOS.has(r.name))

          return (
            <div key={env.id} className={`${styles.envCard} ${styles[derived.className] || ''}`}>
              {/* Header */}
              <div className={styles.envHeader}>
                <div className={styles.envTitle}>
                  <h2>{env.name}</h2>
                  <span className={styles.portBadge}>:{env.base_port}</span>
                  {devUrl && (
                    <a href={devUrl} target="_blank" rel="noopener noreferrer" className={styles.devLink}>
                      {envNum}.dev ↗
                    </a>
                  )}
                </div>
                <div className={styles.envStatus}>
                  <span className={`${styles.statusBadge} ${styles[derived.className] || ''}`}>
                    {derived.label}
                  </span>
                  {env.change && (
                    <span className={styles.changeName}>{env.change}</span>
                  )}
                  {env.agents?.length > 0 && (
                    <span className={styles.agentBadge} title={env.agents.join(', ')}>
                      {env.agents.length} agent{env.agents.length > 1 ? 's' : ''}
                    </span>
                  )}
                </div>
              </div>

              {/* Services row */}
              {env.services?.length > 0 && (
                <div className={styles.servicesList}>
                  {env.services.map(svc => (
                    <span
                      key={svc.name}
                      className={`${styles.serviceBadge} ${svc.status === 'running' ? styles.serviceRunning : styles.serviceStopped}`}
                      title={`${svc.name}: ${svc.status}${svc.health !== 'none' ? ` (${svc.health})` : ''}`}
                    >
                      <span className={styles.serviceDot} />
                      {svc.name}
                    </span>
                  ))}
                </div>
              )}

              {/* Changes section — always visible */}
              <div className={styles.changesSection}>
                <h3 className={styles.changesSectionTitle}>Changes</h3>
                {changedRepos.length === 0 ? (
                  <div className={styles.noChanges}>All repos on main — no changes</div>
                ) : (
                  <div className={styles.changedRepoList}>
                    {changedRepos.map(repo => (
                      <div key={repo.name} className={styles.changedRepo}>
                        <span className={styles.changedRepoName}>{repo.name}</span>
                        <code className={`${styles.changedBranch} ${repo.branch !== 'main' ? styles.branchHighlight : ''}`}>
                          {repo.branch}
                        </code>
                        {repo.dirty && (
                          <span className={styles.dirtyBadge}>{repo.dirty_files} dirty</span>
                        )}
                        {repo.behind > 0 && (
                          <span className={styles.behindBadge}>↓{repo.behind} behind</span>
                        )}
                        {repo.ahead > 0 && (
                          <span className={styles.aheadBadge}>↑{repo.ahead}</span>
                        )}
                        <span className={styles.changedCommitMsg}>{repo.commit_message}</span>
                      </div>
                    ))}
                  </div>
                )}
              </div>

              {/* Actions */}
              <div className={styles.envActions}>
                {changedRepos.length > 0 ? (
                  <button
                    onClick={() => triggerDeploy(env.id)}
                    disabled={isDeploying}
                    className={`${styles.deployButton} ${styles.dev}`}
                  >
                    {isDeploying ? '🚀 Deploying...' : '🔧 Deploy to Dev'}
                  </button>
                ) : (
                  <button disabled className={`${styles.deployButton} ${styles.disabled}`}>
                    No changes
                  </button>
                )}
                <button onClick={() => toggleRepos(env.id)} className={styles.expandButton}>
                  {isExpanded ? '▾ Hide All Repos' : '▸ All Repos'}
                </button>
              </div>

              {/* Expanded: full repo list */}
              {isExpanded && (
                <div className={styles.repoList}>
                  {/* Services */}
                  <h4 className={styles.repoGroupTitle}>Services</h4>
                  {serviceRepos.map(repo => (
                    <RepoRow key={repo.name} repo={repo} />
                  ))}
                  {/* Dependencies */}
                  <h4 className={`${styles.repoGroupTitle} ${styles.depGroupTitle}`}>Dependencies</h4>
                  {depRepos.map(repo => (
                    <RepoRow key={repo.name} repo={repo} muted />
                  ))}
                </div>
              )}
            </div>
          )
        })}
      </div>

      {/* Deploy History */}
      <div className={styles.deploysSection}>
        <h2 className={styles.sectionTitle}>Deploy History</h2>
        {deploys.length === 0 ? (
          <p className={styles.emptyMessage}>No deploys yet</p>
        ) : (
          <div className={styles.deploysList}>
            {deploys.slice(0, 15).map(deploy => (
              <div key={deploy.id} className={styles.deployItem}>
                <div className={styles.deployHeader}>
                  <span className={styles.deployTarget}>slot-{deploy.slot}</span>
                  <span className={styles.deployStatus}>{deploy.action}</span>
                </div>
                <div className={styles.deployInfo}>
                  <span className={styles.deployProject}>{deploy.agent}</span>
                  {deploy.detail && <span className={styles.deployMessage}>{deploy.detail}</span>}
                  <span className={styles.deployTime}>{formatTime(deploy.timestamp)}</span>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

function RepoRow({ repo, muted }: { repo: EnvironmentRepo; muted?: boolean }) {
  const isOnMain = repo.branch === 'main'
  const isClean = isOnMain && !repo.dirty && repo.behind === 0
  return (
    <div className={`${styles.repoItem} ${isClean ? styles.repoClean : ''} ${muted ? styles.repoMuted : ''} ${repo.dirty ? styles.repoDirty : ''} ${repo.behind > 0 ? styles.repoBehind : ''}`}>
      <div className={styles.repoHeader}>
        <span className={styles.repoName}>{repo.name}</span>
        <div className={styles.repoBadges}>
          <code className={`${styles.branchName} ${!isOnMain ? styles.branchHighlight : ''}`}>
            {repo.branch}
          </code>
          {repo.dirty && (
            <span className={styles.dirtyBadge} title={`${repo.dirty_files} uncommitted files`}>
              {repo.dirty_files} dirty
            </span>
          )}
          {repo.ahead > 0 && <span className={styles.aheadBadge}>↑{repo.ahead}</span>}
          {repo.behind > 0 && <span className={styles.behindBadge}>↓{repo.behind}</span>}
        </div>
      </div>
      <div className={styles.repoCommit}>
        <code>{repo.commit?.slice(0, 7)}</code>
        <span className={styles.commitMessage}>{repo.commit_message}</span>
      </div>
    </div>
  )
}
