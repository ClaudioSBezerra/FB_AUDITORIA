import { useState } from 'react'
import { toast } from 'sonner'
import { Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { api } from './util'

interface Item { competencia: string; origem: string; linhas: number }
interface Retirada {
  competencia: string; itens: Item[]; linhas: number; fechadas: string[]
  ambiente: string; confirmacao: string; contas_liberadas: number
}

// Retirar a movimentação carregada, de um mês ou toda (história 2.23). Primeiro
// mostra o que será apagado; só depois pede a confirmação digitada.
export default function RetirarMovimentacao() {
  const [mes, setMes] = useState('')
  const [previa, setPrevia] = useState<Retirada | null>(null)
  const [texto, setTexto] = useState('')
  const [ocupado, setOcupado] = useState(false)
  const [erro, setErro] = useState('')

  const limpar = () => { setPrevia(null); setTexto(''); setErro('') }

  async function conferir(competencia: string) {
    limpar(); setOcupado(true)
    try { setPrevia(await api<Retirada>(`/api/aud/movimentacao/retirada?competencia=${competencia}`)) }
    catch (e: any) { setErro(e.message) } finally { setOcupado(false) }
  }

  async function retirar() {
    setOcupado(true); setErro('')
    try {
      const r = await api<Retirada>('/api/aud/movimentacao/retirada', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ competencia: previa!.competencia, confirmacao: texto }),
      })
      toast.success(`${r.linhas.toLocaleString('pt-BR')} lançamentos retirados`)
      limpar()
    } catch (e: any) { setErro(e.message) } finally { setOcupado(false) }
  }

  const tudoEmProducao = previa && !previa.competencia && previa.ambiente !== 'teste'
  const impedido = !previa || previa.linhas === 0 || previa.fechadas.length > 0 || !!tudoEmProducao
  return (
    <div className="rounded-lg border bg-white p-4 space-y-3">
      <h2 className="text-sm font-semibold">Retirar movimentação carregada</h2>
      <p className="text-xs text-muted-foreground">Apaga os lançamentos para receber uma carga nova. Estrutura, Mapa, departamentos, dicionário, filiais, orçado e planilha do comparativo ficam. Mês fechado não é apagado.</p>
      <div className="flex flex-wrap items-end gap-2 text-sm">
        <label className="flex flex-col text-xs">Mês<input type="month" value={mes} onChange={e => { setMes(e.target.value); limpar() }} className="rounded border px-2 py-1 text-sm" /></label>
        <Button size="sm" variant="outline" disabled={ocupado || !mes} onClick={() => conferir(mes)}>Ver o mês</Button>
        <Button size="sm" variant="outline" disabled={ocupado} onClick={() => { setMes(''); conferir('') }}>Ver tudo</Button>
      </div>
      {erro && <p className="text-xs text-red-700">{erro}</p>}
      {previa && (
        <div className="rounded border border-red-200 bg-red-50 p-3 space-y-2 text-xs">
          <p className="text-sm font-semibold">{previa.linhas === 0 ? 'Nada a retirar' : `Serão apagados ${previa.linhas.toLocaleString('pt-BR')} lançamentos`} · {previa.competencia || 'todos os meses'}</p>
          {previa.itens.length > 0 && (
            <ul className="max-h-40 overflow-y-auto">
              {previa.itens.map(i => <li key={i.competencia + i.origem}>{i.competencia} · {i.origem} · {i.linhas.toLocaleString('pt-BR')} linhas</li>)}
            </ul>
          )}
          {previa.fechadas.length > 0 && <p className="text-red-700">Há mês fechado ({previa.fechadas.join(', ')}). Reabra o fechamento antes.</p>}
          {tudoEmProducao && <p className="text-red-700">Em produção só é possível retirar um mês por vez.</p>}
          {!impedido && (
            <div className="flex flex-wrap items-end gap-2">
              <label className="flex flex-col">Digite <b>{previa.confirmacao}</b> para confirmar<input value={texto} onChange={e => setTexto(e.target.value)} className="rounded border px-2 py-1 text-sm" /></label>
              <Button size="sm" variant="destructive" disabled={ocupado || texto.trim() !== previa.confirmacao} onClick={retirar}><Trash2 className="h-4 w-4 mr-1.5" />Retirar</Button>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
