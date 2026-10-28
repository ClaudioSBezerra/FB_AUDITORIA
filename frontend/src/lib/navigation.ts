// Menu do FB_AUDITORIA — quatro áreas de gestão (AD-12) mais o módulo "Plataforma":
//   Administrativo (id empresa) → DRE, fechamentos, Mapa do DRE
//   Configurações               → carga de dados, usuários, trilha de auditoria
//   Comercial / Operação        → em breve
//   Plataforma (config)         → ambiente e usuários da plataforma (só admin de plataforma)
// Cada área é um item no rail (AppRail) e tem a sua barra de abas (ModuleTabs).
// Acesso: users.areas (+ admin de plataforma vê tudo); abas podem exigir perfil.

export interface ModuleTab {
  label: string
  path: string
  disabled?: boolean       // "em breve" — aparece, mas não navega
  perfis?: string[]        // só estes perfis (admin de plataforma sempre vê)
  masterOnly?: boolean     // só admin de plataforma
}

export type AreaId = 'empresa' | 'configuracoes' | 'comercial' | 'operacao'

export interface ModuleConfig {
  label: string
  area?: AreaId
  tabs: ModuleTab[]
}

export const modules: Record<string, ModuleConfig> = {
  empresa: {
    label: 'Administrativo',
    area: 'empresa',
    tabs: [
      { label: 'DRE',         path: '/empresa/dre' },
      { label: 'Fechamentos', path: '/empresa/fechamentos' },
      { label: 'Mapa do DRE', path: '/empresa/mapa', perfis: ['controladoria', 'auditoria'] },
    ],
  },
  configuracoes: {
    label: 'Configurações',
    area: 'configuracoes',
    tabs: [
      { label: 'Carga de lançamentos', path: '/configuracoes/carga', perfis: ['controladoria'] },
      { label: 'Filiais',             path: '/configuracoes/filiais', perfis: ['controladoria', 'auditoria'] },
      { label: 'Departamentos',       path: '/configuracoes/departamentos', perfis: ['controladoria', 'auditoria'] },
      { label: 'Dicionário',          path: '/configuracoes/dicionario', perfis: ['controladoria', 'auditoria'] },
      { label: 'Usuários e perfis',    path: '/configuracoes/usuarios', perfis: ['controladoria'] },
      { label: 'Trilha de auditoria',  path: '/configuracoes/trilha', perfis: ['controladoria', 'auditoria'] },
    ],
  },
  comercial: {
    label: 'Comercial',
    area: 'comercial',
    tabs: [{ label: 'Em breve', path: '/comercial/em-breve', disabled: true }],
  },
  operacao: {
    label: 'Operação',
    area: 'operacao',
    tabs: [{ label: 'Em breve', path: '/operacao/em-breve', disabled: true }],
  },
  config: {
    label: 'Plataforma',
    tabs: [
      { label: 'Ambiente',               path: '/config/ambiente', masterOnly: true },
      { label: 'Dados e ambiente',       path: '/config/dados', masterOnly: true },
      { label: 'Usuários da plataforma', path: '/config/usuarios', masterOnly: true },
    ],
  },
}

export function getActiveModule(pathname: string): string {
  if (pathname.startsWith('/comercial'))      return 'comercial'
  if (pathname.startsWith('/operacao'))       return 'operacao'
  if (pathname.startsWith('/configuracoes'))  return 'configuracoes'
  if (pathname.startsWith('/config'))         return 'config'
  return 'empresa'
}
