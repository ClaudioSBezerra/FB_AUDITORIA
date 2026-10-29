import { useState } from 'react'
import { toast } from 'sonner'
import { Upload } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { api } from './util'

const fmt = (v: number) => v.toLocaleString('pt-BR', { minimumFractionDigits: 2, maximumFractionDigits: 2 })

interface Par { competencia: string; filial: string; lancamentos: number }
interface Aviso { codigo: string; mensagem: string }
interface Resposta {
  simulacao: boolean; linhas: number; carga_id?: number; origem: string; arquivo: string
  pares: Par[]; avisos: Aviso[]; ajustes_ignorados?: number; linhas_ignoradas?: number
  conferencia_total?: { esperado: number; calculado: number; ok: boolean }
}

// Cartão grande com o que foi lido do arquivo: competência e filial com a contagem
// de cada par e o total. É a salvaguarda contra arquivo com mês ou filial desatualizados.
function CartaoLido({ r, gravado }: { r: Resposta; gravado: boolean }) {
  return (
    <div className={`rounded-lg border-2 p-4 space-y-2 ${gravado ? 'border-green-600 bg-green-50' : 'border-amber-500 bg-amber-50'}`}>
      <div className="text-xs font-semibold uppercase tracking-wide">{gravado ? 'Carga gravada' : 'Conferência: nada foi gravado ainda'}</div>
      <ul className="space-y-1">
        {r.pares.map(p => (
          <li key={p.competencia + '|' + p.filial} className="text-lg font-bold">
            {gravado ? 'Gravado em' : 'Será gravado em'} {p.competencia}, filial {p.filial}: {p.lancamentos.toLocaleString('pt-BR')} lançamentos
          </li>
        ))}
      </ul>
      <div className="text-sm">Total do arquivo: <strong>{r.linhas.toLocaleString('pt-BR')}</strong> lançamentos em {r.pares.length} par(es) competência/filial. Origem: {r.origem}.</div>
      {r.avisos.length > 0 && (
        <ul className="list-disc pl-5 space-y-1 text-sm text-amber-900">
          {r.avisos.map((a, i) => <li key={i}><strong>Aviso:</strong> {a.mensagem}</li>)}
        </ul>
      )}
      {r.conferencia_total && (r.conferencia_total.ok
        ? <p className="text-sm text-green-800">TOTAL do arquivo confere com a soma das contas ({fmt(r.conferencia_total.calculado)}).</p>
        : <p className="text-sm text-amber-900"><strong>Aviso:</strong> o TOTAL do arquivo ({fmt(r.conferencia_total.esperado)}) difere da soma das contas ({fmt(r.conferencia_total.calculado)}). Confira o arquivo.</p>)}
      {!!r.linhas_ignoradas && <p className="text-xs">{r.linhas_ignoradas} linha(s) ignorada(s): subtotais sem código e linhas de conferência sem Mês e Loja.</p>}
      {!!r.ajustes_ignorados && <p className="text-xs">{r.ajustes_ignorados} linha(s) com ajuste: o ajuste não é importado, só o valor realizado.</p>}
    </div>
  )
}

// Carga de lançamentos por CSV ou XLSX — usada até a ingestão incremental do Winthor
// existir; os dados entram na mesma tabela analítica. Duas etapas (história 2.19):
// "Conferir arquivo" simula (?simular=1, sem gravar) e só depois aparece "Confirmar carga".
export default function AdmCarga() {
  const [arquivo, setArquivo] = useState<File | null>(null)
  const [ocupado, setOcupado] = useState(false)
  const [conferido, setConferido] = useState<Resposta | null>(null) // simulação do arquivo atual
  const [gravado, setGravado] = useState<Resposta | null>(null)
  const [erro, setErro] = useState('')

  function escolher(f: File | null) {
    setArquivo(f); setConferido(null); setGravado(null); setErro('')
  }

  async function enviar(simular: boolean) {
    if (!arquivo || ocupado) return
    if (!simular && !conferido) return // a tela nunca grava sem a conferência
    setOcupado(true); setErro('')
    try {
      const fd = new FormData(); fd.append('arquivo', arquivo)
      const r = await api<Resposta>('/api/aud/dre/lancamentos/importar' + (simular ? '?simular=1' : ''), { method: 'POST', body: fd })
      if (simular) { setConferido(r); setGravado(null) }
      else { setGravado(r); setConferido(null); setArquivo(null); toast.success(`${r.linhas} lançamentos importados`) }
    } catch (e: any) { setErro(e.message); setConferido(null) } finally { setOcupado(false) }
  }

  return (
    <div className="max-w-2xl space-y-4">
      {conferido && <CartaoLido r={conferido} gravado={false} />}
      {gravado && <CartaoLido r={gravado} gravado />}
      <div className="rounded-lg border bg-white p-4 space-y-3">
        <h2 className="text-sm font-semibold">Importar lançamentos (CSV ou XLSX)</h2>
        <p className="text-xs text-muted-foreground">
          Dois formatos, reconhecidos pelo cabeçalho (pode vir depois de linhas de título, até a linha 20).
          <br /><strong>Planilha:</strong> <code>CONTA</code>, <code>VL. Realizado</code>, <code>Mês</code> (AAAAMM, ex.: 202609) e <code>Loja</code> (obrigatórias);
          <code> DESCRIÇÃO</code> (opcional). <code>Ajuste</code>/<code>Ajustes</code> não é importado (as linhas com ajuste são só contadas); <code>Saldo</code> e <code>Grupo</code> são ignorados. Loja <code>01</code> e <code>1</code> são a mesma filial.
          <br /><strong>Entrada_Dados:</strong> a aba de carga mensal aceita como está. Com as colunas <code>Código</code> e <code>Conta</code>, o código é a conta e <code>Conta</code> é a descrição. O cabeçalho pode se repetir (duas tabelas na mesma aba). Linhas sem código (subtotais), com valor em branco e linhas sem <code>Mês</code> e <code>Loja</code> (TOTAL, prova real) são ignoradas; a linha <code>TOTAL</code> é conferida contra a soma das contas. Se o XLSX foi salvo sem o valor calculado das fórmulas de Mês e Loja, abra no Excel e salve de novo.
          <br /><strong>Sistema:</strong> <code>competencia</code> (AAAA-MM), <code>filial</code>, <code>conta</code>, <code>valor</code> (obrigatórias);
          <code> centro_custo, canal, conta_desc, tipo_doc, doc_ref, historico</code> (opcionais).
          <br />CSV com separador <code>;</code> ou <code>,</code>, ou XLSX (primeira planilha com o cabeçalho); valor aceita 1.234,56. A carga é tudo-ou-nada: se uma linha for inválida, nada é gravado. Reenviar o arquivo substitui os lançamentos de cada competência e filial presentes nele (os de outras origens ficam). Competência fechada é recusada.
          <br /><strong>Duas etapas:</strong> primeiro <em>Conferir arquivo</em> (lê e valida, não grava) mostra a competência, a filial e a quantidade de lançamentos que o arquivo traz, com avisos; só então aparece <em>Confirmar carga</em>.
        </p>
        <input key={gravado?.carga_id ?? 0} type="file" accept=".csv,.xlsx,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" onChange={e => escolher(e.target.files?.[0] ?? null)} className="text-sm" />
        <div className="flex flex-wrap gap-2">
          <Button size="sm" variant={conferido ? 'outline' : 'default'} disabled={!arquivo || ocupado} onClick={() => enviar(true)}>{ocupado && !conferido ? 'Lendo…' : 'Conferir arquivo'}</Button>
          {conferido && <Button size="sm" disabled={ocupado} onClick={() => enviar(false)}><Upload className="h-4 w-4 mr-1.5" />{ocupado ? 'Gravando…' : 'Confirmar carga'}</Button>}
        </div>
        {erro && <pre className="whitespace-pre-wrap rounded-md border border-red-200 bg-red-50 p-2 text-xs text-red-700">{erro}</pre>}
      </div>
    </div>
  )
}
