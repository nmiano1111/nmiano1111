import { Routes, Route, Navigate } from 'react-router-dom'
import { Layout } from '@/components/layout/Layout'
import { OverviewPage } from '@/pages/Overview'
import { JobsPage } from '@/pages/Jobs'
import { JobPage } from '@/pages/Job'
import { RunDetailPage } from '@/pages/RunDetail'
import { NotFoundPage } from '@/pages/NotFound'

export default function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route path="/" element={<OverviewPage />} />
        <Route path="/jobs" element={<JobsPage />} />
        <Route path="/jobs/:id" element={<JobPage />} />
        <Route path="/runs" element={<Navigate to="/jobs" replace />} />
        <Route path="/runs/:id" element={<RunDetailPage />} />
        <Route path="*" element={<NotFoundPage />} />
      </Route>
    </Routes>
  )
}
