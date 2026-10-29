import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { useAuth } from '@/contexts/AuthContext'
import { Upload, Download } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { api } from './util'

interface Depto { id: string; codigo: string; nome: string; grupo: string }

const GRUPOS = ['Comercial', 'Logística', 'Administrativo']
const VAZIO: Depto = { id: '', codigo: '', nome: '', grupo: GRUPOS[0] }
const URL = '/api/aud/departamentos'

// Modelo fictício gerado no navegador (nenhum dado real).
const MODELO = [
  'codigo;nome;grupo',
  'VEN;Vendas;Comercial',
  'LOG;Logistica;Logística',
  'ADM;Administracao;Administrativo',
].join('\n')

function baixarModelo() {
  const url = window.URL.createObjectURL(new Blob(['\ufeff' + MODELO + '\n'], { type: 'text/csv;charset=utf-8' }))
  const a = document.createElement('a')
  a.href = url; a.download = 'modelo_departamentos.csv'; a.click()
  window.URL.revokeObjectURL(url)
}

interface ResultadoCarga { linhas: number; criados: number; atualizados: number; inalterados: number }

// Bloco "Carregar departamentos" (história 2.20): upsert por código; os que não vêm no arquivo ficam.
function BlocoCarga({ aoCarregar }: { aoCarregar: () => void }) {
  const [arquivo, setArquivo] = useState<File | null>(null)
  const [ocupado, setOcupado] = useState(false)
  const [resultado, setResultado] = useState<ResultadoCarga | null>(null)
  const [erro, setErro] = useState('')

  async function enviar() {
    if (!arquivo || ocupado) return
    setOcupado(true); setErro(''); setResultado(null)
    try {
      const fd = new FormData(); fd.append('arquivo', arquivo)
      const r = await api<ResultadoCarga>(URL + '/carga', { method: 'POST', body: fd })
      setResultado(r); setArquivo(null); aoCarregar(); toast.success('Departamentos carregados')
    } catch (e: any) { setErro(e.message) } finally { setOcupado(false) }
  }

  return (
    <div className="rounded-lg border bg-white p-3 space-y-2">
      <h2 className="text-sm font-semibold">Carregar departamentos</h2>
      <p className="text-xs text-muted-foreground">
        CSV ou XLSX com <code>codigo</code>, <code>nome</code> e <code>grupo</code> (Comercial, Logística ou Administrativo). Cria os novos e atualiza nome e grupo
        dos que já existem (pelo código); os que não estiverem no arquivo continuam como estão. Se uma linha for inválida, nada é gravado.
      </p>
      <input key={resultado ? 'ok' : 'novo'} type="file" accept=".csv,.xlsx,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
        onChange={e => setArquivo(e.target.files?.[0] ?? null)} className="text-sm" />
      <div className="flex flex-wrap gap-2">
        <Button size="sm" disabled={!arquivo || ocupado} onClick={enviar}><Upload className="h-4 w-4 mr-1.5" />{ocupado ? 'Enviando…' : 'Carregar'}</Button>
        <Button size="sm" variant="outline" onClick={baixarModelo}><Download className="h-4 w-4 mr-1.5" />Baixar modelo CSV</Button>
      </div>
      {erro && <pre className="whitespace-pre-wrap rounded-md border border-red-200 bg-red-50 p-2 text-xs text-red-700">{erro}</pre>}
      {resultado && (
        <p className="rounded-md border bg-gray-50 p-2 text-xs">
          {resultado.linhas} linha(s): {resultado.criados} criado(s), {resultado.atualizados} atualizado(s), {resultado.inalterados} inalterado(s).
        </p>
      )}
    </div>
  )
}

export default function AdmDepartamentos() {
  const { perfil, isAdmin } = useAuth()
  const editar = isAdmin || ['analista', 'controladoria'].includes(perfil ?? '')
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
      {editar && <BlocoCarga aoCarregar={recarregar} />}
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
