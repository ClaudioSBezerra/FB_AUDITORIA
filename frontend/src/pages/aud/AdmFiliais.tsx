import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { useAuth } from '@/contexts/AuthContext'
import { api } from './util'

interface FG { id: string; codigo: string; nome: string; filiais: string[] }
interface Form { id: string; codigo: string; nome: string; filiais: string }

const VAZIO: Form = { id: '', codigo: '', nome: '', filiais: '' }
const URL = '/api/aud/filiais-gerenciais'

export default function AdmFiliais() {
  const { perfil, isAdmin } = useAuth()
  const editar = isAdmin || ['analista', 'controladoria'].includes(perfil ?? '')
  const qc = useQueryClient()
  const q = useQuery({ queryKey: ['aud-filiais-gerenciais'], queryFn: () => api<FG[]>(URL) })
  const [form, setForm] = useState<Form | null>(null)
  const [ocupado, setOcupado] = useState(false)

  const recarregar = () => qc.invalidateQueries({ queryKey: ['aud-filiais-gerenciais'] })

  async function salvar() {
    if (!form || ocupado) return
    setOcupado(true)
    const filiais = form.filiais.split(/[\s,;]+/).map(s => s.trim()).filter(Boolean)
    try {
      await api(URL, {
        method: form.id ? 'PUT' : 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: form.id, codigo: form.codigo, nome: form.nome, filiais }),
      })
      toast.success('Salvo'); setForm(null); recarregar()
    } catch (e: any) { toast.error(e.message) } finally { setOcupado(false) }
  }

  async function excluir(g: FG) {
    if (ocupado || !window.confirm(`Excluir a filial gerencial ${g.codigo}?`)) return
    setOcupado(true)
    try {
      await api(`${URL}?id=${encodeURIComponent(g.id)}`, { method: 'DELETE' })
      toast.success('Excluída'); recarregar()
    } catch (e: any) { toast.error(e.message) } finally { setOcupado(false) }
  }

  return (
    <div className="max-w-4xl space-y-3">
      <p className="text-sm text-muted-foreground">
        Uma filial gerencial agrupa filiais do ERP. A filial do ERP sem ligação vale por si.
      </p>
      {editar && !form && (
        <button className="h-8 rounded-md border bg-white px-3 text-sm" onClick={() => setForm(VAZIO)}>Nova filial gerencial</button>
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
          </div>
          <label className="block text-xs">Filiais do ERP
            <input className="mt-1 block h-8 w-full rounded-md border px-2 text-sm" placeholder="separadas por vírgula, ponto e vírgula ou espaço" value={form.filiais}
              onChange={e => setForm({ ...form, filiais: e.target.value })} /></label>
          <div className="flex gap-2">
            <button className="h-8 rounded-md bg-gray-900 px-3 text-white disabled:opacity-50" disabled={ocupado} onClick={salvar}>Salvar</button>
            <button className="h-8 rounded-md border px-3" onClick={() => setForm(null)}>Cancelar</button>
          </div>
        </div>
      )}
      {q.isLoading && <p className="text-sm text-muted-foreground">Carregando...</p>}
      {q.isError && <p className="text-sm text-red-600">Erro ao carregar as filiais: {(q.error as Error).message}</p>}
      {!q.isLoading && !q.isError && (
      <div className="rounded-lg border bg-white overflow-hidden">
        <table className="w-full text-sm">
          <thead><tr className="border-b bg-gray-50 text-xs text-muted-foreground">
            <th className="text-left p-2.5 font-medium">Código</th><th className="text-left p-2.5 font-medium">Nome</th>
            <th className="text-left p-2.5 font-medium">Filiais do ERP</th>{editar && <th className="p-2.5" />}</tr></thead>
          <tbody>
            {(q.data ?? []).map(g => (
              <tr key={g.id} className="border-b last:border-0">
                <td className="p-2.5 font-medium">{g.codigo}</td>
                <td className="p-2.5">{g.nome}</td>
                <td className="p-2.5">{g.filiais.join(', ') || '—'}</td>
                {editar && (
                  <td className="p-2.5 text-right whitespace-nowrap">
                    <button className="text-xs underline mr-3"
                      onClick={() => setForm({ id: g.id, codigo: g.codigo, nome: g.nome, filiais: g.filiais.join(', ') })}>Editar</button>
                    <button className="text-xs underline text-red-600 disabled:opacity-50" disabled={ocupado} onClick={() => excluir(g)}>Excluir</button>
                  </td>
                )}
              </tr>
            ))}
            {q.data?.length === 0 && (
              <tr><td className="p-2.5 text-muted-foreground" colSpan={editar ? 4 : 3}>Nenhuma filial gerencial cadastrada.</td></tr>
            )}
          </tbody>
        </table>
      </div>
      )}
    </div>
  )
}
