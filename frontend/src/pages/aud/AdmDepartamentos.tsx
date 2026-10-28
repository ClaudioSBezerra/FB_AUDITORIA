import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { useAuth } from '@/contexts/AuthContext'
import { api } from './util'

interface Depto { id: string; codigo: string; nome: string; grupo: string }

const GRUPOS = ['Comercial', 'Logística', 'Administrativo']
const VAZIO: Depto = { id: '', codigo: '', nome: '', grupo: GRUPOS[0] }
const URL = '/api/aud/departamentos'

export default function AdmDepartamentos() {
  const { perfil, isAdmin } = useAuth()
  const editar = isAdmin || perfil === 'controladoria'
  const qc = useQueryClient()
  const q = useQuery({ queryKey: ['aud-departamentos'], queryFn: () => api<Depto[]>(URL) })
  const [form, setForm] = useState<Depto | null>(null)
  const [ocupado, setOcupado] = useState(false)

  const recarregar = () => qc.invalidateQueries({ queryKey: ['aud-departamentos'] })

  async function salvar() {
    if (!form || ocupado) return
    setOcupado(true)
    try {
      await api(URL, {
        method: form.id ? 'PUT' : 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(form),
      })
      toast.success('Salvo'); setForm(null); recarregar()
    } catch (e: any) { toast.error(e.message) } finally { setOcupado(false) }
  }

  async function excluir(d: Depto) {
    if (ocupado || !window.confirm(`Excluir o departamento ${d.codigo}?`)) return
    setOcupado(true)
    try {
      await api(`${URL}?id=${encodeURIComponent(d.id)}`, { method: 'DELETE' })
      toast.success('Excluído'); recarregar()
    } catch (e: any) { toast.error(e.message) } finally { setOcupado(false) }
  }

  return (
    <div className="max-w-4xl space-y-3">
      <p className="text-sm text-muted-foreground">
        Departamentos da empresa e o grupo de cada um. As contas se ligam a eles pelo dicionário.
      </p>
      {editar && !form && (
        <button className="h-8 rounded-md border bg-white px-3 text-sm" onClick={() => setForm(VAZIO)}>Novo departamento</button>
      )}
      {form && (
        <div className="rounded-lg border bg-white p-3 space-y-2 text-sm">
          <div className="flex gap-2">
            <label className="text-xs">Código
              <input className="mt-1 block h-8 w-32 rounded-md border px-2 text-sm" value={form.codigo}
              onChange={e => setForm({ ...form, codigo: e.target.value })} /></label>
            <label className="text-xs flex-1">Nome
              <input className="mt-1 block h-8 w-full rounded-md border px-2 text-sm" value={form.nome}
              onChange={e => setForm({ ...form, nome: e.target.value })} /></label>
            <label className="text-xs">Grupo
              <select className="mt-1 block h-8 rounded-md border px-2 text-sm" value={form.grupo}
              onChange={e => setForm({ ...form, grupo: e.target.value })}>
                {GRUPOS.map(g => <option key={g} value={g}>{g}</option>)}
              </select></label>
          </div>
          <div className="flex gap-2">
            <button className="h-8 rounded-md bg-gray-900 px-3 text-white disabled:opacity-50" disabled={ocupado} onClick={salvar}>Salvar</button>
            <button className="h-8 rounded-md border px-3" onClick={() => setForm(null)}>Cancelar</button>
          </div>
        </div>
      )}
      {q.isLoading && <p className="text-sm text-muted-foreground">Carregando...</p>}
      {q.isError && <p className="text-sm text-red-600">Erro ao carregar os departamentos: {(q.error as Error).message}</p>}
      {!q.isLoading && !q.isError && (
      <div className="rounded-lg border bg-white overflow-hidden">
        <table className="w-full text-sm">
          <thead><tr className="border-b bg-gray-50 text-xs text-muted-foreground">
            <th className="text-left p-2.5 font-medium">Código</th><th className="text-left p-2.5 font-medium">Nome</th>
            <th className="text-left p-2.5 font-medium">Grupo</th>{editar && <th className="p-2.5" />}</tr></thead>
          <tbody>
            {(q.data ?? []).map(d => (
              <tr key={d.id} className="border-b last:border-0">
                <td className="p-2.5 font-medium">{d.codigo}</td>
                <td className="p-2.5">{d.nome}</td>
                <td className="p-2.5">{d.grupo}</td>
                {editar && (
                  <td className="p-2.5 text-right whitespace-nowrap">
                    <button className="text-xs underline mr-3" onClick={() => setForm(d)}>Editar</button>
                    <button className="text-xs underline text-red-600 disabled:opacity-50" disabled={ocupado} onClick={() => excluir(d)}>Excluir</button>
                  </td>
                )}
              </tr>
            ))}
            {q.data?.length === 0 && (
              <tr><td className="p-2.5 text-muted-foreground" colSpan={editar ? 4 : 3}>Nenhum departamento cadastrado.</td></tr>
            )}
          </tbody>
        </table>
      </div>
      )}
    </div>
  )
}
