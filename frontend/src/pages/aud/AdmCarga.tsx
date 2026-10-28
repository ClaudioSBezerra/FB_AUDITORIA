import { useState } from 'react'
import { toast } from 'sonner'
import { Upload } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { api } from './util'

// Carga de lançamentos por CSV ou XLSX — usada até a ingestão incremental do Winthor
// existir; os dados entram na mesma tabela analítica.
export default function AdmCarga() {
  const [arquivo, setArquivo] = useState<File | null>(null)
  const [enviando, setEnviando] = useState(false)

  async function enviar() {
    if (!arquivo) return
    setEnviando(true)
    try {
      const fd = new FormData(); fd.append('arquivo', arquivo)
      const r = await api<{ linhas: number; ajustes_ignorados?: number }>('/api/aud/dre/lancamentos/importar', { method: 'POST', body: fd })
      toast.success(`${r.linhas} lançamentos importados`)
      if (r.ajustes_ignorados && r.ajustes_ignorados > 0) toast.warning(`${r.ajustes_ignorados} linha(s) com ajuste: o ajuste não é importado, só o valor realizado`)
      setArquivo(null)
    } catch (e: any) { toast.error(e.message) } finally { setEnviando(false) }
  }

  return (
    <div className="max-w-2xl space-y-4">
      <div className="rounded-lg border bg-white p-4 space-y-3">
        <h2 className="text-sm font-semibold">Importar lançamentos (CSV ou XLSX)</h2>
        <p className="text-xs text-muted-foreground">
          Dois formatos, reconhecidos pelo cabeçalho (pode vir depois de linhas de título, até a linha 20).
          <br /><strong>Planilha:</strong> <code>CONTA</code>, <code>VL. Realizado</code>, <code>Mês</code> (AAAAMM, ex.: 202609) e <code>Loja</code> (obrigatórias);
          <code> DESCRIÇÃO</code> (opcional). <code>Ajustes</code> não é importado (as linhas com ajuste são só contadas); <code>Saldo</code> e <code>Grupo</code> são ignorados. Loja <code>01</code> e <code>1</code> são a mesma filial.
          <br /><strong>Sistema:</strong> <code>competencia</code> (AAAA-MM), <code>filial</code>, <code>conta</code>, <code>valor</code> (obrigatórias);
          <code> centro_custo, canal, conta_desc, tipo_doc, doc_ref, historico</code> (opcionais).
          <br />CSV com separador <code>;</code> ou <code>,</code>, ou XLSX (primeira planilha com o cabeçalho); valor aceita 1.234,56. A carga é tudo-ou-nada: se uma linha for inválida, nada é gravado. Reenviar o arquivo substitui os lançamentos de cada competência e filial presentes nele (os de outras origens ficam). Competência fechada é recusada.
        </p>
        <input type="file" accept=".csv,.xlsx,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" onChange={e => setArquivo(e.target.files?.[0] ?? null)} className="text-sm" />
        <div><Button size="sm" disabled={!arquivo || enviando} onClick={enviar}><Upload className="h-4 w-4 mr-1.5" />{enviando ? 'Enviando…' : 'Importar'}</Button></div>
      </div>
    </div>
  )
}
