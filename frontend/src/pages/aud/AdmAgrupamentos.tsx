import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { useAuth } from '@/contexts/AuthContext'
import { api } from './util'

interface FG { id: string; codigo: string; nome: string; filiais: string[] }
interface Agr { id: string; codigo: string; nome: string; filiais: string[] }
interface Form { id: string; codigo: string; nome: string; filiais: string[] }

const VAZIO: Form = { id: '', codigo: '', nome: '', filiais: [] }
const URL = '/api/aud/agrupamentos-filiais'

export default function AdmAgrupamentos() {
  const { perfil, isAdmin } = useAuth()
  const editar = isAdmin || ['analista', 'controladoria'].includes(perfil ?? '')
  const qc = useQueryClient()
  const q = useQuery({ queryKey: ['aud-agrupamentos'], queryFn: () => api<Agr[]>(URL) })
  const gerenciais = useQuery({ queryKey: ['aud-filiais-gerenciais'], queryFn: () => api<FG[]>('/api/aud/filiais-gerenciais') })
  const [form, setForm] = useState<Form | null>(null)
  const [ocupado, setOcupado] = useState(false)

  const recarregar = () => qc.invalidateQueries({ queryKey: ['aud-agrupamentos'] })

  function alternar(codigo: string) {
    if (!form) return
    const tem = form.filiais.includes(codigo)
    setForm({ ...form, filiais: tem ? form.filiais.filter(c => c !== codigo) : [...form.filiais, codigo] })
  }

  async function salvar() {
    if (!form || ocupado) return
    setOcupado(true)
    try {
      await api(URL, {
        method: form.id ? 'PUT' : 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: form.id, codigo: form.codigo, nome: form.nome, filiais: form.filiais }),
      })
      toast.success('Salvo'); setForm(null); recarregar()
    } catch (e: any) { toast.error(e.message) } finally { setOcupado(false) }
  }

  async function excluir(a: Agr) {
    if (ocupado || !window.confirm(`Excluir o agrupamento ${a.codigo}?`)) return
    setOcupado(true)
    try {
      await api(`${URL}?id=${encodeURIComponent(a.id)}`, { method: 'DELETE' })
      toast.success('Excluído'); recarregar()
    } catch (e: any) { toast.error(e.message) } finally { setOcupado(false) }
  }

  return (
    <div className="max-w-4xl space-y-3">
      <p className="text-sm text-muted-foreground">
        Um agrupamento soma filiais gerenciais numa visão nomeada. Uma filial gerencial pode estar em vários agrupamentos.
        A visão de todas as filiais (Grupo) não precisa de cadastro; o código GRUPO é reservado.
      </p>
      {editar && !form && (
        <button className="h-8 rounded-md border bg-white px-3 text-sm" onClick={() => setForm(VAZIO)}>Novo agrupamento</button>
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
          <fieldset className="text-xs">
            <legend className="mb-1">Filiais gerenciais do agrupamento</legend>
            {gerenciais.isLoading && <p className="text-muted-foreground">Carregando...</p>}
            {gerenciais.isError && <p className="text-red-600">Erro ao carregar as filiais gerenciais.</p>}
            {gerenciais.data?.length === 0 && <p className="text-muted-foreground">Cadastre antes as filiais gerenciais, na aba Filiais.</p>}
            <div className="flex flex-wrap gap-x-4 gap-y-1">
              {(gerenciais.data ?? []).map(g => (
                <label key={g.id} className="flex items-center gap-1.5 text-sm">
                  <input type="checkbox" checked={form.filiais.includes(g.codigo)} onChange={() => alternar(g.codigo)} />
                  {g.codigo} <span className="text-muted-foreground">{g.nome}</span>
                </label>
              ))}
            </div>
          </fieldset>
          <div className="flex gap-2">
            <button className="h-8 rounded-md bg-gray-900 px-3 text-white disabled:opacity-50" disabled={ocupado} onClick={salvar}>Salvar</button>
            <button className="h-8 rounded-md border px-3" onClick={() => setForm(null)}>Cancelar</button>
          </div>
        </div>
      )}
      {q.isLoading && <p className="text-sm text-muted-foreground">Carregando...</p>}
      {q.isError && <p className="text-sm text-red-600">Erro ao carregar os agrupamentos: {(q.error as Error).message}</p>}
      {!q.isLoading && !q.isError && (
        <div className="rounded-lg border bg-white overflow-hidden">
          <table className="w-full text-sm">
            <thead><tr className="border-b bg-gray-50 text-xs text-muted-foreground">
              <th className="text-left p-2.5 font-medium">Código</th><th className="text-left p-2.5 font-medium">Nome</th>
              <th className="text-left p-2.5 font-medium">Filiais gerenciais</th>{editar && <th className="p-2.5" />}</tr></thead>
            <tbody>
              {(q.data ?? []).map(a => (
                <tr key={a.id} className="border-b last:border-0">
                  <td className="p-2.5 font-medium">{a.codigo}</td>
                  <td className="p-2.5">{a.nome}</td>
                  <td className="p-2.5">{a.filiais.join(', ') || '—'}</td>
                  {editar && (
                    <td className="p-2.5 text-right whitespace-nowrap">
                      <button className="text-xs underline mr-3"
                        onClick={() => setForm({ id: a.id, codigo: a.codigo, nome: a.nome, filiais: a.filiais })}>Editar</button>
                      <button className="text-xs underline text-red-600 disabled:opacity-50" disabled={ocupado} onClick={() => excluir(a)}>Excluir</button>
                    </td>
                  )}
                </tr>
              ))}
              {q.data?.length === 0 && (
                <tr><td className="p-2.5 text-muted-foreground" colSpan={editar ? 4 : 3}>Nenhum agrupamento cadastrado.</td></tr>
              )}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
