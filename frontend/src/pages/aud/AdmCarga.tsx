import { useState } from 'react'
import { toast } from 'sonner'
import { Upload } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { api } from './util'

// Carga de lançamentos por CSV — usada até a ingestão incremental do Winthor
// existir; os dados entram na mesma tabela analítica.
export default function AdmCarga() {
  const [arquivo, setArquivo] = useState<File | null>(null)
  const [enviando, setEnviando] = useState(false)

  async function enviar() {
    if (!arquivo) return
    setEnviando(true)
    try {
      const fd = new FormData(); fd.append('arquivo', arquivo)
      const r = await api<{ linhas: number }>('/api/aud/dre/lancamentos/importar', { method: 'POST', body: fd })
      toast.success(`${r.linhas} lançamentos importados`); setArquivo(null)
    } catch (e: any) { toast.error(e.message) } finally { setEnviando(false) }
  }

  return (
    <div className="max-w-2xl space-y-4">
      <div className="rounded-lg border bg-white p-4 space-y-3">
        <h2 className="text-sm font-semibold">Importar lançamentos (CSV)</h2>
        <p className="text-xs text-muted-foreground">
          Colunas: <code>competencia</code> (AAAA-MM), <code>conta</code>, <code>valor</code> (obrigatórias, junto com <code>filial</code>);
          <code> centro_custo, canal, conta_desc, tipo_doc, doc_ref, historico</code> (opcionais).
          Separador <code>;</code> ou <code>,</code>; valor aceita 1.234,56. A carga é tudo-ou-nada: se uma linha for inválida ou sem filial, nada é gravado. Reenviar o arquivo substitui os lançamentos de cada competência e filial presentes nele (os de outras origens ficam).
        </p>
        <input type="file" accept=".csv,text/csv" onChange={e => setArquivo(e.target.files?.[0] ?? null)} className="text-sm" />
        <div><Button size="sm" disabled={!arquivo || enviando} onClick={enviar}><Upload className="h-4 w-4 mr-1.5" />{enviando ? 'Enviando…' : 'Importar'}</Button></div>
      </div>
    </div>
  )
}
