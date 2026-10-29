import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { AlertTriangle, Download, Lock, Upload } from 'lucide-react'
import { useAuth } from '@/contexts/AuthContext'
import { api, MESES } from './util'

// Comparativo de fechamento (história 2.18): a planilha de análise do cliente fica
// gravada e é comparada, linha a linha ou conta a conta, com o que o sistema tem.
// Só leitura (a carga da planilha exige a capacidade de carregar).

interface Escopo { ano: number; filiais: string[]; meses: string[] }
interface CargaResumo { id: number; arquivo: string; ano: number; visao: string; filial: string; quando: string; usuario: string }
interface Periodos { contas: Escopo[]; linhas: Escopo[]; cargas: CargaResumo[] }
interface Item {
  chave: string; descricao: string; status: Status; planilha: number | null; sistema: number | null
  diferenca: number; diferenca_pct?: number; subtotal?: boolean; obs?: string
}
type Status = 'OK' | 'DIVERGE' | 'SO_PLANILHA' | 'SO_SISTEMA'
interface Resp {
  competencia: string; nivel: string; filial: string; origem: string; tolerancia: number; fechada: boolean
  carga: { id: number; arquivo: string; quando: string; usuario: string } | null
  resumo: { ok: number; diverge: number; so_planilha: number; so_sistema: number; sinal_invertido?: number; soma_diferencas: number; total_planilha: number; total_sistema: number; totais_validos: boolean }
  total: number; linhas: Item[]; avisos: string[]
}
interface ResultadoCarga {
  ano: number; visao: string; filial: string; contas: number; linhas: number; meses: number; filiais: number; ignoradas: number; avisos: string[]
}

const num = (v: number) => v.toLocaleString('pt-BR', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
const LIMITE = 500
const ROTULO: Record<Status, string> = { OK: 'OK', DIVERGE: 'Diverge', SO_PLANILHA: 'Só planilha', SO_SISTEMA: 'Só sistema' }
const COR: Record<Status, string> = {
  OK: 'bg-green-100 text-green-800',
  DIVERGE: 'bg-red-100 text-red-800',
  SO_PLANILHA: 'bg-amber-100 text-amber-900',
  SO_SISTEMA: 'bg-blue-100 text-blue-800',
}
// Filtro da lista: divergentes (tudo, menos OK) por padrão.
const FILTROS: { id: string; rotulo: string; status: string }[] = [
  { id: 'problemas', rotulo: 'Divergentes (sem OK)', status: 'DIVERGE,SO_PLANILHA,SO_SISTEMA' },
  { id: 'todos', rotulo: 'Todas', status: '' },
  { id: 'DIVERGE', rotulo: 'Só Diverge', status: 'DIVERGE' },
  { id: 'SO_PLANILHA', rotulo: 'Só planilha', status: 'SO_PLANILHA' },
  { id: 'SO_SISTEMA', rotulo: 'Só sistema', status: 'SO_SISTEMA' },
  { id: 'OK', rotulo: 'Só OK', status: 'OK' },
]

export default function Comparativo() {
  const { perfil, isAdmin } = useAuth()
  const podeCarregar = isAdmin || ['analista', 'controladoria'].includes(perfil ?? '')
  const qc = useQueryClient()
  const periodos = useQuery({ queryKey: ['comparativo-periodos'], queryFn: () => api<Periodos>('/api/aud/comparativo/periodos') })

  const [nivel, setNivel] = useState<'linha' | 'conta'>('linha')
  const [ano, setAno] = useState('')
  const [mes, setMes] = useState('')
  const [filial, setFilial] = useState('')
  const [tol, setTol] = useState('1,00')
  const [origem, setOrigem] = useState('')
  const [filtro, setFiltro] = useState('problemas')
  const [busca, setBusca] = useState('')
  const [pedido, setPedido] = useState<string | null>(null)
  const [pagina, setPagina] = useState(0)

  const p = periodos.data
  const escopos = nivel === 'conta' ? (p?.contas ?? []) : (p?.linhas ?? [])
  const anos = Array.from(new Set([...(p?.contas ?? []), ...(p?.linhas ?? [])].map(e => e.ano))).sort((a, b) => b - a)
  const anoSel = ano || String(anos[0] ?? new Date().getFullYear())
  const esc = escopos.find(e => String(e.ano) === anoSel)
  const mesesDoAno = Array.from(new Set([...(p?.contas ?? []), ...(p?.linhas ?? [])].filter(e => String(e.ano) === anoSel).flatMap(e => e.meses))).sort()
  const mesSel = mes || (mesesDoAno.length ? String(Number(mesesDoAno[mesesDoAno.length - 1].slice(5))) : String(new Date().getMonth() + 1))
  const filiais = esc?.filiais.filter(f => f !== '') ?? []

  const status = FILTROS.find(f => f.id === filtro)?.status ?? ''
  const montar = (extra?: Record<string, string>) => {
    const q = new URLSearchParams({ ano: anoSel, mes: mesSel, nivel, tolerancia: tol })
    if (filial.trim()) q.set('filial', filial.trim())
    if (nivel === 'conta' && origem.trim()) q.set('origem', origem.trim())
    for (const [k, v] of Object.entries(extra ?? {})) q.set(k, v)
    return q
  }
  const r = useQuery({
    queryKey: ['comparativo', pedido, filtro, pagina], enabled: pedido !== null, retry: false,
    queryFn: () => {
      const q = new URLSearchParams(pedido!)
      if (status) q.set('status', status)
      q.set('limite', String(LIMITE)); q.set('offset', String(pagina * LIMITE))
      return api<Resp>(`/api/aud/comparativo?${q}`)
    },
  })
  const d = r.data

  function comparar() {
    if (nivel === 'conta' && !filial.trim()) { toast.error('Informe a filial para comparar por conta'); return }
    setPagina(0); setPedido(montar().toString())
  }

  async function exportar() {
    if (!pedido) return
    try {
      const q = new URLSearchParams(pedido); q.set('formato', 'csv')
      const resp = await fetch(`/api/aud/comparativo?${q}`)
      if (!resp.ok) throw new Error((await resp.text()).trim() || `HTTP ${resp.status}`)
      const url = URL.createObjectURL(await resp.blob())
      const el = document.createElement('a')
      el.href = url; el.download = `comparativo-${nivel}-${anoSel}-${mesSel.padStart(2, '0')}.csv`; el.click()
      URL.revokeObjectURL(url)
    } catch (e: any) { toast.error(e.message) }
  }

  const campo = 'h-9 rounded-md border px-2 text-sm bg-white'
  const termo = busca.trim().toLowerCase()
  const linhas = (d?.linhas ?? []).filter(x => !termo || x.chave.toLowerCase().includes(termo) || x.descricao.toLowerCase().includes(termo))
  const semCarga = periodos.isSuccess && anos.length === 0

  return (
    <div className="space-y-4 max-w-6xl">
      {podeCarregar && <BlocoCarga aoCarregar={() => { qc.invalidateQueries({ queryKey: ['comparativo-periodos'] }); setPedido(null) }} />}

      <div className="rounded-lg border bg-white p-4 space-y-3">
        <div className="flex flex-wrap items-end gap-3">
          <div><label className="text-xs text-muted-foreground block mb-1">Comparar</label>
            <div className="inline-flex rounded-md border overflow-hidden h-9 text-sm">
              {(['linha', 'conta'] as const).map(n => (
                <button key={n} className={`px-3 ${nivel === n ? 'bg-primary text-primary-foreground' : 'bg-white'}`}
                  onClick={() => { setNivel(n); setFilial(''); setPedido(null) }}>{n === 'linha' ? 'Por linha' : 'Por conta'}</button>
              ))}
            </div></div>
          <div><label className="text-xs text-muted-foreground block mb-1">Ano</label>
            <select className={campo} value={anoSel} onChange={e => { setAno(e.target.value); setMes(''); setPedido(null) }}>
              {anos.length === 0 && <option value={anoSel}>{anoSel}</option>}
              {anos.map(a => <option key={a} value={a}>{a}</option>)}
            </select></div>
          <div><label className="text-xs text-muted-foreground block mb-1">Mês</label>
            <select className={campo} value={mesSel} onChange={e => { setMes(e.target.value); setPedido(null) }}>
              {MESES.map((m, i) => <option key={m} value={i + 1}>{m}</option>)}
            </select></div>
          <div><label className="text-xs text-muted-foreground block mb-1">Filial{nivel === 'linha' ? ' (opcional)' : ''}</label>
            <input className={`${campo} w-28`} list="comparativo-filiais" value={filial} onChange={e => { setFilial(e.target.value); setPedido(null) }}
              placeholder={nivel === 'linha' ? 'da planilha' : ''} />
            <datalist id="comparativo-filiais">{filiais.map(f => <option key={f} value={f} />)}</datalist></div>
          {nivel === 'conta' && (
            <div><label className="text-xs text-muted-foreground block mb-1">Origem (opcional)</label>
              <input className={`${campo} w-28`} value={origem} onChange={e => { setOrigem(e.target.value); setPedido(null) }} placeholder="todas" /></div>
          )}
          <div><label className="text-xs text-muted-foreground block mb-1">Tolerância (R$)</label>
            <input className={`${campo} w-24`} value={tol} onChange={e => setTol(e.target.value)} inputMode="decimal" /></div>
          <button className="h-9 rounded-md bg-primary text-primary-foreground px-4 text-sm disabled:opacity-50" onClick={comparar} disabled={periodos.isLoading || semCarga}>Comparar</button>
          {d && <button className="h-9 rounded-md border bg-white px-3 text-sm inline-flex items-center gap-1.5" onClick={exportar}><Download className="h-4 w-4" />Exportar CSV</button>}
        </div>
        {semCarga && <p className="text-xs text-muted-foreground">Nenhuma planilha carregada ainda.{podeCarregar ? ' Envie a planilha de análise acima.' : ' Peça a quem carrega os dados para enviar a planilha.'}</p>}
        {p && p.cargas.length > 0 && (
          <p className="text-xs text-muted-foreground">Última planilha: {p.cargas[0].arquivo}, ano {p.cargas[0].ano}, em {p.cargas[0].quando}{p.cargas[0].usuario ? ` por ${p.cargas[0].usuario}` : ''}.</p>
        )}
        <ComoFunciona />
      </div>

      {r.isError && <p className="text-sm text-red-600">{(r.error as Error).message}</p>}

      {d && (
        <>
          {d.avisos.map((a, i) => (
            <div key={i} className="flex gap-2 rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900">
              <AlertTriangle className="h-4 w-4 mt-0.5 shrink-0" /><div>{a}</div>
            </div>
          ))}
          {d.fechada && (
            <div className="flex gap-2 rounded-md border bg-gray-50 p-3 text-sm">
              <Lock className="h-4 w-4 mt-0.5 shrink-0" /><div>A competência {d.competencia} está fechada: o lado do sistema {d.nivel === 'linha' ? 'vem do fechamento' : 'não muda sem reabrir'}.</div>
            </div>
          )}
          {d.carga && <p className="text-xs text-muted-foreground">Planilha: {d.carga.arquivo}, enviada em {d.carga.quando}{d.carga.usuario ? ` por ${d.carga.usuario}` : ''}. Filial {d.filial || '—'}.</p>}

          <div className="grid grid-cols-2 md:grid-cols-5 gap-3">
            <Cartao titulo="OK" valor={d.resumo.ok} sub={`dentro de ${num(d.tolerancia)}${d.resumo.sinal_invertido ? ` · ${d.resumo.sinal_invertido} com sinal invertido` : ''}`} cor="text-green-700" onClick={() => { setFiltro('OK'); setPagina(0) }} />
            <Cartao titulo="Diverge" valor={d.resumo.diverge} sub="acima da tolerância" cor="text-red-700" onClick={() => { setFiltro('DIVERGE'); setPagina(0) }} />
            <Cartao titulo="Só planilha" valor={d.resumo.so_planilha} sub="sem par no sistema" cor="text-amber-700" onClick={() => { setFiltro('SO_PLANILHA'); setPagina(0) }} />
            <Cartao titulo="Só sistema" valor={d.resumo.so_sistema} sub="sem par na planilha" cor="text-blue-700" onClick={() => { setFiltro('SO_SISTEMA'); setPagina(0) }} />
            <Cartao titulo="Soma das diferenças" valor={num(d.resumo.soma_diferencas)} sub={d.resumo.totais_validos ? `planilha ${num(d.resumo.total_planilha)} · sistema ${num(d.resumo.total_sistema)}` : 'em valor absoluto'} />
          </div>

          <div className="rounded-lg border bg-white overflow-x-auto">
            <div className="flex flex-wrap items-center gap-3 p-3 border-b">
              <h3 className="text-sm font-semibold mr-auto">{d.nivel === 'linha' ? 'Linhas do DRE' : 'Contas'} · {d.competencia} ({d.total})</h3>
              <select className={campo} value={filtro} onChange={e => { setFiltro(e.target.value); setPagina(0) }}>
                {FILTROS.map(f => <option key={f.id} value={f.id}>{f.rotulo}</option>)}
              </select>
              <input className={`${campo} w-48`} placeholder="Buscar nesta página…" value={busca} onChange={e => setBusca(e.target.value)} />
            </div>
            <table className="w-full text-xs">
              <thead><tr className="border-b bg-gray-50 text-muted-foreground">
                <th className="text-left p-2">Status</th><th className="text-left p-2">{d.nivel === 'linha' ? 'Linha' : 'Conta'}</th><th className="text-left p-2">Descrição</th>
                <th className="text-right p-2">Planilha</th><th className="text-right p-2">Sistema</th><th className="text-right p-2">Planilha − sistema</th><th className="text-right p-2">%</th></tr></thead>
              <tbody>
                {linhas.map(x => (
                  <tr key={x.chave} className={`border-b last:border-0 ${x.subtotal ? 'font-semibold' : ''}`}>
                    <td className="p-2"><span className={`rounded px-1.5 py-0.5 text-[11px] ${COR[x.status]}`}>{ROTULO[x.status]}</span>{x.obs && <span className="ml-1.5 text-[11px] text-muted-foreground">({x.obs})</span>}</td>
                    <td className="p-2 font-mono">{x.chave}</td><td className="p-2">{x.descricao}</td>
                    <td className="p-2 text-right tabular-nums">{x.planilha == null ? '—' : num(x.planilha)}</td>
                    <td className="p-2 text-right tabular-nums">{x.sistema == null ? '—' : num(x.sistema)}</td>
                    <td className={`p-2 text-right tabular-nums ${x.status === 'OK' ? '' : 'font-semibold text-red-700'}`}>{num(x.diferenca)}</td>
                    <td className="p-2 text-right tabular-nums text-muted-foreground">{x.diferenca_pct == null ? '' : `${num(x.diferenca_pct)}%`}</td>
                  </tr>
                ))}
                {linhas.length === 0 && <tr><td colSpan={7} className="p-4 text-center text-muted-foreground">{pagina > 0 ? 'Fim da lista.' : 'Nada a mostrar com este filtro.'}</td></tr>}
              </tbody>
            </table>
            {d.total > LIMITE && (
              <div className="flex items-center gap-2 p-2 text-xs">
                <button className="h-7 rounded border px-2 disabled:opacity-40" disabled={pagina === 0} onClick={() => setPagina(pagina - 1)}>Anterior</button>
                <span>{pagina * LIMITE + 1}–{Math.min((pagina + 1) * LIMITE, d.total)} de {d.total}</span>
                <button className="h-7 rounded border px-2 disabled:opacity-40" disabled={(pagina + 1) * LIMITE >= d.total} onClick={() => setPagina(pagina + 1)}>Próxima</button>
              </div>
            )}
          </div>
        </>
      )}
    </div>
  )
}

function Cartao({ titulo, valor, sub, cor, onClick }: { titulo: string; valor: number | string; sub: string; cor?: string; onClick?: () => void }) {
  const corpo = (
    <>
      <div className="text-xs text-muted-foreground truncate" title={titulo}>{titulo}</div>
      <div className={`text-lg font-semibold tabular-nums ${cor ?? ''}`}>{valor}</div>
      <div className="text-[11px] text-muted-foreground truncate" title={sub}>{sub}</div>
    </>
  )
  return onClick
    ? <button className="rounded-lg border bg-white p-3 text-left hover:bg-gray-50" onClick={onClick}>{corpo}</button>
    : <div className="rounded-lg border bg-white p-3">{corpo}</div>
}

function ComoFunciona() {
  return (
    <details className="text-xs text-muted-foreground">
      <summary className="cursor-pointer text-foreground">Como funciona</summary>
      <ul className="list-disc pl-5 mt-2 space-y-1">
        <li><b>Lado da planilha:</b> o que veio da planilha de análise enviada: a aba <code>MENSAL</code> (linhas do DRE, uma filial por vez) e a aba <code>Real_AA</code> (soma por conta, mês e loja).</li>
        <li><b>Lado do sistema:</b> por linha, o DRE do sistema (mês fechado vem do fechamento, mês aberto é calculado na hora); por conta, a soma bruta dos lançamentos da filial no mês (sem o sinal do Mapa), de todas as origens ou só da origem escolhida.</li>
        <li><b>OK ou Diverge:</b> a diferença é planilha menos sistema. Até a tolerância (em R$) é OK; acima, Diverge. O percentual é só informativo. Só planilha e Só sistema são linhas ou contas sem par do outro lado. Por conta, se os dois lados têm o mesmo valor com o sinal trocado (o sistema guarda o valor bruto e a planilha já traz o sinal), conta como OK e aparece como "sinal invertido".</li>
        <li><b>A planilha fica gravada:</b> enviar de novo o mesmo ano substitui o que estava gravado daquele ano (as contas de todas as lojas e as linhas da filial da planilha).</li>
        <li><b>Não entra no DRE:</b> os dados da planilha ficam em tabelas à parte e nunca são somados aos lançamentos.</li>
        <li>A filial <code>01</code> e a <code>1</code> são a mesma. Por linha, sem filial informada, vale a filial da última planilha.</li>
      </ul>
    </details>
  )
}

function BlocoCarga({ aoCarregar }: { aoCarregar: () => void }) {
  const [arquivo, setArquivo] = useState<File | null>(null)
  const [ano, setAno] = useState('')
  const [ocupado, setOcupado] = useState(false)
  const [resultado, setResultado] = useState<ResultadoCarga | null>(null)
  const [erro, setErro] = useState('')

  async function enviar() {
    if (!arquivo || ocupado) return
    setOcupado(true); setErro(''); setResultado(null)
    try {
      const fd = new FormData(); fd.append('arquivo', arquivo)
      if (ano.trim()) fd.append('ano', ano.trim())
      const res = await api<ResultadoCarga>('/api/aud/comparativo/carga', { method: 'POST', body: fd })
      setResultado(res); setArquivo(null); aoCarregar(); toast.success('Planilha carregada')
    } catch (e: any) { setErro(e.message) } finally { setOcupado(false) }
  }

  return (
    <div className="rounded-lg border bg-white p-4 space-y-3">
      <h2 className="text-sm font-semibold">Carregar a planilha de análise</h2>
      <p className="text-xs text-muted-foreground">
        XLSX da análise de fechamento, com as abas <code>MENSAL</code> e <code>Real_AA</code> do ano. O ano vem da aba MENSAL (célula C2), se você não informar.
        Reenviar o mesmo ano substitui o que estava gravado. Os dados da planilha não entram no DRE.
      </p>
      <div className="flex flex-wrap items-end gap-3">
        <input type="file" accept=".xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
          onChange={e => setArquivo(e.target.files?.[0] ?? null)} className="text-sm" />
        <div><label className="text-xs text-muted-foreground block mb-1">Ano (opcional)</label>
          <input className="h-9 w-24 rounded-md border px-2 text-sm bg-white" value={ano} onChange={e => setAno(e.target.value)} inputMode="numeric" /></div>
        <button className="h-9 rounded-md bg-primary text-primary-foreground px-4 text-sm inline-flex items-center gap-1.5 disabled:opacity-50" disabled={!arquivo || ocupado} onClick={enviar}>
          <Upload className="h-4 w-4" />{ocupado ? 'Enviando…' : 'Carregar'}
        </button>
      </div>
      {erro && <pre className="whitespace-pre-wrap rounded-md border border-red-200 bg-red-50 p-2 text-xs text-red-700">{erro}</pre>}
      {resultado && (
        <div className="space-y-2">
          <p className="rounded-md border bg-gray-50 p-2 text-xs">
            Ano {resultado.ano}: {resultado.contas} contas, {resultado.linhas} linhas do DRE, {resultado.meses} mês(es), {resultado.filiais} filial(is); {resultado.ignoradas} linha(s) da aba Real ignoradas
            {resultado.visao ? `; visão "${resultado.visao}"` : ''}{resultado.filial ? `, filial ${resultado.filial}` : ''}.
          </p>
          {resultado.avisos.map((a, i) => (
            <div key={i} className="flex gap-2 rounded-md border border-amber-300 bg-amber-50 p-2 text-xs text-amber-900">
              <AlertTriangle className="h-4 w-4 shrink-0" /><div>{a}</div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
