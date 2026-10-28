import { lazy, Suspense } from 'react'
import { BrowserRouter, Routes, Route, Navigate, useLocation, useParams, Link } from 'react-router-dom'
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
const ForgotPassword = lazy(() => import('./pages/ForgotPassword'))
const ResetPassword = lazy(() => import('./pages/ResetPassword'))
const GestaoAmbiente = lazy(() => import('./pages/GestaoAmbiente'))
const ConfigAmbienteEmpresa = lazy(() => import('./pages/ConfigAmbienteEmpresa'))
const AdminUsers = lazy(() => import('./pages/AdminUsers'))
const AreaHome = lazy(() => import('./pages/aud/AreaHome'))
const DRE = lazy(() => import('./pages/aud/DRE'))
const DREMapa = lazy(() => import('./pages/aud/DREMapa'))
const DREFechamentos = lazy(() => import('./pages/aud/DREFechamentos'))
const AdmCarga = lazy(() => import('./pages/aud/AdmCarga'))
const AdmUsuarios = lazy(() => import('./pages/aud/AdmUsuarios'))
const AdmFiliais = lazy(() => import('./pages/aud/AdmFiliais'))
const AdmDepartamentos = lazy(() => import('./pages/aud/AdmDepartamentos'))
const AdmDicionario = lazy(() => import('./pages/aud/AdmDicionario'))
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

// Caminho antigo -> novo, preservando o restante (/estrategico/dre -> /empresa/dre).
function RedirecionaPrefixo({ para }: { para: string }) {
  const resto = useParams()['*'] ?? ''
  const { search, hash } = useLocation()
  return <Navigate to={(resto ? `${para}/${resto}` : para) + search + hash} replace />
}

// "/" leva para a primeira área liberada.
function Landing() {
  const { loading, acessoCarregado, isAuthenticated, hasArea } = useAuth()
  if (loading || (isAuthenticated && !acessoCarregado)) return null
  if (hasArea('empresa')) return <Navigate to="/empresa/dre" replace />
  if (hasArea('comercial')) return <Navigate to="/comercial" replace />
  if (hasArea('operacao')) return <Navigate to="/operacao" replace />
  if (hasArea('configuracoes')) return <Navigate to="/configuracoes" replace />
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

function FaixaAmbiente() {
  const { ambienteEmpresa } = useAuth()
  if (ambienteEmpresa !== 'teste') return null
  return (
    <div role="status" className="shrink-0 bg-amber-500 text-amber-950 text-center text-xs font-bold tracking-widest py-1">
      AMBIENTE DE TESTE
    </div>
  )
}

function AppLayout() {
  return (
    <div className="flex h-screen overflow-hidden bg-background">
      <AppRail />
      <div className="flex flex-col flex-1 min-w-0">
        <FaixaAmbiente />
        <AppHeader />
        <ModuleTabs />
        <main className="flex-1 overflow-auto">
          <div className="p-4">
            <Routes>
              <Route path="/" element={<Landing />} />

              {/* Administrativo (área empresa) */}
              <Route path="/empresa" element={<Navigate to="/empresa/dre" replace />} />
              <Route path="/empresa/dre" element={<AreaRoute area="empresa"><DRE /></AreaRoute>} />
              <Route path="/empresa/fechamentos" element={<AreaRoute area="empresa"><DREFechamentos /></AreaRoute>} />
              <Route path="/empresa/mapa" element={<AreaRoute area="empresa" perfis={['controladoria', 'auditoria']}><DREMapa /></AreaRoute>} />

              {/* Configurações */}
              <Route path="/configuracoes" element={<AreaRoute area="configuracoes"><AreaHome moduleId="configuracoes" /></AreaRoute>} />
              <Route path="/configuracoes/carga" element={<AreaRoute area="configuracoes" perfis={['controladoria']}><AdmCarga /></AreaRoute>} />
              <Route path="/configuracoes/filiais" element={<AreaRoute area="configuracoes" perfis={['controladoria', 'auditoria']}><AdmFiliais /></AreaRoute>} />
              <Route path="/configuracoes/departamentos" element={<AreaRoute area="configuracoes" perfis={['controladoria', 'auditoria']}><AdmDepartamentos /></AreaRoute>} />
              <Route path="/configuracoes/dicionario" element={<AreaRoute area="configuracoes" perfis={['controladoria', 'auditoria']}><AdmDicionario /></AreaRoute>} />
              <Route path="/configuracoes/usuarios" element={<AreaRoute area="configuracoes" perfis={['controladoria']}><AdmUsuarios /></AreaRoute>} />
              <Route path="/configuracoes/trilha" element={<AreaRoute area="configuracoes" perfis={['controladoria', 'auditoria']}><AdmTrilha /></AreaRoute>} />

              {/* Comercial e Operação (em breve) */}
              <Route path="/comercial" element={<AreaRoute area="comercial"><AreaHome moduleId="comercial" /></AreaRoute>} />
              <Route path="/operacao" element={<AreaRoute area="operacao"><AreaHome moduleId="operacao" /></AreaRoute>} />

              {/* Caminhos antigos (ids de área anteriores) */}
              <Route path="/estrategico/*" element={<RedirecionaPrefixo para="/empresa" />} />
              <Route path="/administrativo/*" element={<RedirecionaPrefixo para="/configuracoes" />} />
              <Route path="/operacional" element={<Navigate to="/operacao" replace />} />

              {/* Plataforma (admin de plataforma) */}
              <Route path="/config" element={<Navigate to="/config/ambiente" replace />} />
              <Route path="/config/ambiente" element={<PlatformAdminRoute><GestaoAmbiente /></PlatformAdminRoute>} />
              <Route path="/config/dados" element={<PlatformAdminRoute><ConfigAmbienteEmpresa /></PlatformAdminRoute>} />
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
              {/* Cadastro público fechado (story 4.1): usuários são criados pelo administrador. */}
              <Route path="/register" element={<Navigate to="/login" replace />} />
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
