import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Upload, Download } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { api, brl, MESES } from './util'

// Modelos fictícios gerados no navegador (nenhum dado real).
const MODELO_ESTRUTURA = [
  'codigo;descricao;nivel;ordem;tipo;soma_de;papel',
  'RECEITA;Receita de vendas;1;10;conta;;',
  'DEVOLUCAO;Devolucoes;1;20;conta;;',
  'LIQUIDA;Receita liquida;0;30;subtotal;RECEITA|DEVOLUCAO;faturamento_liquido',
].join('\n')
const MODELO_MAPA = [
  'conta_padrao;linha_codigo;sinal',
  '1001;RECEITA;1',
  '2%;DEVOLUCAO;1',
].join('\n')

function baixarModelo(nome: string, conteudo: string) {
  const url = URL.createObjectURL(new Blob(['﻿' + conteudo + '\n'], { type: 'text/csv;charset=utf-8' }))
  const a = document.createElement('a')
  a.href = url; a.download = nome; a.click()
  URL.revokeObjectURL(url)
}

interface Resultado { [k: string]: any }

function BlocoCarga({ titulo, rota, descricao, modeloNome, modelo, resumo, aoCarregar }: {
  titulo: string; rota: string; descricao: string; modeloNome: string; modelo: string
  resumo: (r: Resultado) => string; aoCarregar: () => void
}) {
  const [arquivo, setArquivo] = useState<File | null>(null)
  const [ocupado, setOcupado] = useState(false)
  const [resultado, setResultado] = useState<Resultado | null>(null)
  const [erro, setErro] = useState('')

  async function enviar() {
    if (!arquivo || ocupado) return
    setOcupado(true); setErro(''); setResultado(null)
    try {
      const fd = new FormData(); fd.append('arquivo', arquivo)
      const r = await api<Resultado>(rota, { method: 'POST', body: fd })
      setResultado(r); setArquivo(null); aoCarregar(); toast.success(titulo + ': carga concluída')
    } catch (e: any) { setErro(e.message) } finally { setOcupado(false) }
  }

  return (
    <div className="rounded-lg border bg-white p-4 space-y-3">
      <h2 className="text-sm font-semibold">{titulo}</h2>
      <p className="text-xs text-muted-foreground">{descricao}</p>
      <input type="file" accept=".csv,.xlsx,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
        onChange={e => setArquivo(e.target.files?.[0] ?? null)} className="text-sm" />
      <div className="flex flex-wrap gap-2">
        <Button size="sm" disabled={!arquivo || ocupado} onClick={enviar}><Upload className="h-4 w-4 mr-1.5" />{ocupado ? 'Enviando…' : 'Carregar'}</Button>
        <Button size="sm" variant="outline" onClick={() => baixarModelo(modeloNome, modelo)}><Download className="h-4 w-4 mr-1.5" />Baixar modelo CSV</Button>
      </div>
      {erro && <pre className="whitespace-pre-wrap rounded-md border border-red-200 bg-red-50 p-2 text-xs text-red-700">{erro}</pre>}
      {resultado && (
        <p className="rounded-md border bg-gray-50 p-2 text-xs">
          {resumo(resultado)} {resultado.meses_fechados}
        </p>
      )}
    </div>
  )
}

export function CargasDRE() {
  const qc = useQueryClient()
  const recarregar = () => {
    for (const k of ['dre-linhas', 'dre-mapa', 'dre', 'dre-sem-mapa']) qc.invalidateQueries({ queryKey: [k] })
  }
  return (
    <div className="grid gap-4 md:grid-cols-2">
      <BlocoCarga
        titulo="Carregar estrutura" rota="/api/aud/dre/estrutura/carga" modeloNome="modelo_estrutura_dre.csv" modelo={MODELO_ESTRUTURA}
        descricao="CSV ou XLSX com codigo, descricao, tipo (conta ou subtotal); nivel, ordem, soma_de (códigos separados por vírgula ou |; -COD subtrai) e papel (faturamento_liquido, no máximo uma linha) opcionais. Substitui a estrutura: o que não vier no arquivo sai do DRE (a linha fica inativa, não é apagada) e a carga é recusada se a linha ainda tem conta no Mapa. Tudo ou nada."
        resumo={r => `${r.linhas} linhas (${r.subtotais} subtotais): ${r.novas} novas, ${r.atualizadas} atualizadas, ${r.desativadas} desativadas.`}
        aoCarregar={recarregar} />
      <BlocoCarga
        titulo="Carregar Mapa" rota="/api/aud/dre/mapa/carga" modeloNome="modelo_mapa_dre.csv" modelo={MODELO_MAPA}
        descricao="CSV ou XLSX com conta_padrao (aceita % no fim, como prefixo) e linha_codigo (linha de conta da estrutura); sinal (1 ou -1; vazio = 1) opcional. Substitui o Mapa inteiro: contas que não vierem no arquivo são removidas. Carregue a estrutura antes. Tudo ou nada."
        resumo={r => `${r.linhas} contas: ${r.inseridas} inseridas, ${r.atualizadas} atualizadas, ${r.removidas} removidas.`}
        aoCarregar={recarregar} />
    </div>
  )
}

interface SemMapa { total: number; soma: number; truncado: boolean; itens: { conta: string; descricao: string; valor: number }[] }

// Contas com lançamento no período e sem Mapa (ligada pelo aviso do DRE).
export function ContasSemMapa({ ano, mes, visao }: { ano: number; mes: number; visao: string }) {
  const q = `ano=${ano}&mes=${mes}&visao=${visao}`
  const lista = useQuery({ queryKey: ['dre-sem-mapa', q], queryFn: () => api<SemMapa>(`/api/aud/dre/mapa/sem-mapa?${q}`) })
  const d = lista.data
  return (
    <div id="sem-mapa" className="rounded-lg border border-amber-300 bg-amber-50 p-3 space-y-2">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-sm font-semibold">
          Contas sem Mapa — {visao === 'acumulada' ? `acumulado até ${MESES[mes - 1]}/${ano}` : `${MESES[mes - 1]}/${ano}`}
          {d && ` (${d.total} conta(s), ${brl(d.soma)})`}
        </h2>
        <a className="text-xs underline" href={`/api/aud/dre/mapa/sem-mapa?${q}&formato=csv`}>Baixar CSV</a>
      </div>
      {lista.isLoading && <p className="text-xs text-muted-foreground">Carregando…</p>}
      {d && d.total === 0 && <p className="text-xs text-muted-foreground">Nenhuma conta sem Mapa no período.</p>}
      {d && d.total > 0 && (
        <table className="w-full text-sm">
          <tbody>
            {d.itens.map(x => (
              <tr key={x.conta} className="border-b border-amber-200 last:border-0">
                <td className="p-1.5 font-mono text-xs">{x.conta}</td><td className="p-1.5">{x.descricao}</td>
                <td className="p-1.5 text-right tabular-nums">{brl(x.valor)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {d?.truncado && <p className="text-xs text-muted-foreground">Mostrando {d.itens.length} de {d.total}; o CSV traz todas.</p>}
    </div>
  )
}
