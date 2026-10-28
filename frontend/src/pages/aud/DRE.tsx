import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Lock, AlertTriangle } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { useAuth } from '@/contexts/AuthContext'
import { api, brl, pctFmt, MESES } from './util'
import { DetalheLinhaDialog } from './DetalheDRE'

interface Linha {
  codigo: string; descricao: string; nivel: number; tipo: 'conta' | 'subtotal'
  realizado: number; orcado: number; ano_anterior: number
  var_orcado_pct: number | null; var_ano_pct: number | null; pct_faturamento: number | null
}
interface DreResp {
  congelado: boolean; versao: number; fechamento_ativo: boolean; fechamento_versao: number; visao: string; linhas: Linha[]
  nao_mapeado: { conta: string; valor: number }[]
  pct_status: 'ok' | 'sem_papel' | 'zero'; aviso_faturamento: string
}
interface Atualizacao {
  hora: string; status: 'ok' | 'sem_fonte' | 'recusada' | 'erro' | 'parcial'; mensagem: string
  fontes: { fonte: string; status: string; linhas: number; mensagem?: string }[]
}
interface Lanc {
  id: number; competencia: string; filial: string; centro_custo: string; canal: string
  conta: string; conta_desc: string; valor: number; linha: string
  tipo_doc: string; doc_ref: string; historico: string
}

const hoje = new Date()

function Var({ v }: { v: number | null }) {
  if (v == null) return <span className="text-muted-foreground">—</span>
  return <span className={v >= 0 ? 'text-emerald-700' : 'text-red-600'}>{pctFmt(v)}</span>
}

export default function DRE() {
  const { perfil, isAdmin } = useAuth()
  const qc = useQueryClient()
  const [ano, setAno] = useState(hoje.getFullYear())
  const [mes, setMes] = useState(hoje.getMonth() + 1)
  const [visao, setVisao] = useState<'mensal' | 'acumulada'>('mensal')
  const [filial, setFilial] = useState('')
  const [drill, setDrill] = useState<Linha | null>(null)
  const [atualizando, setAtualizando] = useState(false)
  const [atual, setAtual] = useState<Atualizacao | null>(null)
  const podeFechar = isAdmin || perfil === 'controladoria'
  // Gestor segue sem o detalhe por conta e lançamento até o escopo por departamento (4.7).
  const podeDetalhar = isAdmin || ['controladoria', 'auditoria', 'diretoria'].includes(perfil ?? '')

  const q = `ano=${ano}&mes=${mes}&visao=${visao}&filial=${encodeURIComponent(filial)}`
  const dre = useQuery({ queryKey: ['dre', q], queryFn: () => api<DreResp>(`/api/aud/dre?${q}`) })
  const filiais = useQuery({ queryKey: ['dre-filiais'], queryFn: () => api<string[]>('/api/aud/dre/filiais') })
  const lancs = useQuery({
    queryKey: ['dre-lanc', q, drill?.codigo],
    enabled: !!drill && !podeDetalhar,
    queryFn: () => api<{ lancamentos: Lanc[]; truncado: boolean; aviso?: string }>(`/api/aud/dre/lancamentos?${q}&linha=${drill!.codigo}`),
  })

  async function fechar() {
    const observacao = window.prompt(`Fechar ${MESES[mes - 1]}/${ano} para o grupo inteiro (todas as filiais, independentemente do filtro de filial)? Observação (opcional):`)
    if (observacao === null) return
    try {
      const r = await api<{ versao: number }>('/api/aud/dre/fechamentos', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ano, mes, observacao }),
      })
      toast.success(`Período fechado — versão ${r.versao}`)
      qc.invalidateQueries({ queryKey: ['dre'] })
      qc.invalidateQueries({ queryKey: ['dre-fechamentos'] })
    } catch (e: any) { toast.error(e.message) }
  }

  async function reabrir() {
    const motivo = window.prompt(`Reabrir ${MESES[mes - 1]}/${ano}? Informe o motivo (obrigatório, até 500 caracteres):`)
    if (motivo === null) return
    if (motivo.length > 500) { toast.error('Motivo: no máximo 500 caracteres'); return }
    if (!motivo.trim()) { toast.error('Informe o motivo da reabertura'); return }
    try {
      await api('/api/aud/dre/fechamentos/reabrir', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ano, mes, motivo }),
      })
      toast.success('Período reaberto — o próximo fechamento cria nova versão')
      qc.invalidateQueries({ queryKey: ['dre'] })
      qc.invalidateQueries({ queryKey: ['dre-fechamentos'] })
    } catch (e: any) { toast.error(e.message) }
  }

  async function atualizarDados() {
    if (atualizando) return
    setAtualizando(true)
    try {
      const r = await fetch('/api/aud/dados/atualizar', { method: 'POST' })
      const txt = await r.text()
      let res: Atualizacao | null = null
      try { res = JSON.parse(txt) as Atualizacao } catch { /* resposta de texto (403, 500) */ }
      if (!res || !res.status) throw new Error(txt.trim() || `HTTP ${r.status}`)
      setAtual(res)
      if (res.status === 'ok') {
        toast.success(res.mensagem)
        qc.invalidateQueries({ queryKey: ['dre'] })
      } else if (res.status === 'sem_fonte') toast.warning(res.mensagem)
      else toast.error(res.mensagem)
    } catch (e: any) { toast.error(e.message) } finally { setAtualizando(false) }
  }

  const d = dre.data
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end gap-3">
        <div>
          <label className="text-xs text-muted-foreground block mb-1">Mês</label>
          <select className="h-9 rounded-md border px-2 text-sm bg-white" value={mes} onChange={e => setMes(+e.target.value)}>
            {MESES.map((m, i) => <option key={m} value={i + 1}>{m}</option>)}
          </select>
        </div>
        <div>
          <label className="text-xs text-muted-foreground block mb-1">Ano</label>
          <Input type="number" className="w-24 h-9" value={ano} onChange={e => setAno(+e.target.value || ano)} />
        </div>
        <div>
          <label className="text-xs text-muted-foreground block mb-1">Visão</label>
          <select className="h-9 rounded-md border px-2 text-sm bg-white" value={visao} onChange={e => setVisao(e.target.value as any)}>
            <option value="mensal">Mensal</option>
            <option value="acumulada">Acumulada no ano</option>
          </select>
        </div>
        <div>
          <label className="text-xs text-muted-foreground block mb-1">Filial</label>
          <select className="h-9 rounded-md border px-2 text-sm bg-white" value={filial} onChange={e => setFilial(e.target.value)}>
            <option value="">Consolidado</option>
            {(filiais.data ?? []).map(f => <option key={f} value={f}>{f}</option>)}
          </select>
        </div>
        <div className="ml-auto flex items-center gap-2">
          {d?.congelado && (
            <span className="inline-flex items-center gap-1 text-xs bg-sky-50 text-sky-700 border border-sky-200 rounded-full px-2.5 py-1">
              <Lock className="h-3 w-3" /> Fechado — versão {d.versao}
            </span>
          )}
          {podeFechar && <Button size="sm" variant="outline" disabled={atualizando} onClick={atualizarDados}>
            {atualizando ? 'Atualizando…' : 'Atualizar dados'}</Button>}
          {podeFechar && visao === 'mensal' && !d?.fechamento_ativo && <Button size="sm" variant="outline" onClick={fechar}>Fechar período (grupo inteiro)</Button>}
          {podeFechar && visao === 'mensal' && d?.fechamento_ativo && <Button size="sm" variant="outline" onClick={reabrir}>Reabrir período</Button>}
        </div>
      </div>

      {atual && (
        <div className={`rounded-md border p-3 text-sm ${atual.status === 'ok' ? 'border-emerald-300 bg-emerald-50 text-emerald-900' : 'border-amber-300 bg-amber-50 text-amber-900'}`}>
          <b>Atualizar dados — {new Date(atual.hora).toLocaleString('pt-BR')}:</b> {atual.mensagem}
          {atual.fontes.map(f => (
            <div key={f.fonte} className="text-xs mt-1">{f.fonte}: {f.status === 'ok' ? `${f.linhas} linha(s)` : f.mensagem || f.status}</div>
          ))}
        </div>
      )}

      {d && d.nao_mapeado.length > 0 && (
        <div className="flex gap-2 rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900">
          <AlertTriangle className="h-4 w-4 mt-0.5 shrink-0" />
          <div>
            <b>{d.nao_mapeado.length} conta(s) sem mapeamento</b> ficaram fora do DRE ({brl(d.nao_mapeado.reduce((s, x) => s + x.valor, 0))}):{' '}
            {d.nao_mapeado.slice(0, 5).map(x => x.conta).join(', ')}{d.nao_mapeado.length > 5 ? '…' : ''}. Ajuste em Mapa do DRE.
          </div>
        </div>
      )}

      {d?.pct_status === 'sem_papel' && (
        <div className="flex gap-2 rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900">
          <AlertTriangle className="h-4 w-4 mt-0.5 shrink-0" />
          <div>{d.aviso_faturamento}</div>
        </div>
      )}

      {dre.isError && <p className="text-sm text-red-600">{(dre.error as Error).message}</p>}
      <div className="rounded-lg border bg-white overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b bg-gray-50 text-xs text-muted-foreground">
              <th className="text-left font-medium p-2.5">Linha</th>
              <th className="text-right font-medium p-2.5">Realizado</th>
              <th className="text-right font-medium p-2.5" title="Percentual sobre o Faturamento Líquido">% Fat. Líq.</th>
              <th className="text-right font-medium p-2.5">Orçado</th>
              <th className="text-right font-medium p-2.5">Δ Orçado</th>
              <th className="text-right font-medium p-2.5">Ano anterior</th>
              <th className="text-right font-medium p-2.5">Δ Ano</th>
            </tr>
          </thead>
          <tbody>
            {(d?.linhas ?? []).map(l => (
              <tr key={l.codigo}
                className={`border-b last:border-0 cursor-pointer hover:bg-sky-50/50 ${l.tipo === 'subtotal' ? 'bg-gray-50 font-semibold' : ''}`}
                onClick={() => setDrill(l)}>
                <td className="p-2.5" style={{ paddingLeft: 10 + l.nivel * 14 }}>{l.descricao}</td>
                <td className="p-2.5 text-right tabular-nums">{brl(l.realizado)}</td>
                <td className="p-2.5 text-right tabular-nums text-muted-foreground">
                  {l.pct_faturamento != null ? pctFmt(l.pct_faturamento)
                    : d?.pct_status === 'zero' ? <span title="Faturamento Líquido zero">não calculável</span> : '—'}
                </td>
                <td className="p-2.5 text-right tabular-nums text-muted-foreground">{brl(l.orcado)}</td>
                <td className="p-2.5 text-right tabular-nums"><Var v={l.var_orcado_pct} /></td>
                <td className="p-2.5 text-right tabular-nums text-muted-foreground">{brl(l.ano_anterior)}</td>
                <td className="p-2.5 text-right tabular-nums"><Var v={l.var_ano_pct} /></td>
              </tr>
            ))}
            {dre.isLoading && <tr><td colSpan={7} className="p-6 text-center text-muted-foreground">Carregando…</td></tr>}
          </tbody>
        </table>
      </div>
      <p className="text-xs text-muted-foreground">Clique numa linha para ver as contas e, nelas, os lançamentos.</p>

      {podeDetalhar && <DetalheLinhaDialog q={q} linha={drill} onClose={() => setDrill(null)} />}
      <Dialog open={!!drill && !podeDetalhar} onOpenChange={o => !o && setDrill(null)}>
        <DialogContent className="max-w-5xl max-h-[85vh] overflow-hidden flex flex-col">
          <DialogHeader><DialogTitle>{drill?.descricao} — lançamentos</DialogTitle></DialogHeader>
          {d?.congelado && (
            <p className="text-xs text-sky-800 bg-sky-50 border border-sky-200 rounded px-2.5 py-1.5">
              Período fechado: valores e contas do fechamento v{d.versao}.
            </p>
          )}
          {lancs.data?.aviso && (
            <p className="text-xs text-amber-800 bg-amber-50 border border-amber-200 rounded px-2.5 py-1.5">{lancs.data.aviso}</p>
          )}
          <div className="overflow-auto">
            <table className="w-full text-xs">
              <thead className="sticky top-0 bg-white">
                <tr className="border-b text-muted-foreground">
                  <th className="text-left p-2">Comp.</th><th className="text-left p-2">Filial</th>
                  <th className="text-left p-2">Conta</th><th className="text-left p-2">Linha</th>
                  <th className="text-left p-2">Documento</th><th className="text-left p-2">Histórico</th>
                  <th className="text-right p-2">Valor</th>
                </tr>
              </thead>
              <tbody>
                {(lancs.data?.lancamentos ?? []).map(l => (
                  <tr key={`${l.competencia}|${l.conta}|${l.filial}|${l.id}`} className="border-b last:border-0">
                    <td className="p-2 whitespace-nowrap">{l.competencia}</td>
                    <td className="p-2">{l.filial}</td>
                    <td className="p-2 whitespace-nowrap">{l.conta}{l.conta_desc ? ` · ${l.conta_desc}` : ''}</td>
                    <td className="p-2">{l.linha}</td>
                    <td className="p-2 whitespace-nowrap">{[l.tipo_doc, l.doc_ref].filter(Boolean).join(' ')}</td>
                    <td className="p-2">{l.historico}</td>
                    <td className="p-2 text-right tabular-nums">{brl(l.valor)}</td>
                  </tr>
                ))}
                {lancs.isLoading && <tr><td colSpan={7} className="p-6 text-center text-muted-foreground">Carregando…</td></tr>}
                {lancs.data && lancs.data.lancamentos.length === 0 && <tr><td colSpan={7} className="p-6 text-center text-muted-foreground">Sem lançamentos no período.</td></tr>}
              </tbody>
            </table>
            {lancs.data?.truncado && <p className="text-xs text-amber-700 p-2">Mostrando os primeiros 2.000 lançamentos — refine por filial.</p>}
          </div>
        </DialogContent>
      </Dialog>
    </div>
  )
}
