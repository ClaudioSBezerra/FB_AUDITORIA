import { Link } from 'react-router-dom'
import { modules } from '@/lib/navigation'
import { useAuth } from '@/contexts/AuthContext'

// Página de entrada de uma área do menu: lista o que existe e o que vem em breve.
export default function AreaHome({ moduleId }: { moduleId: string }) {
  const { perfil, isAdmin } = useAuth()
  const cfg = modules[moduleId]
  const tabs = cfg.tabs.filter(t => isAdmin || !t.perfis || (perfil != null && t.perfis.includes(perfil)))
  return (
    <div className="max-w-3xl space-y-4">
      <div>
        <h1 className="text-xl font-semibold">{cfg.label}</h1>
        <p className="text-sm text-muted-foreground">Funcionalidades desta área.</p>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        {tabs.map(t => t.disabled ? (
          <div key={t.path} className="rounded-lg border border-dashed p-4 text-muted-foreground">
            <p className="text-sm font-medium">{t.label}</p>
            <p className="text-xs mt-1">Em breve</p>
          </div>
        ) : (
          <Link key={t.path} to={t.path} className="rounded-lg border bg-white p-4 hover:border-primary/50 transition-colors">
            <p className="text-sm font-medium">{t.label}</p>
          </Link>
        ))}
        {tabs.length === 0 && <p className="text-sm text-muted-foreground">Nada disponível para o seu perfil nesta área.</p>}
      </div>
    </div>
  )
}
