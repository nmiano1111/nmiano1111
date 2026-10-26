import { Routes, Route } from 'react-router-dom'
import { Layout } from '@/components/layout/Layout'
import { OverviewPage } from '@/pages/Overview'
import { RunsPage } from '@/pages/Runs'
import { RunDetailPage } from '@/pages/RunDetail'
import { NotFoundPage } from '@/pages/NotFound'

export default function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route path="/" element={<OverviewPage />} />
        <Route path="/runs" element={<RunsPage />} />
        <Route path="/runs/:id" element={<RunDetailPage />} />
        <Route path="*" element={<NotFoundPage />} />
      </Route>
    </Routes>
  )
}
