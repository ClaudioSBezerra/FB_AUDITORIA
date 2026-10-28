import { Fragment, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { AlertTriangle } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { api, brl, pctFmt, MESES } from './util'
import { DetalheContaDialog } from './DetalheDRE'
import { SeletorRecorte, useRecorte } from './Recorte'

// "Meu departamento" (história 4.8): as contas dos departamentos do Gerente com
// Realizado, Orçado, Ano anterior e variação. Orçado e Ano anterior ausentes vêm
// como null do servidor e aparecem como "sem orçado" / "sem ano anterior", nunca zero.

interface Linha {
  conta: string; descricao: string; linha: string
  realizado: number; orcado: number | null; ano_anterior: number | null
  var_orcado_valor: number | null; var_orcado_pct: number | null
  var_ano_anterior_valor: number | null; var_ano_anterior_pct: number | null
  pct_faturamento: number | null
}
interface Depto { codigo: string; nome: string; grupo: string; totais: Linha; contas: Linha[] }
interface Resp {
  pct_status: 'ok' | 'sem_papel' | 'zero'; avisos: string[]; tem_departamento: boolean
  departamentos: Depto[]; total: Linha
}

const hoje = new Date()

function Base({ v, sem }: { v: number | null; sem: string }) {
  return v == null ? <span className="italic text-muted-foreground">{sem}</span> : <>{brl(v)}</>
}

function Var({ valor, pct }: { valor: number | null; pct: number | null }) {
  if (valor == null) return <span className="text-muted-foreground">—</span>
  const cor = valor >= 0 ? 'text-emerald-700' : 'text-red-600'
  return (
    <span className={cor}>
      {brl(valor)} <span className="text-muted-foreground">({pct != null ? pctFmt(pct) : 'não calculável'})</span>
    </span>
  )
}

export default function MeuDepartamento() {
  const [ano, setAno] = useState(hoje.getFullYear())
  const [mes, setMes] = useState(hoje.getMonth() + 1)
  const [visao, setVisao] = useState<'mensal' | 'acumulada'>('mensal')
  const { recorte, setRecorte, param } = useRecorte()
  const [conta, setConta] = useState<string | null>(null)
  const q = `ano=${ano}&mes=${mes}&visao=${visao}&${param}`
  const dados = useQuery({ queryKey: ['meu-departamento', q], queryFn: () => api<Resp>(`/api/aud/meu-departamento?${q}`) })
  const d = dados.data

  const pctFl = (v: number | null) =>
    v != null ? `${v.toFixed(1).replace('.', ',')}%` : d?.pct_status === 'zero' ? 'não calculável' : '—'

  const celulas = (x: Linha, pequeno: boolean) => {
    const c = pequeno ? 'p-2 text-right tabular-nums text-xs' : 'p-2.5 text-right tabular-nums'
    return (
      <>
        <td className={c}>{brl(x.realizado)}</td>
        <td className={`${c} text-muted-foreground`} title="Percentual sobre o Faturamento Líquido">{pctFl(x.pct_faturamento)}</td>
        <td className={c}><Base v={x.orcado} sem="sem orçado" /></td>
        <td className={c}><Var valor={x.var_orcado_valor} pct={x.var_orcado_pct} /></td>
        <td className={c}><Base v={x.ano_anterior} sem="sem ano anterior" /></td>
        <td className={c}><Var valor={x.var_ano_anterior_valor} pct={x.var_ano_anterior_pct} /></td>
      </>
    )
  }

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
          <select className="h-9 rounded-md border px-2 text-sm bg-white" value={visao} onChange={e => setVisao(e.target.value as 'mensal' | 'acumulada')}>
            <option value="mensal">Mensal</option>
            <option value="acumulada">Acumulada no ano</option>
          </select>
        </div>
        <SeletorRecorte valor={recorte} onChange={setRecorte} />
      </div>

      {(d?.avisos ?? []).map(a => (
        <div key={a} className="flex gap-2 rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900">
          <AlertTriangle className="h-4 w-4 mt-0.5 shrink-0" /><div>{a}</div>
        </div>
      ))}
      {d && !d.tem_departamento && (
        <p className="text-sm text-muted-foreground">Seu usuário ainda não tem departamento vinculado. Fale com a controladoria.</p>
      )}
      {dados.isError && <p className="text-sm text-red-600">{(dados.error as Error).message}</p>}

      <div className="rounded-lg border bg-white overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b bg-gray-50 text-xs text-muted-foreground">
              <th className="text-left font-medium p-2.5">Departamento / conta</th>
              <th className="text-right font-medium p-2.5">Realizado</th>
              <th className="text-right font-medium p-2.5">% Fat. Líq.</th>
              <th className="text-right font-medium p-2.5">Orçado</th>
              <th className="text-right font-medium p-2.5">Var. orçado</th>
              <th className="text-right font-medium p-2.5">Ano anterior</th>
              <th className="text-right font-medium p-2.5">Var. ano anterior</th>
            </tr>
          </thead>
          <tbody>
            {(d?.departamentos ?? []).map(x => (
              <Fragment key={x.codigo}>
                <tr className="border-b bg-sky-50/60 font-semibold">
                  <td className="p-2.5">{x.codigo} · {x.nome}</td>
                  {celulas(x.totais, false)}
                </tr>
                {x.contas.map(c => (
                  <tr key={`${x.codigo}|${c.conta}`} className="border-b last:border-0 cursor-pointer hover:bg-sky-50/50" onClick={() => setConta(c.conta)}>
                    <td className="p-2 pl-8 text-xs">{c.conta}{c.descricao ? ` · ${c.descricao}` : ''}</td>
                    {celulas(c, true)}
                  </tr>
                ))}
              </Fragment>
            ))}
            {d && d.departamentos.length > 0 && (
              <tr className="border-t-2 bg-gray-50 font-semibold">
                <td className="p-2.5">Total dos meus departamentos</td>
                {celulas(d.total, false)}
              </tr>
            )}
            {d && d.departamentos.length === 0 && d.tem_departamento && (
              <tr><td colSpan={7} className="p-6 text-center text-muted-foreground">Nenhuma conta com movimento no período.</td></tr>
            )}
            {dados.isLoading && <tr><td colSpan={7} className="p-6 text-center text-muted-foreground">Carregando…</td></tr>}
          </tbody>
        </table>
      </div>
      <p className="text-xs text-muted-foreground">
        Clique numa conta para ver os lançamentos. Mês fechado vem do fechamento (contas e departamento congelados); mês aberto, ao vivo.
        Variação = Realizado − base, em valor e em percentual da base. &ldquo;Sem orçado&rdquo;: ainda não há orçado por conta, e nenhum valor é estimado.
        &ldquo;Sem ano anterior&rdquo;: a conta não teve movimento no ano anterior.
      </p>
      <DetalheContaDialog q={q} conta={conta} onClose={() => setConta(null)} />
    </div>
  )
}
