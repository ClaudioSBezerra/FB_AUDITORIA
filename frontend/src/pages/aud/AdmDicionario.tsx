import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Upload } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useAuth } from '@/contexts/AuthContext'
import { api } from './util'

interface Conta {
  conta: string; descricao: string; grupo: string
  departamento_id?: string; departamento_codigo?: string; departamento_nome?: string
  origem?: string; padrao?: string
}
interface Lista { total: number; pagina: number; limite: number; contas: Conta[]; pendentes_total: number; pendentes: Conta[] }
interface Depto { id: string; codigo: string; nome: string }
interface Resultado {
  linhas: number; novas: number; atualizadas: number; vinculos_grupo: number; vinculos_informados: number
  mantidos_manual: number; vinculos_removidos: number; sem_departamento: number
}

const LIMITE = 50
const headers = { 'Content-Type': 'application/json' }

export default function AdmDicionario() {
  const { perfil, isAdmin } = useAuth()
  const editar = isAdmin || ['analista', 'controladoria'].includes(perfil ?? '')
  const qc = useQueryClient()
  const [pagina, setPagina] = useState(1)
  const [busca, setBusca] = useState('')
  const [arquivo, setArquivo] = useState<File | null>(null)
  const [ocupado, setOcupado] = useState(false)
  const [resultado, setResultado] = useState<Resultado | null>(null)
  const [erroCarga, setErroCarga] = useState('')

  const lista = useQuery({
    queryKey: ['aud-dicionario', pagina, busca],
    queryFn: () => api<Lista>(`/api/aud/dicionario?pagina=${pagina}&limite=${LIMITE}&busca=${encodeURIComponent(busca)}`),
  })
  const deptos = useQuery({ queryKey: ['aud-departamentos'], queryFn: () => api<Depto[]>('/api/aud/departamentos') })
  const recarregar = () => qc.invalidateQueries({ queryKey: ['aud-dicionario'] })

  async function enviar() {
    if (!arquivo || ocupado) return
    setOcupado(true); setErroCarga(''); setResultado(null)
    try {
      const fd = new FormData(); fd.append('arquivo', arquivo)
      const r = await api<Resultado>('/api/aud/dicionario/carga', { method: 'POST', body: fd })
      setResultado(r); setArquivo(null); setPagina(1); recarregar()
      toast.success(`${r.linhas} contas carregadas`)
    } catch (e: any) { setErroCarga(e.message) } finally { setOcupado(false) }
  }

  async function corrigir(c: Conta, departamentoId: string) {
    if (ocupado) return
    setOcupado(true)
    try {
      await api('/api/aud/dicionario/conta', {
        method: 'PUT', headers, body: JSON.stringify({ conta: c.conta, departamento_id: departamentoId }),
      })
      toast.success('Departamento atualizado'); recarregar()
    } catch (e: any) { toast.error(e.message) } finally { setOcupado(false) }
  }

  const seletor = (c: Conta) => (
    <select className="h-7 rounded-md border px-1 text-xs disabled:opacity-50" disabled={ocupado}
      value={c.origem === 'manual' || !c.padrao ? (c.departamento_id ?? '') : ''}
      onChange={e => corrigir(c, e.target.value)}>
      <option value="">{c.origem === 'manual' ? 'Voltar a derivar do grupo' : (c.departamento_id ? 'Derivado do grupo' : 'Sem departamento')}</option>
      {(deptos.data ?? []).map(d => <option key={d.id} value={d.id}>{d.codigo} — {d.nome}</option>)}
    </select>
  )

  const total = lista.data?.total ?? 0
  const paginas = Math.max(1, Math.ceil(total / LIMITE))

  return (
    <div className="max-w-5xl space-y-4">
      {editar && (
        <div className="rounded-lg border bg-white p-4 space-y-3">
          <h2 className="text-sm font-semibold">Carregar dicionário (CSV ou XLSX)</h2>
          <p className="text-xs text-muted-foreground">
            Colunas: <code>conta</code> e <code>grupo</code> (obrigatórias); <code>descrição</code> e <code>departamento</code> (código do departamento) opcionais.
            Sem departamento no arquivo, a conta fica no departamento cujo código é o grupo dela. Reenviar atualiza descrição, grupo e departamento;
            correções feitas conta a conta não são desfeitas por reenvio que só deriva do grupo. A carga é tudo-ou-nada.
          </p>
          <input type="file" accept=".csv,.xlsx,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
            onChange={e => setArquivo(e.target.files?.[0] ?? null)} className="text-sm" />
          <div><Button size="sm" disabled={!arquivo || ocupado} onClick={enviar}><Upload className="h-4 w-4 mr-1.5" />{ocupado ? 'Enviando…' : 'Carregar'}</Button></div>
          {erroCarga && <pre className="whitespace-pre-wrap rounded-md border border-red-200 bg-red-50 p-2 text-xs text-red-700">{erroCarga}</pre>}
          {resultado && (
            <p className="rounded-md border bg-gray-50 p-2 text-xs">
              {resultado.linhas} contas: {resultado.novas} novas, {resultado.atualizadas} atualizadas. Vínculos: {resultado.vinculos_grupo} pelo grupo,
              {' '}{resultado.vinculos_informados} informados no arquivo, {resultado.mantidos_manual} mantidos, {resultado.vinculos_removidos} removidos.
              {' '}Sem departamento: {resultado.sem_departamento}.
            </p>
          )}
        </div>
      )}

      {(lista.data?.pendentes_total ?? 0) > 0 && (
        <div className="rounded-lg border border-amber-300 bg-amber-50 p-3 space-y-2">
          <h2 className="text-sm font-semibold">Contas sem departamento ({lista.data!.pendentes_total})</h2>
          <table className="w-full text-sm">
            <tbody>
              {lista.data!.pendentes.map(c => (
                <tr key={c.conta} className="border-b border-amber-200 last:border-0">
                  <td className="p-1.5 font-medium">{c.conta}</td><td className="p-1.5">{c.descricao}</td>
                  <td className="p-1.5">Grupo {c.grupo || '—'}</td>
                  {editar && <td className="p-1.5 text-right">{seletor(c)}</td>}
                </tr>
              ))}
            </tbody>
          </table>
          {lista.data!.pendentes_total > lista.data!.pendentes.length && (
            <p className="text-xs text-muted-foreground">Mostrando as primeiras {lista.data!.pendentes.length}.</p>
          )}
        </div>
      )}

      <div className="space-y-2">
        <input className="h-8 w-64 rounded-md border px-2 text-sm" placeholder="Buscar conta ou descrição" value={busca}
          onChange={e => { setBusca(e.target.value); setPagina(1) }} />
        {lista.isLoading && <p className="text-sm text-muted-foreground">Carregando...</p>}
        {lista.isError && <p className="text-sm text-red-600">Erro ao carregar o dicionário: {(lista.error as Error).message}</p>}
        {lista.data && (
          <div className="rounded-lg border bg-white overflow-hidden">
            <table className="w-full text-sm">
              <thead><tr className="border-b bg-gray-50 text-xs text-muted-foreground">
                <th className="text-left p-2.5 font-medium">Conta</th><th className="text-left p-2.5 font-medium">Descrição</th>
                <th className="text-left p-2.5 font-medium">Grupo</th><th className="text-left p-2.5 font-medium">Departamento</th>
                <th className="text-left p-2.5 font-medium">Vínculo</th>{editar && <th className="p-2.5" />}</tr></thead>
              <tbody>
                {lista.data.contas.map(c => (
                  <tr key={c.conta} className="border-b last:border-0">
                    <td className="p-2.5 font-medium">{c.conta}</td><td className="p-2.5">{c.descricao}</td><td className="p-2.5">{c.grupo || '—'}</td>
                    <td className="p-2.5">{c.departamento_codigo ? `${c.departamento_codigo} — ${c.departamento_nome}` : '—'}</td>
                    <td className="p-2.5">{c.origem ? (c.origem === 'manual' ? 'manual' : 'grupo') + (c.padrao ? ` (${c.padrao})` : '') : '—'}</td>
                    {editar && <td className="p-2.5 text-right">{seletor(c)}</td>}
                  </tr>
                ))}
                {lista.data.contas.length === 0 && (
                  <tr><td className="p-2.5 text-muted-foreground" colSpan={editar ? 6 : 5}>Nenhuma conta no dicionário.</td></tr>
                )}
              </tbody>
            </table>
          </div>
        )}
        <div className="flex items-center gap-2 text-xs">
          <button className="h-7 rounded-md border px-2 disabled:opacity-50" disabled={pagina <= 1} onClick={() => setPagina(pagina - 1)}>Anterior</button>
          <span>Página {pagina} de {paginas} ({total} contas)</span>
          <button className="h-7 rounded-md border px-2 disabled:opacity-50" disabled={pagina >= paginas} onClick={() => setPagina(pagina + 1)}>Próxima</button>
        </div>
      </div>
    </div>
  )
}
