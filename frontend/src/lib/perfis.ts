// Rótulos dos perfis (users.perfil). O valor gravado no banco não muda; o
// rótulo é o que o cliente vê: controladoria = Controller, gestor = Gerente.
export const PERFIS: ReadonlyArray<readonly [string, string]> = [
  ['analista', 'Analista'],
  ['controladoria', 'Controller'],
  ['gestor', 'Gerente'],
  ['diretoria', 'Diretoria'],
  ['auditoria', 'Auditoria'],
]

export function rotuloPerfil(perfil: string | null | undefined): string {
  return PERFIS.find(([v]) => v === perfil)?.[1] ?? perfil ?? ''
}
