import { useEffect, useState } from 'react'
import styles from './Services.module.css'

interface Service {
  name: string
  host: string
  port: number
  status: string
  latency_ms: number
  detail?: string
}

function statusIcon(s: string) {
  switch (s) {
    case 'up': return '🟢'
    case 'down': return '🔴'
    default: return '❓'
  }
}

function parseDetail(detail?: string): Record<string, unknown> | null {
  if (!detail) return null
  try { return JSON.parse(detail) } catch { return null }
}

export default function Services() {
  const [services, setServices] = useState<Service[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [lastCheck, setLastCheck] = useState<Date | null>(null)

  const fetchServices = async () => {
    try {
      const res = await fetch('/api/services')
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      const data = await res.json()
      setServices(data || [])
      setError(null)
      setLastCheck(new Date())
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to fetch')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchServices()
    const interval = setInterval(fetchServices, 30000)
    return () => clearInterval(interval)
  }, [])

  if (loading) return <div className={styles.container}><p>Loading...</p></div>
  if (error) return <div className={styles.container}><p className={styles.errorMsg}>Error: {error}</p></div>

  const serverServices = services.filter(s => s.host === 'server')
  const wslServices = services.filter(s => s.host === 'wsl')
  const upCount = services.filter(s => s.status === 'up').length
  const totalCount = services.length

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <h2 className={styles.title}>📡 Services</h2>
        <div className={styles.headerRight}>
          <span className={styles.summary}>
            {upCount}/{totalCount} up
          </span>
          <button className={styles.refreshBtn} onClick={fetchServices}>
            🔄 Refresh
          </button>
          {lastCheck && (
            <span className={styles.lastCheck}>
              checked {lastCheck.toLocaleTimeString()}
            </span>
          )}
        </div>
      </div>

      <div className={styles.section}>
        <h3 className={styles.sectionTitle}>🖥️ Server (kayushkin.com)</h3>
        <div className={styles.grid}>
          {serverServices.map(s => (
            <ServiceCard key={s.name} service={s} />
          ))}
        </div>
      </div>

      {wslServices.length > 0 && (
        <div className={styles.section}>
          <h3 className={styles.sectionTitle}>🐧 WSL (local)</h3>
          <div className={styles.grid}>
            {wslServices.map(s => (
              <ServiceCard key={s.name} service={s} />
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

function ServiceCard({ service: s }: { service: Service }) {
  const [expanded, setExpanded] = useState(false)
  const detail = parseDetail(s.detail)

  return (
    <div className={`${styles.card} ${s.status === 'up' ? styles.cardUp : styles.cardDown}`}>
      <div className={styles.cardHeader} onClick={() => s.detail && setExpanded(!expanded)}>
        <span className={styles.cardIcon}>{statusIcon(s.status)}</span>
        <div className={styles.cardInfo}>
          <span className={styles.cardName}>{s.name}</span>
          <span className={styles.cardPort}>:{s.port}</span>
        </div>
        <span className={styles.cardLatency}>
          {s.status === 'up' ? `${s.latency_ms}ms` : s.status}
        </span>
      </div>
      {expanded && detail && (
        <div className={styles.cardDetail}>
          {Object.entries(detail).map(([k, v]) => (
            <div key={k} className={styles.detailRow}>
              <span className={styles.detailKey}>{k}</span>
              <span className={styles.detailVal}>{typeof v === 'object' ? JSON.stringify(v) : String(v)}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
