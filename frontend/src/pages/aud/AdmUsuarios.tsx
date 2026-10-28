import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from './util'
import { PERFIS } from '@/lib/perfis'

interface Dep { id: string; codigo: string; nome: string }
interface U { id: string; full_name: string; email: string; perfil: string; areas: string[]; departamentos: Dep[] }
interface Depto { id: string; codigo: string; nome: string; grupo: string }
const AREAS = [['empresa', 'Administrativo'], ['configuracoes', 'Configurações'], ['comercial', 'Comercial'], ['operacao', 'Operação'], ['acompanhamento', 'Acompanhamento']]

export default function AdmUsuarios() {
  const qc = useQueryClient()
  const q = useQuery({ queryKey: ['aud-usuarios'], queryFn: () => api<U[]>('/api/aud/usuarios') })

  const deptos = useQuery({ queryKey: ['aud-departamentos'], queryFn: () => api<Depto[]>('/api/aud/departamentos') })

  async function salvar(u: U, patch: Partial<U>, deptoIds?: string[]) {
    const n = { ...u, ...patch }
    if (n.areas.length === 0) { toast.error('Mantenha ao menos uma área'); return }
    try {
      await api('/api/aud/usuarios', { method: 'PUT', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ user_id: n.id, perfil: n.perfil, areas: n.areas, ...(n.perfil === 'gestor' && deptoIds ? { departamentos: deptoIds } : {}) }) })
      qc.invalidateQueries({ queryKey: ['aud-usuarios'] }); toast.success('Salvo')
    } catch (e: any) { toast.error(e.message) }
  }

  return (
    <div className="max-w-4xl rounded-lg border bg-white overflow-hidden">
      <table className="w-full text-sm">
        <thead><tr className="border-b bg-gray-50 text-xs text-muted-foreground">
          <th className="text-left p-2.5 font-medium">Usuário</th><th className="text-left p-2.5 font-medium">Perfil</th>
          <th className="text-left p-2.5 font-medium">Áreas do menu</th></tr></thead>
        <tbody>
          {(q.data ?? []).map(u => (
            <tr key={u.id} className="border-b last:border-0">
              <td className="p-2.5"><div className="font-medium">{u.full_name}</div><div className="text-xs text-muted-foreground">{u.email}</div></td>
              <td className="p-2.5">
                <select className="h-8 rounded-md border px-2 text-sm bg-white" value={u.perfil} onChange={e => salvar(u, { perfil: e.target.value })}>
                  {PERFIS.map(([v, l]) => <option key={v} value={v}>{l}</option>)}
                </select>
                {u.perfil === 'gestor' && (
                  <div className="mt-2 max-w-xs">
                    <div className="text-[11px] text-muted-foreground mb-1">
                      Departamentos do Gerente{u.departamentos.length === 0 ? ' (nenhum: não vê dado algum)' : ''}
                    </div>
                    <div className="flex flex-wrap gap-x-3 gap-y-1">
                      {(deptos.data ?? []).map(d => {
                        const ids = u.departamentos.map(x => x.id)
                        return (
                          <label key={d.id} className="flex items-center gap-1.5 text-xs" title={d.nome}>
                            <input type="checkbox" checked={ids.includes(d.id)}
                              onChange={e => salvar(u, {}, e.target.checked ? [...ids, d.id] : ids.filter(x => x !== d.id))} />
                            {d.codigo}
                          </label>
                        )
                      })}
                      {(deptos.data ?? []).length === 0 && <span className="text-xs text-muted-foreground">Cadastre departamentos primeiro.</span>}
                    </div>
                  </div>
                )}
              </td>
              <td className="p-2.5">
                <div className="flex flex-wrap gap-3">
                  {AREAS.map(([v, l]) => (
                    <label key={v} className="flex items-center gap-1.5 text-xs">
                      <input type="checkbox" checked={u.areas.includes(v)}
                        onChange={e => salvar(u, { areas: e.target.checked ? [...u.areas, v] : u.areas.filter(a => a !== v) })} />
                      {l}
                    </label>
                  ))}
                </div>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
