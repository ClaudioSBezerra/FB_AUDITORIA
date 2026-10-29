import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { toast } from 'sonner'
import { AlertTriangle, Download, Lock } from 'lucide-react'
import { api } from './util'

// Conferência entre origens (história 2.12): compara, por filial e conta, a soma
// bruta de duas origens de lançamentos na mesma competência. Só leitura.

interface OrigemInfo { origem: string; linhas: number; primeira_competencia: string; ultima_competencia: string; linhas_na_competencia: number }
interface Total { origem: string; total: number; contas: number; linhas: number }
interface Dif { filial: string; conta: string; descricao: string; valor_a: number; valor_b: number; diferenca: number }
interface SoUm { filial: string; conta: string; descricao: string; valor: number }
interface ResLinha { codigo: string; descricao: string; valor_a: number; valor_b: number; diferenca: number }
interface Resp {
  competencia: string; tolerancia: number; filial: string; a: Total; b: Total; fechada: boolean
  soma_no_dre: boolean; aviso?: string; pares_comparados: number; iguais: number
  total_diferencas: number; diferencas: Dif[]; total_so_em_a: number; so_em_a: SoUm[]
  total_so_em_b: number; so_em_b: SoUm[]; resumo_linhas: ResLinha[]
}

const num = (v: number) => v.toLocaleString('pt-BR', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
const LIMITE = 200

function mesAtual() {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
}

export default function Conferencia() {
  const [competencia, setCompetencia] = useState(mesAtual())
  const [a, setA] = useState('')
  const [b, setB] = useState('')
  const [filial, setFilial] = useState('')
  const [tol, setTol] = useState('0,01')
  const [pedido, setPedido] = useState<string | null>(null)
  const [pagina, setPagina] = useState(0)

  const origens = useQuery({
    queryKey: ['conferencia-origens', competencia],
    queryFn: () => api<OrigemInfo[]>(`/api/aud/conferencia/origens?competencia=${competencia}`),
  })
  const lista = origens.data ?? []

  const params = () => {
    const p = new URLSearchParams({ competencia, origem_a: a, origem_b: b, tolerancia: tol })
    if (filial.trim()) p.set('filial', filial.trim())
    return p
  }
  const r = useQuery({
    queryKey: ['conferencia', pedido, pagina], enabled: pedido !== null, retry: false,
    queryFn: () => api<Resp>(`/api/aud/conferencia?${pedido}&limite=${LIMITE}&offset=${pagina * LIMITE}`),
  })
  const d = r.data

  function conferir() {
    if (!a || !b) { toast.error('Escolha as duas origens'); return }
    if (a === b) { toast.error('As origens devem ser diferentes'); return }
    setPagina(0); setPedido(params().toString())
  }

  async function exportar() {
    if (!pedido) return
    try {
      const resp = await fetch(`/api/aud/conferencia?${pedido}&formato=csv`)
      if (!resp.ok) throw new Error((await resp.text()).trim() || `HTTP ${resp.status}`)
      const url = URL.createObjectURL(await resp.blob())
      const el = document.createElement('a')
      el.href = url; el.download = `conferencia-${competencia}.csv`; el.click()
      URL.revokeObjectURL(url)
    } catch (e: any) { toast.error(e.message) }
  }

  const campo = 'h-9 rounded-md border px-2 text-sm bg-white'
  const semOrigens = origens.isSuccess && lista.length < 2
  return (
    <div className="space-y-4 max-w-6xl">
      <div className="rounded-lg border bg-white p-4 space-y-3">
        <div className="flex flex-wrap items-end gap-3">
          <div><label className="text-xs text-muted-foreground block mb-1">Competência</label>
            <input type="month" className={campo} value={competencia} onChange={e => { setCompetencia(e.target.value); setPedido(null) }} /></div>
          <div><label className="text-xs text-muted-foreground block mb-1">Origem A</label>
            <select className={campo} value={a} onChange={e => setA(e.target.value)}>
              <option value="">Escolha…</option>
              {lista.map(o => <option key={o.origem} value={o.origem}>{o.origem} ({o.linhas_na_competencia} linhas no mês)</option>)}
            </select></div>
          <div><label className="text-xs text-muted-foreground block mb-1">Origem B</label>
            <select className={campo} value={b} onChange={e => setB(e.target.value)}>
              <option value="">Escolha…</option>
              {lista.map(o => <option key={o.origem} value={o.origem}>{o.origem} ({o.linhas_na_competencia} linhas no mês)</option>)}
            </select></div>
          <div><label className="text-xs text-muted-foreground block mb-1">Filial (opcional)</label>
            <input className={`${campo} w-24`} value={filial} onChange={e => setFilial(e.target.value)} /></div>
          <div><label className="text-xs text-muted-foreground block mb-1">Tolerância</label>
            <input className={`${campo} w-24`} value={tol} onChange={e => setTol(e.target.value)} inputMode="decimal" /></div>
          <button className="h-9 rounded-md bg-primary text-primary-foreground px-4 text-sm disabled:opacity-50" onClick={conferir} disabled={origens.isLoading}>Conferir</button>
          {d && <button className="h-9 rounded-md border bg-white px-3 text-sm inline-flex items-center gap-1.5" onClick={exportar}><Download className="h-4 w-4" />Exportar CSV</button>}
        </div>
        {semOrigens && <p className="text-xs text-muted-foreground">Há menos de duas origens de lançamentos carregadas; a conferência precisa de duas (por exemplo, <code>csv</code> e <code>csv.contas</code>, ou a planilha e o Oracle).</p>}
        <p className="text-xs text-muted-foreground">Compara a soma bruta de cada origem por filial e conta, sem passar pelo DRE e sem o sinal do Mapa. A filial <code>01</code> e a <code>1</code> são a mesma.</p>
      </div>

      {r.isError && <p className="text-sm text-red-600">{(r.error as Error).message}</p>}

      {d && (
        <>
          {d.soma_no_dre && (
            <div className="flex gap-2 rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900">
              <AlertTriangle className="h-4 w-4 mt-0.5 shrink-0" /><div>{d.aviso}</div>
            </div>
          )}
          {d.fechada && (
            <div className="flex gap-2 rounded-md border bg-gray-50 p-3 text-sm">
              <Lock className="h-4 w-4 mt-0.5 shrink-0" /><div>A competência {d.competencia} está fechada: os lançamentos não mudam sem reabrir.</div>
            </div>
          )}
          <div className="grid grid-cols-2 md:grid-cols-5 gap-3">
            <Cartao titulo={`A · ${d.a.origem}`} valor={num(d.a.total)} sub={`${d.a.contas} contas, ${d.a.linhas} linhas`} />
            <Cartao titulo={`B · ${d.b.origem}`} valor={num(d.b.total)} sub={`${d.b.contas} contas, ${d.b.linhas} linhas`} />
            <Cartao titulo="A − B" valor={num(Math.round((d.a.total - d.b.total) * 100) / 100)} sub="diferença dos totais" />
            <Cartao titulo="Pares comparados" valor={String(d.pares_comparados)} sub={`${d.iguais} dentro da tolerância de ${num(d.tolerancia)}`} />
            <Cartao titulo="Diferenças" valor={String(d.total_diferencas)} sub={`${d.total_so_em_a + d.total_so_em_b} só em um lado`} alerta={d.total_diferencas > 0 || d.total_so_em_a + d.total_so_em_b > 0} />
          </div>

          <Secao titulo={`Diferenças acima da tolerância (${d.total_diferencas})`}>
            <table className="w-full text-xs">
              <thead><tr className="border-b bg-gray-50 text-muted-foreground">
                <th className="text-left p-2">Filial</th><th className="text-left p-2">Conta</th><th className="text-left p-2">Descrição</th>
                <th className="text-right p-2">A</th><th className="text-right p-2">B</th><th className="text-right p-2">A − B</th></tr></thead>
              <tbody>
                {d.diferencas.map(x => (
                  <tr key={x.filial + '|' + x.conta} className="border-b last:border-0">
                    <td className="p-2">{x.filial}</td><td className="p-2">{x.conta}</td><td className="p-2">{x.descricao}</td>
                    <td className="p-2 text-right tabular-nums">{num(x.valor_a)}</td><td className="p-2 text-right tabular-nums">{num(x.valor_b)}</td>
                    <td className="p-2 text-right tabular-nums font-semibold text-red-700">{num(x.diferenca)}</td>
                  </tr>
                ))}
                {d.diferencas.length === 0 && <tr><td colSpan={6} className="p-4 text-center text-muted-foreground">{pagina > 0 ? 'Fim da lista.' : 'Nenhuma diferença acima da tolerância.'}</td></tr>}
              </tbody>
            </table>
            {d.total_diferencas > LIMITE && (
              <div className="flex items-center gap-2 p-2 text-xs">
                <button className="h-7 rounded border px-2 disabled:opacity-40" disabled={pagina === 0} onClick={() => setPagina(pagina - 1)}>Anterior</button>
                <span>{pagina * LIMITE + 1}–{Math.min((pagina + 1) * LIMITE, d.total_diferencas)} de {d.total_diferencas}</span>
                <button className="h-7 rounded border px-2 disabled:opacity-40" disabled={(pagina + 1) * LIMITE >= d.total_diferencas} onClick={() => setPagina(pagina + 1)}>Próxima</button>
              </div>
            )}
          </Secao>

          <div className="grid md:grid-cols-2 gap-4">
            <SoUmLado titulo={`Só em A · ${d.a.origem} (${d.total_so_em_a})`} itens={d.so_em_a} />
            <SoUmLado titulo={`Só em B · ${d.b.origem} (${d.total_so_em_b})`} itens={d.so_em_b} />
          </div>
          {(d.total_so_em_a > LIMITE || d.total_so_em_b > LIMITE || d.total_diferencas > LIMITE) && (
            <p className="text-xs text-muted-foreground">As listas mostram até {LIMITE} linhas por página; o CSV traz tudo.</p>
          )}

          <Secao titulo="Resumo por linha do DRE (pelo Mapa, valor bruto)">
            <table className="w-full text-xs">
              <thead><tr className="border-b bg-gray-50 text-muted-foreground">
                <th className="text-left p-2">Linha</th><th className="text-right p-2">A</th><th className="text-right p-2">B</th><th className="text-right p-2">A − B</th></tr></thead>
              <tbody>
                {d.resumo_linhas.map(x => (
                  <tr key={x.codigo || 'sem'} className="border-b last:border-0">
                    <td className="p-2">{x.descricao}</td>
                    <td className="p-2 text-right tabular-nums">{num(x.valor_a)}</td><td className="p-2 text-right tabular-nums">{num(x.valor_b)}</td>
                    <td className={`p-2 text-right tabular-nums ${Math.abs(x.diferenca) > d.tolerancia ? 'font-semibold text-red-700' : ''}`}>{num(x.diferenca)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </Secao>
        </>
      )}
    </div>
  )
}

function Cartao({ titulo, valor, sub, alerta }: { titulo: string; valor: string; sub: string; alerta?: boolean }) {
  return (
    <div className={`rounded-lg border bg-white p-3 ${alerta ? 'border-amber-300' : ''}`}>
      <div className="text-xs text-muted-foreground truncate" title={titulo}>{titulo}</div>
      <div className="text-lg font-semibold tabular-nums">{valor}</div>
      <div className="text-[11px] text-muted-foreground">{sub}</div>
    </div>
  )
}

function Secao({ titulo, children }: { titulo: string; children: React.ReactNode }) {
  return (
    <div className="rounded-lg border bg-white overflow-x-auto">
      <h3 className="text-sm font-semibold p-3 border-b">{titulo}</h3>
      {children}
    </div>
  )
}

function SoUmLado({ titulo, itens }: { titulo: string; itens: SoUm[] }) {
  return (
    <Secao titulo={titulo}>
      <table className="w-full text-xs">
        <thead><tr className="border-b bg-gray-50 text-muted-foreground">
          <th className="text-left p-2">Filial</th><th className="text-left p-2">Conta</th><th className="text-left p-2">Descrição</th><th className="text-right p-2">Valor</th></tr></thead>
        <tbody>
          {itens.map(x => (
            <tr key={x.filial + '|' + x.conta} className="border-b last:border-0">
              <td className="p-2">{x.filial}</td><td className="p-2">{x.conta}</td><td className="p-2">{x.descricao}</td>
              <td className="p-2 text-right tabular-nums">{num(x.valor)}</td>
            </tr>
          ))}
          {itens.length === 0 && <tr><td colSpan={4} className="p-4 text-center text-muted-foreground">Nenhuma.</td></tr>}
        </tbody>
      </table>
    </Secao>
  )
}
