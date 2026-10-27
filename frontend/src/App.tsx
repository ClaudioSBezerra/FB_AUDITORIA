import { lazy, Suspense } from 'react'
import { BrowserRouter, Routes, Route, Navigate, useLocation, Link } from 'react-router-dom'
import { QueryClientProvider } from '@tanstack/react-query'
import { queryClient } from '@/lib/queryClient'
import { Toaster } from '@/components/ui/sonner'
import { AppRail } from '@/components/AppRail'
import { CompanySwitcher } from '@/components/CompanySwitcher'
import { AuthProvider, useAuth } from './contexts/AuthContext'
import { getActiveModule, modules } from '@/lib/navigation'
import { cn } from '@/lib/utils'

// Páginas autenticadas ficam em chunks separados; só o login/registro entram cedo.
const Login = lazy(() => import('./pages/Login'))
const Register = lazy(() => import('./pages/Register'))
const ForgotPassword = lazy(() => import('./pages/ForgotPassword'))
const ResetPassword = lazy(() => import('./pages/ResetPassword'))
const GestaoAmbiente = lazy(() => import('./pages/GestaoAmbiente'))
const AdminUsers = lazy(() => import('./pages/AdminUsers'))
const AreaHome = lazy(() => import('./pages/aud/AreaHome'))
const DRE = lazy(() => import('./pages/aud/DRE'))
const DREMapa = lazy(() => import('./pages/aud/DREMapa'))
const DREFechamentos = lazy(() => import('./pages/aud/DREFechamentos'))
const AdmCarga = lazy(() => import('./pages/aud/AdmCarga'))
const AdmUsuarios = lazy(() => import('./pages/aud/AdmUsuarios'))
const AdmTrilha = lazy(() => import('./pages/aud/AdmTrilha'))

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, loading } = useAuth()
  const location = useLocation()
  if (loading) return null
  if (!isAuthenticated) return <Navigate to="/login" state={{ from: location }} replace />
  return <>{children}</>
}

// Rota de uma área do menu (users.areas) e, opcionalmente, de perfis específicos.
// Espera /api/aud/me responder antes de decidir (senão o 1º render, sem áreas,
// mandaria o usuário embora da própria tela). O backend revalida tudo.
function AreaRoute({ area, perfis, children }: { area: string; perfis?: string[]; children: React.ReactNode }) {
  const { isAuthenticated, loading, hasArea, acessoCarregado, perfil, isAdmin } = useAuth()
  const location = useLocation()
  if (loading) return null
  if (!isAuthenticated) return <Navigate to="/login" state={{ from: location }} replace />
  if (!acessoCarregado) return null
  if (!hasArea(area)) return <Navigate to="/" replace />
  if (perfis && !isAdmin && !(perfil && perfis.includes(perfil))) return <Navigate to="/" replace />
  return <>{children}</>
}

function PlatformAdminRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, loading, isAdmin, acessoCarregado } = useAuth()
  const location = useLocation()
  if (loading) return null
  if (!isAuthenticated) return <Navigate to="/login" state={{ from: location }} replace />
  if (!acessoCarregado) return null
  if (!isAdmin) return <Navigate to="/" replace />
  return <>{children}</>
}

// "/" leva para a primeira área liberada.
function Landing() {
  const { loading, acessoCarregado, isAuthenticated, hasArea } = useAuth()
  if (loading || (isAuthenticated && !acessoCarregado)) return null
  if (hasArea('estrategico')) return <Navigate to="/estrategico/dre" replace />
  if (hasArea('operacional')) return <Navigate to="/operacional" replace />
  if (hasArea('administrativo')) return <Navigate to="/administrativo" replace />
  return <p className="text-sm text-muted-foreground">Seu usuário ainda não tem nenhuma área liberada. Fale com a controladoria.</p>
}

// ── Barra de abas por área ───────────────────────────────────────────────────
function ModuleTabs() {
  const location = useLocation()
  const { perfil, isAdmin } = useAuth()
  const moduleCfg = modules[getActiveModule(location.pathname)]
  if (!moduleCfg || moduleCfg.tabs.length === 0) return null

  const visibleTabs = moduleCfg.tabs.filter(t =>
    (!t.masterOnly || isAdmin) &&
    (!t.perfis || isAdmin || (perfil != null && t.perfis.includes(perfil)))
  )
  if (visibleTabs.length === 0) return null

  return (
    <div className="border-b bg-white px-4 flex items-center gap-0.5 overflow-x-auto shrink-0 h-10">
      {visibleTabs.map(tab => tab.disabled ? (
        <span key={tab.path} title="Em breve" className="px-3 py-1.5 text-xs rounded-md text-muted-foreground/50 cursor-not-allowed whitespace-nowrap">
          {tab.label}
        </span>
      ) : (
        <Link key={tab.path} to={tab.path}
          className={cn(
            'px-3 py-1.5 text-xs rounded-md whitespace-nowrap transition-colors',
            location.pathname === tab.path
              ? 'bg-primary/10 text-primary font-semibold'
              : 'text-muted-foreground hover:bg-gray-100 hover:text-foreground'
          )}>
          {tab.label}
        </Link>
      ))}
    </div>
  )
}

function AppHeader() {
  const location = useLocation()
  const moduleCfg = modules[getActiveModule(location.pathname)]
  const { company } = useAuth()
  return (
    <header className="flex items-center justify-between h-12 border-b bg-white px-4 shrink-0">
      <span className="text-sm font-semibold text-foreground">{moduleCfg?.label ?? 'Auditoria'}</span>
      <div className="flex items-center gap-2">
        {company && (
          <span className="text-xs font-medium text-sky-700 bg-sky-50 border border-sky-200 px-2.5 py-1 rounded-full max-w-[220px] truncate" title={company}>
            {company}
          </span>
        )}
        <CompanySwitcher compact />
      </div>
    </header>
  )
}

function AppLayout() {
  return (
    <div className="flex h-screen overflow-hidden bg-background">
      <AppRail />
      <div className="flex flex-col flex-1 min-w-0">
        <AppHeader />
        <ModuleTabs />
        <main className="flex-1 overflow-auto">
          <div className="p-4">
            <Routes>
              <Route path="/" element={<Landing />} />

              {/* Estratégico */}
              <Route path="/estrategico" element={<Navigate to="/estrategico/dre" replace />} />
              <Route path="/estrategico/dre" element={<AreaRoute area="estrategico"><DRE /></AreaRoute>} />
              <Route path="/estrategico/fechamentos" element={<AreaRoute area="estrategico"><DREFechamentos /></AreaRoute>} />
              <Route path="/estrategico/mapa" element={<AreaRoute area="estrategico" perfis={['controladoria', 'auditoria']}><DREMapa /></AreaRoute>} />

              {/* Operacional */}
              <Route path="/operacional" element={<AreaRoute area="operacional"><AreaHome moduleId="operacional" /></AreaRoute>} />

              {/* Administrativo */}
              <Route path="/administrativo" element={<AreaRoute area="administrativo"><AreaHome moduleId="administrativo" /></AreaRoute>} />
              <Route path="/administrativo/carga" element={<AreaRoute area="administrativo" perfis={['controladoria']}><AdmCarga /></AreaRoute>} />
              <Route path="/administrativo/usuarios" element={<AreaRoute area="administrativo" perfis={['controladoria']}><AdmUsuarios /></AreaRoute>} />
              <Route path="/administrativo/trilha" element={<AreaRoute area="administrativo" perfis={['controladoria', 'auditoria']}><AdmTrilha /></AreaRoute>} />

              {/* Configurações (admin de plataforma) */}
              <Route path="/config" element={<Navigate to="/config/ambiente" replace />} />
              <Route path="/config/ambiente" element={<PlatformAdminRoute><GestaoAmbiente /></PlatformAdminRoute>} />
              <Route path="/config/usuarios" element={<PlatformAdminRoute><AdminUsers /></PlatformAdminRoute>} />

              <Route path="*" element={<Navigate to="/" replace />} />
            </Routes>
          </div>
        </main>
      </div>
    </div>
  )
}

function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <AuthProvider>
          <Toaster />
          <Suspense fallback={null}>
            <Routes>
              <Route path="/login" element={<Login />} />
              <Route path="/register" element={<Register />} />
              <Route path="/forgot-password" element={<ForgotPassword />} />
              <Route path="/reset-senha" element={<ResetPassword />} />
              <Route path="/*" element={<ProtectedRoute><AppLayout /></ProtectedRoute>} />
            </Routes>
          </Suspense>
        </AuthProvider>
      </BrowserRouter>
    </QueryClientProvider>
  )
}

export default App
