import { useState } from 'react'
import { Outlet, NavLink } from 'react-router-dom'
import ThemeToggle from './ThemeToggle'
import styles from './DashboardLayout.module.css'

export default function DashboardLayout() {
  const [navOpen, setNavOpen] = useState(false)

  return (
    <div className={styles.dashboard}>
      <header className={styles.header}>
        <div className={styles.headerLeft}>
          <button
            className={styles.menuToggle}
            onClick={() => setNavOpen(!navOpen)}
            aria-label="Toggle navigation"
          >
            {navOpen ? '✕' : '☰'}
          </button>
          <ThemeToggle />
        </div>

        {navOpen && <div className={styles.navOverlay} onClick={() => setNavOpen(false)} />}

        <nav className={`${styles.nav} ${navOpen ? styles.navOpen : ''}`}>
          <NavLink to="/" end className={({isActive}) => isActive ? styles.navActive : styles.navLink} onClick={() => setNavOpen(false)}>💬 Chat</NavLink>
          <NavLink to="/agents" className={({isActive}) => isActive ? styles.navActive : styles.navLink} onClick={() => setNavOpen(false)}>🎭 Agents</NavLink>
          <NavLink to="/models" className={({isActive}) => isActive ? styles.navActive : styles.navLink} onClick={() => setNavOpen(false)}>🤖 Models</NavLink>
          <NavLink to="/usage" className={({isActive}) => isActive ? styles.navActive : styles.navLink} onClick={() => setNavOpen(false)}>📊 Usage</NavLink>
          <NavLink to="/services" className={({isActive}) => isActive ? styles.navActive : styles.navLink} onClick={() => setNavOpen(false)}>📡 Services</NavLink>
          <NavLink to="/forge" className={({isActive}) => isActive ? styles.navActive : styles.navLink} onClick={() => setNavOpen(false)}>🔨 Forge</NavLink>
          <NavLink to="/topology" className={({isActive}) => isActive ? styles.navActive : styles.navLink} onClick={() => setNavOpen(false)}>🕸️ Topology</NavLink>
        </nav>
      </header>

      <main className={styles.content}>
        <Outlet />
      </main>
    </div>
  )
}
