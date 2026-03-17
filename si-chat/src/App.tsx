import { Routes, Route } from 'react-router-dom'
import DashboardLayout from './components/DashboardLayout'
import SiChat from './pages/SiChat'
import ModelStatus from './pages/ModelStatus'
import Services from './pages/Services'
import Agents from './pages/Agents'
import Usage from './pages/Usage'
import Forge from './pages/Forge'
import Topology from './pages/Topology'

export default function App() {
  return (
    <Routes>
      <Route element={<DashboardLayout />}>
        <Route index element={<SiChat />} />
        <Route path="agents" element={<Agents />} />
        <Route path="models" element={<ModelStatus />} />
        <Route path="usage" element={<Usage />} />
        <Route path="services" element={<Services />} />
        <Route path="forge" element={<Forge />} />
        <Route path="topology" element={<Topology />} />
      </Route>
    </Routes>
  )
}
