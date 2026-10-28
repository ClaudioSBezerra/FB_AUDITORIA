import { Fragment, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { AlertTriangle } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { api, brl, pctFmt, MESES } from './util'
import { DetalheContaDialog } from './DetalheDRE'
import { SeletorRecorte, useRecorte } from './Recorte'

interface ContaV { conta: string; descricao: string; linha: string; valor: number; pct_faturamento: number | null }
interface DeptoV { codigo: string; nome: string; total: number; pct_faturamento: number | null; contas: ContaV[] }
interface GrupoV { grupo: string; total: number; pct_faturamento: number | null; departamentos: DeptoV[] }
interface Resp {
  pct_status: 'ok' | 'sem_papel' | 'zero'; avisos: string[]; faturamento_liquido: number
  grupos: GrupoV[]; sem_departamento: DeptoV | null
}

const hoje = new Date()

export default function DREDepartamentos() {
  const [ano, setAno] = useState(hoje.getFullYear())
  const [mes, setMes] = useState(hoje.getMonth() + 1)
  const [visao, setVisao] = useState<'mensal' | 'acumulada'>('mensal')
  const { recorte, setRecorte, param } = useRecorte()
  const [conta, setConta] = useState<string | null>(null)
  const q = `ano=${ano}&mes=${mes}&visao=${visao}&${param}`
  const dados = useQuery({ queryKey: ['dre-departamentos', q], queryFn: () => api<Resp>(`/api/aud/dre/departamentos?${q}`) })
  const d = dados.data

  const pct = (v: number | null) =>
    v != null ? pctFmt(v) : d?.pct_status === 'zero' ? <span title="Faturamento Líquido zero">não calculável</span> : '—'

  const bloco = (x: DeptoV, chave: string) => (
    <Fragment key={chave}>
      <tr className="border-b bg-gray-50 font-medium">
        <td className="p-2.5 pl-6">{x.codigo ? `${x.codigo} · ${x.nome}` : x.nome}</td>
        <td className="p-2.5 text-right tabular-nums">{brl(x.total)}</td>
        <td className="p-2.5 text-right tabular-nums text-muted-foreground">{pct(x.pct_faturamento)}</td>
      </tr>
      {x.contas.map(c => (
        <tr key={`${chave}|${c.conta}`} className="border-b last:border-0 cursor-pointer hover:bg-sky-50/50" onClick={() => setConta(c.conta)}>
          <td className="p-2 pl-12 text-xs">{c.conta}{c.descricao ? ` · ${c.descricao}` : ''}</td>
          <td className="p-2 text-right tabular-nums text-xs">{brl(c.valor)}</td>
          <td className="p-2 text-right tabular-nums text-xs text-muted-foreground">{pct(c.pct_faturamento)}</td>
        </tr>
      ))}
    </Fragment>
  )

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
        <SeletorRecorte valor={recorte} onChange={setRecorte} />
      </div>

      {(d?.avisos ?? []).map(a => (
        <div key={a} className="flex gap-2 rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900">
          <AlertTriangle className="h-4 w-4 mt-0.5 shrink-0" /><div>{a}</div>
        </div>
      ))}
      {dados.isError && <p className="text-sm text-red-600">{(dados.error as Error).message}</p>}

      <div className="rounded-lg border bg-white overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b bg-gray-50 text-xs text-muted-foreground">
              <th className="text-left font-medium p-2.5">Departamento / conta</th>
              <th className="text-right font-medium p-2.5">Realizado</th>
              <th className="text-right font-medium p-2.5" title="Percentual sobre o Faturamento Líquido">% Fat. Líq.</th>
            </tr>
          </thead>
          <tbody>
            {(d?.grupos ?? []).map(g => (
              <Fragment key={g.grupo}>
                <tr className="border-b bg-sky-50/60 font-semibold">
                  <td className="p-2.5">{g.grupo}</td>
                  <td className="p-2.5 text-right tabular-nums">{brl(g.total)}</td>
                  <td className="p-2.5 text-right tabular-nums text-muted-foreground">{pct(g.pct_faturamento)}</td>
                </tr>
                {g.departamentos.length === 0 && (
                  <tr key={`${g.grupo}-vazio`} className="border-b"><td colSpan={3} className="p-2 pl-6 text-xs text-muted-foreground">Nenhum departamento com movimento.</td></tr>
                )}
                {g.departamentos.map(x => bloco(x, `${g.grupo}|${x.codigo}`))}
              </Fragment>
            ))}
            {d?.sem_departamento && (
              <Fragment>
                <tr className="border-b bg-amber-50/60 font-semibold">
                  <td className="p-2.5">Sem departamento</td>
                  <td className="p-2.5 text-right tabular-nums">{brl(d.sem_departamento.total)}</td>
                  <td className="p-2.5 text-right tabular-nums text-muted-foreground">{pct(d.sem_departamento.pct_faturamento)}</td>
                </tr>
                {d.sem_departamento.contas.map(c => (
                  <tr key={`sem|${c.conta}`} className="border-b last:border-0 cursor-pointer hover:bg-sky-50/50" onClick={() => setConta(c.conta)}>
                    <td className="p-2 pl-12 text-xs">{c.conta}{c.descricao ? ` · ${c.descricao}` : ''}</td>
                    <td className="p-2 text-right tabular-nums text-xs">{brl(c.valor)}</td>
                    <td className="p-2 text-right tabular-nums text-xs text-muted-foreground">{pct(c.pct_faturamento)}</td>
                  </tr>
                ))}
              </Fragment>
            )}
            {dados.isLoading && <tr><td colSpan={3} className="p-6 text-center text-muted-foreground">Carregando…</td></tr>}
          </tbody>
        </table>
      </div>
      <p className="text-xs text-muted-foreground">
        Clique numa conta para ver os lançamentos. Valores com o sinal do Mapa do DRE. Mês fechado vem do fechamento (departamento gravado ao fechar); mês aberto usa o dicionário atual.
      </p>
      <DetalheContaDialog q={q} conta={conta} onClose={() => setConta(null)} />
    </div>
  )
}
