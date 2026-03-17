import { useState, useEffect, useCallback } from 'react'
import styles from './Topology.module.css'

interface TopoNode {
  id: string
  name: string
  type: 'service' | 'external'
  port?: number
  status?: 'running' | 'stopped'
  url?: string
}

interface TopoLink {
  from: string
  to: string
  env_var?: string
  value?: string
  proto?: string
}

interface TopoEnv {
  name: string
  port_base?: number
  nodes: TopoNode[]
  links: TopoLink[]
}

interface TopoData {
  prod: TopoEnv
  staging: TopoEnv[]
  external: TopoNode[]
}

interface RouteInfo {
  env: string
  service: string
  env_var: string
  value: string
  proto: string
  target: string
  options: string[] // "label|value" pairs
}

function classifyNodes(nodes: TopoNode[]) {
  const external: TopoNode[] = []
  const app: TopoNode[] = []
  const infra: TopoNode[] = []
  const infraNames = new Set(['bus', 'logstack', 'bus-agent', 'inber'])

  for (const n of nodes) {
    if (n.type === 'external') external.push(n)
    else if (infraNames.has(n.name)) infra.push(n)
    else app.push(n)
  }
  return { external, app, infra }
}

function NodeCard({ node }: { node: TopoNode }) {
  const status = node.status || 'unknown'
  return (
    <div className={styles.nodeCard}>
      <div className={styles.nodeTop}>
        <span className={`${styles.statusDot} ${styles[status]}`} />
        <span className={styles.nodeName}>{node.name}</span>
      </div>
      <div className={styles.nodeDetails}>
        {node.port && <span className={styles.nodePort}>:{node.port}</span>}
        <span className={`${styles.typeBadge} ${styles[node.type]}`}>{node.type}</span>
      </div>
    </div>
  )
}

function EnvView({ env, nodeMap, routes, onReroute }: {
  env: TopoEnv
  nodeMap: Map<string, TopoNode>
  routes: RouteInfo[]
  onReroute: (env: string, service: string, envVar: string, value: string) => void
}) {
  const { external, app, infra } = classifyNodes(env.nodes)

  return (
    <>
      <div className={styles.nodeGrid}>
        {external.length > 0 && (
          <div>
            <div className={styles.rowLabel}>External</div>
            <div className={styles.row}>
              {external.map(n => <NodeCard key={n.id} node={n} />)}
            </div>
          </div>
        )}
        {app.length > 0 && (
          <div>
            <div className={styles.rowLabel}>Application</div>
            <div className={styles.row}>
              {app.map(n => <NodeCard key={n.id} node={n} />)}
            </div>
          </div>
        )}
        {infra.length > 0 && (
          <div>
            <div className={styles.rowLabel}>Infrastructure</div>
            <div className={styles.row}>
              {infra.map(n => <NodeCard key={n.id} node={n} />)}
            </div>
          </div>
        )}
      </div>

      {env.links.length > 0 && (
        <div className={styles.connectionsSection}>
          <h3 className={styles.sectionTitle}>Connections</h3>
          <table className={styles.connTable}>
            <thead>
              <tr>
                <th>From → To</th>
                <th>Env Var</th>
                <th>Value / Route</th>
                <th>Protocol</th>
              </tr>
            </thead>
            <tbody>
              {env.links.map((link, i) => {
                const fromNode = nodeMap.get(link.from)
                const toNode = nodeMap.get(link.to)
                const proto = link.proto || 'http'
                const envName = env.name === 'prod' ? 'prod' : env.name
                const fromSvc = fromNode?.name || link.from.replace(/^(prod|env\d)-/, '')
                const route = routes.find(
                  r => r.env === envName && r.service === fromSvc && r.env_var === link.env_var
                )
                return (
                  <tr key={i}>
                    <td>
                      {fromNode?.name || link.from}
                      <span className={styles.arrow}>→</span>
                      {toNode?.name || link.to}
                    </td>
                    <td><span className={styles.envVar}>{link.env_var || '—'}</span></td>
                    <td>
                      {route && route.options && route.options.length > 0 ? (
                        <select
                          className={styles.routeSelect}
                          value={route.value}
                          onChange={e => onReroute(envName, route.service, route.env_var, e.target.value)}
                        >
                          {route.options.map(opt => {
                            const [label, val] = opt.split('|')
                            const isActive = val === route.value
                            return (
                              <option key={val} value={val}>
                                {isActive ? '● ' : ''}{label}
                              </option>
                            )
                          })}
                          {!route.options.some(o => o.split('|')[1] === route.value) && (
                            <option value={route.value}>● custom: {route.value}</option>
                          )}
                        </select>
                      ) : (
                        <span className={styles.connValue}>{link.value || route?.value || '—'}</span>
                      )}
                    </td>
                    <td><span className={`${styles.protoBadge} ${styles[proto]}`}>{proto}</span></td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
    </>
  )
}

export default function Topology() {
  const [data, setData] = useState<TopoData | null>(null)
  const [routes, setRoutes] = useState<RouteInfo[]>([])
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [activeTab, setActiveTab] = useState('prod')
  const [lastRefresh, setLastRefresh] = useState<Date | null>(null)
  const [rerouting, setRerouting] = useState(false)

  const fetchData = useCallback(async () => {
    try {
      const [topoRes, routesRes] = await Promise.all([
        fetch('/api/forge/topology'),
        fetch('/api/forge/routes'),
      ])
      if (!topoRes.ok) throw new Error(`HTTP ${topoRes.status}`)
      const json = await topoRes.json()
      setData(json)
      if (routesRes.ok) {
        const routesJson = await routesRes.json()
        setRoutes(routesJson || [])
      }
      setError('')
      setLastRefresh(new Date())
    } catch (e: any) {
      setError(e.message)
    } finally {
      setLoading(false)
    }
  }, [])

  const handleReroute = async (env: string, service: string, envVar: string, value: string) => {
    if (!confirm(`Reroute ${service}.${envVar} to ${value}?\nThis will restart the service.`)) return
    setRerouting(true)
    try {
      const res = await fetch('/api/forge/routes', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ env, service, env_var: envVar, value }),
      })
      const result = await res.json()
      if (!res.ok) {
        alert(`Reroute failed: ${result.error}`)
      } else {
        // Refresh data after reroute
        setTimeout(fetchData, 2000)
      }
    } catch (e: any) {
      alert(`Reroute error: ${e.message}`)
    } finally {
      setRerouting(false)
    }
  }

  useEffect(() => {
    fetchData()
    const interval = setInterval(fetchData, 10000)
    return () => clearInterval(interval)
  }, [fetchData])

  if (loading) return <div className={styles.loading}>Loading topology…</div>
  if (error && !data) return <div className={styles.error}>Error: {error}</div>
  if (!data) return null

  const tabs: { key: string; label: string; env: TopoEnv }[] = [
    { key: 'prod', label: 'Prod', env: data.prod },
    ...(data.staging || []).map((s, i) => ({
      key: `staging-${i}`,
      label: s.name.replace('env-', 'Env '),
      env: s,
    })),
  ]

  const currentTab = tabs.find(t => t.key === activeTab) || tabs[0]
  const nodeMap = new Map<string, TopoNode>()
  currentTab.env.nodes.forEach(n => nodeMap.set(n.id, n))

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <h1 className={styles.title}>Topology</h1>
        {lastRefresh && (
          <span className={styles.lastRefresh}>
            Updated {lastRefresh.toLocaleTimeString()}
          </span>
        )}
      </div>

      {error && <div className={styles.error}>{error}</div>}

      <div className={styles.tabs}>
        {tabs.map(t => (
          <button
            key={t.key}
            className={activeTab === t.key ? styles.tabActive : styles.tab}
            onClick={() => setActiveTab(t.key)}
          >
            {t.label}
          </button>
        ))}
      </div>

      {rerouting && <div className={styles.rerouteOverlay}>Rerouting…</div>}
      <EnvView env={currentTab.env} nodeMap={nodeMap} routes={routes} onReroute={handleReroute} />

      {data.external && data.external.length > 0 && (
        <div className={styles.externalsSection}>
          <h3 className={styles.sectionTitle}>External Services</h3>
          <div className={styles.externalList}>
            {data.external.map(ext => (
              <div key={ext.id} className={styles.externalChip}>
                <span>{ext.name}</span>
                {ext.url && <span className={styles.externalUrl}>{ext.url}</span>}
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
