import { useCallback, useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Database } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { api } from './util'

interface Item { competencia: string; origem: string; status: string; linhas: number; mensagem?: string }
interface Execucao {
  id: number; gatilho: string; de: string; ate: string; inicio: string; fim?: string
  status: string; mensagem: string; itens: Item[]
}
interface Estado { configurado: boolean; motivo: string; observacoes: string[]; max_meses: number; execucoes: Execucao[] }

const ROTULO: Record<string, string> = {
  executando: 'Em andamento', concluida: 'Concluída', concluida_com_ressalvas: 'Concluída com ressalvas', falha: 'Falha',
  aplicada: 'Aplicada', pulada_mes_fechado: 'Pulada: mês fechado', sem_dados: 'Sem dados', rejeitada: 'Rejeitada',
}
const COR: Record<string, string> = {
  concluida: 'text-green-700', aplicada: 'text-green-700', falha: 'text-red-700', rejeitada: 'text-red-700',
  concluida_com_ressalvas: 'text-amber-700', pulada_mes_fechado: 'text-amber-700', sem_dados: 'text-amber-700',
}
const quando = (s: string) => new Date(s).toLocaleString('pt-BR')
const mesAnterior = () => {
  const d = new Date(); d.setDate(1); d.setMonth(d.getMonth() - 1)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
}

// Importar do banco do cliente (história 2.5). A importação segue no servidor
// depois do clique; a lista é relida até ela terminar.
export default function ImportarERP() {
  const [estado, setEstado] = useState<Estado | null>(null)
  const [de, setDe] = useState(mesAnterior)
  const [ate, setAte] = useState(mesAnterior)
  const [enviando, setEnviando] = useState(false)
  const [erro, setErro] = useState('')

  const ler = useCallback(async () => {
    try { setEstado(await api<Estado>('/api/aud/importacao-erp')) } catch (e: any) { setErro(e.message) }
  }, [])
  useEffect(() => { ler() }, [ler])

  const andando = !!estado?.execucoes.some(e => e.status === 'executando')
  useEffect(() => {
    if (!andando) return
    const t = setInterval(ler, 4000)
    return () => clearInterval(t)
  }, [andando, ler])

  async function importar() {
    setEnviando(true); setErro('')
    try {
      await api('/api/aud/importacao-erp', { method: 'POST', body: JSON.stringify({ de, ate }), headers: { 'Content-Type': 'application/json' } })
      toast.success('Importação iniciada')
      await ler()
    } catch (e: any) { setErro(e.message) } finally { setEnviando(false) }
  }

  if (!estado) return erro ? <p className="text-sm text-red-700">{erro}</p> : null
  return (
    <div className="rounded-lg border bg-white p-4 space-y-3">
      <h2 className="text-sm font-semibold">Importar do banco do cliente</h2>
      {!estado.configurado
        ? <p className="text-xs text-muted-foreground">Indisponível: {estado.motivo}.</p>
        : <>
          <ul className="list-disc pl-5 text-xs text-muted-foreground">
            {estado.observacoes.map(o => <li key={o}>{o}</li>)}
            <li>Reimportar substitui o mês inteiro vindo do banco. Mês fechado é pulado. Até {estado.max_meses} meses por vez.</li>
            <li>Um mês que já tem as mesmas contas carregadas por arquivo é rejeitado, para não somar em dobro.</li>
          </ul>
          <div className="flex flex-wrap items-end gap-2 text-sm">
            <label className="flex flex-col text-xs">De<input type="month" value={de} onChange={e => setDe(e.target.value)} className="rounded border px-2 py-1 text-sm" /></label>
            <label className="flex flex-col text-xs">Até<input type="month" value={ate} onChange={e => setAte(e.target.value)} className="rounded border px-2 py-1 text-sm" /></label>
            <Button size="sm" disabled={enviando || andando || !de || !ate} onClick={importar}>
              <Database className="h-4 w-4 mr-1.5" />{andando ? 'Importando…' : 'Importar agora'}
            </Button>
          </div>
        </>}
      {erro && <pre className="whitespace-pre-wrap rounded-md border border-red-200 bg-red-50 p-2 text-xs text-red-700">{erro}</pre>}
      {estado.execucoes.length > 0 && (
        <div className="space-y-2">
          <h3 className="text-xs font-semibold uppercase tracking-wide">Últimas importações</h3>
          {estado.execucoes.slice(0, 10).map((e, i) => (
            <details key={e.id} open={i === 0} className="rounded border p-2 text-xs">
              <summary className="cursor-pointer">
                <span className={`font-semibold ${COR[e.status] ?? ''}`}>{ROTULO[e.status] ?? e.status}</span>
                {' · '}{e.de === e.ate ? e.de : `${e.de} a ${e.ate}`} · início {quando(e.inicio)}{e.fim ? ` · fim ${quando(e.fim)}` : ''}
              </summary>
              {e.mensagem && <p className="mt-1">{e.mensagem}</p>}
              <ul className="mt-1 space-y-1">
                {e.itens.map(it => (
                  <li key={it.competencia + it.origem}>
                    {it.competencia} · {it.origem} · <span className={COR[it.status] ?? 'text-red-700'}>{ROTULO[it.status] ?? it.status}</span>
                    {it.status === 'aplicada' ? ` (${it.linhas.toLocaleString('pt-BR')} linhas)` : ''}
                    {it.mensagem ? <span className="block whitespace-pre-wrap text-muted-foreground">{it.mensagem}</span> : null}
                  </li>
                ))}
              </ul>
            </details>
          ))}
        </div>
      )}
    </div>
  )
}
