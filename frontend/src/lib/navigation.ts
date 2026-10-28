// Menu do FB_AUDITORIA — três áreas de gestão (pedido 27/10/2026):
//   Estratégico    → visão da diretoria/controladoria (DRE, fechamento, briefing)
//   Operacional    → acompanhamento do dia a dia (verbas, pagamentos a terceiros)
//   Administrativo → cadastros, carga de dados, usuários, trilha de auditoria
// Cada área é um item no rail (AppRail) e tem a sua barra de abas (ModuleTabs).
// Acesso: users.areas (+ admin de plataforma vê tudo); abas podem exigir perfil.

export interface ModuleTab {
  label: string
  path: string
  disabled?: boolean       // "em breve" — aparece, mas não navega
  perfis?: string[]        // só estes perfis (admin de plataforma sempre vê)
  masterOnly?: boolean     // só admin de plataforma
}

export interface ModuleConfig {
  label: string
  area?: 'estrategico' | 'operacional' | 'administrativo'
  tabs: ModuleTab[]
}

export const modules: Record<string, ModuleConfig> = {
  estrategico: {
    label: 'Estratégico',
    area: 'estrategico',
    tabs: [
      { label: 'DRE',               path: '/estrategico/dre' },
      { label: 'Fechamentos',       path: '/estrategico/fechamentos' },
      { label: 'Mapa do DRE',       path: '/estrategico/mapa', perfis: ['controladoria', 'auditoria'] },
      { label: 'Briefing diário',   path: '/estrategico/briefing', disabled: true },
    ],
  },
  operacional: {
    label: 'Operacional',
    area: 'operacional',
    tabs: [
      { label: 'Verbas de indústria',   path: '/operacional/verbas', disabled: true },
      { label: 'Pagamentos a terceiros', path: '/operacional/pagamentos', disabled: true },
      { label: 'Exceções',              path: '/operacional/excecoes', disabled: true },
    ],
  },
  administrativo: {
    label: 'Administrativo',
    area: 'administrativo',
    tabs: [
      { label: 'Carga de lançamentos', path: '/administrativo/carga', perfis: ['controladoria'] },
      { label: 'Usuários e perfis',    path: '/administrativo/usuarios', perfis: ['controladoria'] },
      { label: 'Trilha de auditoria',  path: '/administrativo/trilha', perfis: ['controladoria', 'auditoria'] },
    ],
  },
  config: {
    label: 'Configurações',
    tabs: [
      { label: 'Ambiente',               path: '/config/ambiente', masterOnly: true },
      { label: 'Dados e ambiente',       path: '/config/dados', masterOnly: true },
      { label: 'Usuários da plataforma', path: '/config/usuarios', masterOnly: true },
    ],
  },
}

export function getActiveModule(pathname: string): string {
  if (pathname.startsWith('/operacional'))    return 'operacional'
  if (pathname.startsWith('/administrativo')) return 'administrativo'
  if (pathname.startsWith('/config'))         return 'config'
  return 'estrategico'
}
