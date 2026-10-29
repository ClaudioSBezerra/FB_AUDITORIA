import { useState, type ReactNode } from 'react'
import { toast } from 'sonner'
import { Upload, Download } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { api } from './util'

interface Props<R> {
  titulo: string
  descricao: ReactNode
  url: string
  modelo: string        // CSV de exemplo (fictício), gerado no navegador
  nomeModelo: string
  resumo: (r: R) => ReactNode
  aoCarregar: () => void
}

// Bloco de carga por arquivo (CSV ou XLSX) das telas de cadastro (histórias 2.20 e 2.21):
// envia o arquivo, mostra o resumo ou o erro (lista de linhas com problema) e baixa um modelo.
export default function BlocoCargaArquivo<R>({ titulo, descricao, url, modelo, nomeModelo, resumo, aoCarregar }: Props<R>) {
  const [arquivo, setArquivo] = useState<File | null>(null)
  const [ocupado, setOcupado] = useState(false)
  const [resultado, setResultado] = useState<R | null>(null)
  const [erro, setErro] = useState('')

  async function enviar() {
    if (!arquivo || ocupado) return
    setOcupado(true); setErro(''); setResultado(null)
    try {
      const fd = new FormData(); fd.append('arquivo', arquivo)
      const r = await api<R>(url, { method: 'POST', body: fd })
      setResultado(r); setArquivo(null); aoCarregar(); toast.success('Carga concluída')
    } catch (e: any) { setErro(e.message) } finally { setOcupado(false) }
  }

  function baixarModelo() {
    const u = window.URL.createObjectURL(new Blob(['﻿' + modelo + '\n'], { type: 'text/csv;charset=utf-8' }))
    const a = document.createElement('a')
    a.href = u; a.download = nomeModelo; a.click()
    window.URL.revokeObjectURL(u)
  }

  return (
    <div className="rounded-lg border bg-white p-3 space-y-2">
      <h2 className="text-sm font-semibold">{titulo}</h2>
      <p className="text-xs text-muted-foreground">{descricao}</p>
      <input key={resultado ? 'ok' : 'novo'} type="file" accept=".csv,.xlsx,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
        onChange={e => setArquivo(e.target.files?.[0] ?? null)} className="text-sm" />
      <div className="flex flex-wrap gap-2">
        <Button size="sm" disabled={!arquivo || ocupado} onClick={enviar}><Upload className="h-4 w-4 mr-1.5" />{ocupado ? 'Enviando…' : 'Carregar'}</Button>
        <Button size="sm" variant="outline" onClick={baixarModelo}><Download className="h-4 w-4 mr-1.5" />Baixar modelo CSV</Button>
      </div>
      {erro && <pre className="whitespace-pre-wrap rounded-md border border-red-200 bg-red-50 p-2 text-xs text-red-700">{erro}</pre>}
      {resultado && <p className="rounded-md border bg-gray-50 p-2 text-xs">{resumo(resultado)}</p>}
    </div>
  )
}
