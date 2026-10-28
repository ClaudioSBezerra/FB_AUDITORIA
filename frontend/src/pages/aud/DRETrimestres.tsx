import { Fragment } from 'react'
import { useQuery } from '@tanstack/react-query'
import { AlertTriangle } from 'lucide-react'
import { api, brl, pctFmt } from './util'

// Visão por trimestre (história 3.10): T1 a T4 e Ano, cada coluna com realizado,
// percentual sobre o Faturamento Líquido do próprio trimestre, orçado e ano anterior.

interface Coluna {
  rotulo: string; ini: string; fim: string; vazio: boolean; parcial: boolean; meses_fechados: string[]
  pct_status: 'ok' | 'sem_papel' | 'zero'
}
interface Valor { rotulo: string; realizado: number; orcado: number; ano_anterior: number; pct_faturamento: number | null }
interface LinhaT { codigo: string; descricao: string; nivel: number; tipo: 'conta' | 'subtotal'; valores: Valor[] }
interface Resp { ano: number; colunas: Coluna[]; linhas: LinhaT[]; aviso_faturamento: string }

function dica(c: Coluna): string {
  if (c.vazio) return 'Período que ainda não começou'
  if (!c.parcial) return `Todos os meses fechados: ${c.meses_fechados.join(', ')}`
  return c.meses_fechados.length
    ? `Parcial. Meses fechados: ${c.meses_fechados.join(', ')}. Os demais estão abertos (lidos ao vivo).`
    : 'Parcial. Nenhum mês fechado; tudo lido ao vivo.'
}

export default function DRETrimestres({ ano, recorte }: { ano: number; recorte: string }) {
  const q = `ano=${ano}&${recorte}`
  const r = useQuery({ queryKey: ['dre-trimestres', q], queryFn: () => api<Resp>(`/api/aud/dre/trimestres?${q}`) })
  const d = r.data
  return (
    <div className="space-y-3">
      {d?.aviso_faturamento && (
        <div className="flex gap-2 rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900">
          <AlertTriangle className="h-4 w-4 mt-0.5 shrink-0" /><div>{d.aviso_faturamento}</div>
        </div>
      )}
      {r.isError && <p className="text-sm text-red-600">{(r.error as Error).message}</p>}
      <div className="rounded-lg border bg-white overflow-x-auto">
        <table className="w-full text-xs">
          <thead>
            <tr className="border-b bg-gray-50 text-muted-foreground">
              <th rowSpan={2} className="text-left font-medium p-2.5 sticky left-0 bg-gray-50">Linha</th>
              {(d?.colunas ?? []).map(c => (
                <th key={c.rotulo} colSpan={4} className={`text-center font-semibold p-2 border-l ${c.rotulo === 'Ano' ? 'bg-sky-50/70' : ''}`} title={dica(c)}>
                  {c.rotulo}
                  {c.parcial && !c.vazio && (
                    <span className="ml-1.5 rounded-full border border-amber-300 bg-amber-50 text-amber-800 px-1.5 py-0.5 text-[10px] font-normal cursor-help" title={dica(c)}>parcial</span>
                  )}
                  {c.vazio && <span className="ml-1.5 text-[10px] font-normal">não iniciado</span>}
                </th>
              ))}
            </tr>
            <tr className="border-b bg-gray-50 text-muted-foreground">
              {(d?.colunas ?? []).map(c => (
                <Fragment key={c.rotulo}>
                  <th className="text-right font-medium p-1.5 border-l">Realizado</th>
                  <th className="text-right font-medium p-1.5" title="Percentual sobre o Faturamento Líquido do trimestre">% Fat. Líq.</th>
                  <th className="text-right font-medium p-1.5">Orçado</th>
                  <th className="text-right font-medium p-1.5">Ano ant.</th>
                </Fragment>
              ))}
            </tr>
          </thead>
          <tbody>
            {(d?.linhas ?? []).map(l => (
              <tr key={l.codigo} className={`border-b last:border-0 ${l.tipo === 'subtotal' ? 'bg-gray-50 font-semibold' : ''}`}>
                <td className="p-2 sticky left-0 bg-inherit whitespace-nowrap" style={{ paddingLeft: 10 + l.nivel * 14 }}>{l.descricao}</td>
                {l.valores.map((v, i) => {
                  const c = d!.colunas[i]
                  if (c.vazio) return <td key={v.rotulo} colSpan={4} className="p-2 text-center text-muted-foreground border-l">—</td>
                  return (
                    <Fragment key={v.rotulo}>
                      <td className="p-1.5 text-right tabular-nums border-l">{brl(v.realizado)}</td>
                      <td className="p-1.5 text-right tabular-nums text-muted-foreground">
                        {v.pct_faturamento != null ? pctFmt(v.pct_faturamento).replace('+', '')
                          : c.pct_status === 'zero' ? <span title="Faturamento Líquido zero">n/c</span> : '—'}
                      </td>
                      <td className="p-1.5 text-right tabular-nums text-muted-foreground">{brl(v.orcado)}</td>
                      <td className="p-1.5 text-right tabular-nums text-muted-foreground">{brl(v.ano_anterior)}</td>
                    </Fragment>
                  )
                })}
              </tr>
            ))}
            {r.isLoading && <tr><td colSpan={21} className="p-6 text-center text-muted-foreground">Carregando…</td></tr>}
          </tbody>
        </table>
      </div>
      <p className="text-xs text-muted-foreground">
        Cada trimestre soma três meses: mês fechado vem do fechamento, mês aberto é lido ao vivo (trimestre marcado como parcial; passe o mouse para ver os meses fechados).
        O Ano soma os trimestres já iniciados. O percentual usa o Faturamento Líquido do próprio trimestre.
      </p>
    </div>
  )
}
