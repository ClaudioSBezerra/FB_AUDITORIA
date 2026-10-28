import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useAuth } from '@/contexts/AuthContext'
import { api } from './util'

interface Linha { codigo: string; descricao: string; tipo: string; soma_de: string[]; papel?: string }
interface Mapa { conta_padrao: string; linha_codigo: string; sinal: number }

// De-para conta contábil/gerencial → linha do DRE. A controladoria ajusta aqui,
// sem deploy. Padrão com '%' casa por prefixo; o mais específico vence.
export default function DREMapa() {
  const { perfil, isAdmin } = useAuth()
  const qc = useQueryClient()
  const editar = isAdmin || perfil === 'controladoria'
  const linhas = useQuery({ queryKey: ['dre-linhas'], queryFn: () => api<Linha[]>('/api/aud/dre/linhas') })
  const mapa = useQuery({ queryKey: ['dre-mapa'], queryFn: () => api<Mapa[]>('/api/aud/dre/mapa') })
  const [conta, setConta] = useState('')
  const [linha, setLinha] = useState('')
  const [sinal, setSinal] = useState(1)
  const contas = (linhas.data ?? []).filter(l => l.tipo === 'conta')

  async function salvar() {
    try {
      await api('/api/aud/dre/mapa', {
        method: 'PUT', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ conta_padrao: conta, linha_codigo: linha || contas[0]?.codigo, sinal }),
      })
      setConta(''); qc.invalidateQueries({ queryKey: ['dre-mapa'] }); toast.success('Mapeamento salvo')
    } catch (e: any) { toast.error(e.message) }
  }
  async function marcarFaturamento(codigo: string, marcar: boolean) {
    try {
      await api('/api/aud/dre/linhas/papel', {
        method: 'PUT', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ codigo, papel: marcar ? 'faturamento_liquido' : '' }),
      })
      qc.invalidateQueries({ queryKey: ['dre-linhas'] }); qc.invalidateQueries({ queryKey: ['dre'] })
      toast.success(marcar ? 'Base dos percentuais definida' : 'Marca removida')
    } catch (e: any) { toast.error(e.message) }
  }
  async function remover(c: string) {
    try {
      await api(`/api/aud/dre/mapa?conta_padrao=${encodeURIComponent(c)}`, { method: 'DELETE' })
      qc.invalidateQueries({ queryKey: ['dre-mapa'] })
    } catch (e: any) { toast.error(e.message) }
  }

  return (
    <div className="grid gap-6 lg:grid-cols-[1fr_320px] items-start">
      <div className="space-y-3">
        <h2 className="text-sm font-semibold">De-para de contas</h2>
        {editar && (
          <div className="flex flex-wrap items-end gap-2 rounded-lg border bg-white p-3">
            <div><label className="text-xs text-muted-foreground block mb-1">Conta (use % para prefixo)</label>
              <Input className="h-9 w-48" placeholder="3.1.01.%" value={conta} onChange={e => setConta(e.target.value)} /></div>
            <div><label className="text-xs text-muted-foreground block mb-1">Linha do DRE</label>
              <select className="h-9 rounded-md border px-2 text-sm bg-white" value={linha || contas[0]?.codigo || ''} onChange={e => setLinha(e.target.value)}>
                {contas.map(l => <option key={l.codigo} value={l.codigo}>{l.descricao}</option>)}
              </select></div>
            <div><label className="text-xs text-muted-foreground block mb-1">Sinal</label>
              <select className="h-9 rounded-md border px-2 text-sm bg-white" value={sinal} onChange={e => setSinal(+e.target.value)}>
                <option value={1}>+ (mantém)</option><option value={-1}>− (inverte)</option>
              </select></div>
            <Button size="sm" disabled={!conta.trim()} onClick={salvar}>Salvar</Button>
          </div>
        )}
        <div className="rounded-lg border bg-white overflow-hidden">
          <table className="w-full text-sm">
            <thead><tr className="border-b bg-gray-50 text-xs text-muted-foreground">
              <th className="text-left p-2.5 font-medium">Conta</th><th className="text-left p-2.5 font-medium">Linha</th>
              <th className="text-left p-2.5 font-medium">Sinal</th><th /></tr></thead>
            <tbody>
              {(mapa.data ?? []).map(m => (
                <tr key={m.conta_padrao} className="border-b last:border-0">
                  <td className="p-2.5 font-mono text-xs">{m.conta_padrao}</td>
                  <td className="p-2.5">{contas.find(l => l.codigo === m.linha_codigo)?.descricao ?? m.linha_codigo}</td>
                  <td className="p-2.5">{m.sinal > 0 ? '+' : '−'}</td>
                  <td className="p-2.5 text-right">{editar && <button onClick={() => remover(m.conta_padrao)} className="text-muted-foreground hover:text-red-600"><Trash2 className="h-4 w-4" /></button>}</td>
                </tr>
              ))}
              {mapa.data?.length === 0 && <tr><td colSpan={4} className="p-6 text-center text-muted-foreground">Nenhuma conta mapeada ainda.</td></tr>}
            </tbody>
          </table>
        </div>
      </div>
      <div className="space-y-3">
        <h2 className="text-sm font-semibold">Estrutura do DRE</h2>
        <ol className="rounded-lg border bg-white divide-y text-sm">
          {(linhas.data ?? []).map(l => (
            <li key={l.codigo} className={`p-2.5 ${l.tipo === 'subtotal' ? 'bg-gray-50 font-medium' : ''}`}>
              {l.descricao}
              {l.papel === 'faturamento_liquido' && <span className="ml-2 text-[11px] rounded-full bg-sky-50 text-sky-700 border border-sky-200 px-2 py-0.5 font-normal">Faturamento Líquido</span>}
              {editar && (
                <button className="ml-2 text-[11px] font-normal text-muted-foreground hover:text-foreground underline"
                  onClick={() => marcarFaturamento(l.codigo, l.papel !== 'faturamento_liquido')}>
                  {l.papel === 'faturamento_liquido' ? 'desmarcar' : 'usar como Faturamento Líquido'}
                </button>
              )}
              {l.tipo === 'subtotal' && <span className="block text-[11px] text-muted-foreground font-normal">{l.soma_de.join(' ').replace(/ -/g, ' − ')}</span>}
            </li>
          ))}
        </ol>
      </div>
    </div>
  )
}
