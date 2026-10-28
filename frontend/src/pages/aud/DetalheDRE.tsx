import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { AlertTriangle, ArrowLeft } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { api, brl } from './util'

// Detalhe do DRE: da linha (grupo) às contas e da conta aos lançamentos,
// em mês aberto e fechado (história 3.9). `q` traz ano, mês, visão e o recorte.

const POR_PAGINA = 200

interface ContaItem { conta: string; descricao: string; linha: string; valor: number }
interface Lancamento {
  id: number; competencia: string; filial: string; centro_custo: string; canal: string
  origem: string; tipo_doc: string; doc_ref: string; historico: string; valor: number
}
interface LancResp {
  conta: string; descricao: string; total: number; soma_lancamentos: number; meses_fechados: string[]
  divergente: boolean; aviso_divergencia: string; avisos: string[]
  total_lancamentos: number; limite: number; offset: number; lancamentos: Lancamento[]
}

function Aviso({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex gap-2 rounded-md border border-amber-300 bg-amber-50 p-2.5 text-xs text-amber-900">
      <AlertTriangle className="h-4 w-4 shrink-0" /><div>{children}</div>
    </div>
  )
}

function Lancamentos({ q, conta, onVoltar }: { q: string; conta: string; onVoltar?: () => void }) {
  const [pagina, setPagina] = useState(0)
  const r = useQuery({
    queryKey: ['dre-conta-lanc', q, conta, pagina],
    queryFn: () => api<LancResp>(`/api/aud/dre/contas/lancamentos?${q}&conta=${encodeURIComponent(conta)}&limite=${POR_PAGINA}&offset=${pagina * POR_PAGINA}`),
  })
  const d = r.data
  const paginas = d ? Math.max(1, Math.ceil(d.total_lancamentos / POR_PAGINA)) : 1
  return (
    <div className="flex flex-col gap-3 min-h-0">
      <div className="flex items-center gap-2 text-sm">
        {onVoltar && <Button size="sm" variant="ghost" onClick={onVoltar}><ArrowLeft className="h-4 w-4 mr-1" />Contas</Button>}
        <b>{conta}{d?.descricao ? ` · ${d.descricao}` : ''}</b>
        {d && <span className="ml-auto text-muted-foreground">Total no DRE: <b className="text-foreground">{brl(d.total)}</b></span>}
      </div>
      {d?.meses_fechados.length ? (
        <p className="text-xs text-sky-800 bg-sky-50 border border-sky-200 rounded px-2.5 py-1.5">
          Total de mês fechado vem do fechamento ({d.meses_fechados.join(', ')}); os lançamentos listados são lidos da base.
        </p>
      ) : null}
      {d?.divergente && <Aviso><b>Divergência:</b> {d.aviso_divergencia} Soma dos lançamentos: {brl(d.soma_lancamentos)}.</Aviso>}
      {(d?.avisos ?? []).map(a => <Aviso key={a}>{a}</Aviso>)}
      {r.isError && <p className="text-sm text-red-600">{(r.error as Error).message}</p>}
      <div className="overflow-auto">
        <table className="w-full text-xs">
          <thead className="sticky top-0 bg-white">
            <tr className="border-b text-muted-foreground">
              <th className="text-left p-2">Data</th>
              <th className="text-left p-2">Fornecedor / histórico</th>
              <th className="text-left p-2">Nota</th>
              <th className="text-left p-2">Filial</th>
              <th className="text-right p-2">Valor</th>
            </tr>
          </thead>
          <tbody>
            {(d?.lancamentos ?? []).map(l => (
              <tr key={l.id} className="border-b last:border-0">
                <td className="p-2 whitespace-nowrap">{l.competencia}</td>
                <td className="p-2">{l.historico}</td>
                <td className="p-2 whitespace-nowrap">{[l.tipo_doc, l.doc_ref].filter(Boolean).join(' ')}</td>
                <td className="p-2">{l.filial}</td>
                <td className="p-2 text-right tabular-nums">{brl(l.valor)}</td>
              </tr>
            ))}
            {r.isLoading && <tr><td colSpan={5} className="p-6 text-center text-muted-foreground">Carregando…</td></tr>}
            {d && d.lancamentos.length === 0 && <tr><td colSpan={5} className="p-6 text-center text-muted-foreground">Sem lançamentos no período.</td></tr>}
          </tbody>
        </table>
      </div>
      {d && d.total_lancamentos > POR_PAGINA && (
        <div className="flex items-center gap-2 text-xs">
          <Button size="sm" variant="outline" disabled={pagina === 0} onClick={() => setPagina(pagina - 1)}>Anterior</Button>
          <span>Página {pagina + 1} de {paginas} ({d.total_lancamentos} lançamentos)</span>
          <Button size="sm" variant="outline" disabled={pagina + 1 >= paginas} onClick={() => setPagina(pagina + 1)}>Próxima</Button>
        </div>
      )}
    </div>
  )
}

/** Lançamentos de uma conta (usado direto pela tela por departamento). */
export function DetalheContaDialog({ q, conta, onClose }: { q: string; conta: string | null; onClose: () => void }) {
  return (
    <Dialog open={!!conta} onOpenChange={o => !o && onClose()}>
      <DialogContent className="max-w-4xl max-h-[85vh] overflow-hidden flex flex-col">
        <DialogHeader><DialogTitle>Lançamentos da conta</DialogTitle></DialogHeader>
        {conta && <Lancamentos q={q} conta={conta} />}
      </DialogContent>
    </Dialog>
  )
}

/** Da linha do DRE às contas e, clicando numa conta, aos lançamentos. */
export function DetalheLinhaDialog({ q, linha, onClose }: { q: string; linha: { codigo: string; descricao: string } | null; onClose: () => void }) {
  const [conta, setConta] = useState<string | null>(null)
  const contas = useQuery({
    queryKey: ['dre-contas', q, linha?.codigo],
    enabled: !!linha,
    queryFn: () => api<{ contas: ContaItem[]; avisos: string[] }>(`/api/aud/dre/contas?${q}&linha=${encodeURIComponent(linha!.codigo)}`),
  })
  const fechar = () => { setConta(null); onClose() }
  return (
    <Dialog open={!!linha} onOpenChange={o => !o && fechar()}>
      <DialogContent className="max-w-4xl max-h-[85vh] overflow-hidden flex flex-col">
        <DialogHeader><DialogTitle>{linha?.descricao} — {conta ? 'lançamentos' : 'contas'}</DialogTitle></DialogHeader>
        {conta ? <Lancamentos q={q} conta={conta} onVoltar={() => setConta(null)} /> : (
          <div className="flex flex-col gap-3 overflow-auto">
            {(contas.data?.avisos ?? []).map(a => <Aviso key={a}>{a}</Aviso>)}
            {contas.isError && <p className="text-sm text-red-600">{(contas.error as Error).message}</p>}
            <table className="w-full text-xs">
              <thead className="sticky top-0 bg-white">
                <tr className="border-b text-muted-foreground">
                  <th className="text-left p-2">Conta</th><th className="text-left p-2">Linha</th><th className="text-right p-2">Valor</th>
                </tr>
              </thead>
              <tbody>
                {(contas.data?.contas ?? []).map(c => (
                  <tr key={c.conta} className="border-b last:border-0 cursor-pointer hover:bg-sky-50/50" onClick={() => setConta(c.conta)}>
                    <td className="p-2">{c.conta}{c.descricao ? ` · ${c.descricao}` : ''}</td>
                    <td className="p-2">{c.linha}</td>
                    <td className="p-2 text-right tabular-nums">{brl(c.valor)}</td>
                  </tr>
                ))}
                {contas.isLoading && <tr><td colSpan={3} className="p-6 text-center text-muted-foreground">Carregando…</td></tr>}
                {contas.data && contas.data.contas.length === 0 && <tr><td colSpan={3} className="p-6 text-center text-muted-foreground">Sem contas com movimento no período.</td></tr>}
              </tbody>
            </table>
            <p className="text-xs text-muted-foreground">Clique numa conta para ver os lançamentos.</p>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
