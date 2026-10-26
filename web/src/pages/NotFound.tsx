import { Link } from 'react-router-dom'
import { Button } from '@/components/ui/Button'

export function NotFoundPage() {
  return (
    <div className="flex flex-col items-center justify-center h-full py-32 text-center px-6">
      <p className="text-5xl mb-4 opacity-20">404</p>
      <p className="text-base font-medium text-text mb-2">Page not found</p>
      <p className="text-sm text-text-dim mb-6">
        The page you're looking for doesn't exist.
      </p>
      <Link to="/">
        <Button variant="default">Go home</Button>
      </Link>
    </div>
  )
}
