import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Clock } from 'lucide-react'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from '@/components/ui/dialog'
import { cn } from '@/lib/utils'
import { api } from './util'

// Acompanhamento do projeto (história 4.10): Kanban somente leitura. Os dados vêm
// de /api/aud/acompanhamento (JSON embutido no backend); não há nenhuma escrita.

interface Historia {
  id: string
  titulo: string
  epico: { numero: number; nome: string }
  release: string
  status: string
  bloqueada_cliente: boolean
  motivo_bloqueio?: string
  resumo: string
  criterios: string[]
}
interface Dados { gerado_em: string; versao: string; historias: Historia[] }

const COLUNAS = ['Backlog', 'Pronto', 'Em andamento', 'Em revisão', 'Feito', 'Aguardando o cliente'] as const
const COR_COLUNA: Record<string, string> = {
  'Backlog': 'bg-gray-400',
  'Pronto': 'bg-sky-500',
  'Em andamento': 'bg-amber-500',
  'Em revisão': 'bg-violet-500',
  'Feito': 'bg-emerald-500',
  'Aguardando o cliente': 'bg-orange-500',
}

const norm = (s: string) => s.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase()
const avancada = (h: Historia) => h.status === 'Feito' || h.status === 'Em revisão'
const pct = (a: number, b: number) => (b === 0 ? 0 : Math.round((a / b) * 100))

function dataBR(iso: string) {
  const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(iso)
  return m ? `${m[3]}/${m[2]}/${m[1]}` : iso
}

function Barra({ valor }: { valor: number }) {
  return (
    <div className="h-2 w-full rounded-full bg-gray-100 overflow-hidden" role="progressbar" aria-valuenow={valor} aria-valuemin={0} aria-valuemax={100}>
      <div className="h-full bg-primary transition-all" style={{ width: `${valor}%` }} />
    </div>
  )
}

function SeloCliente({ pequeno }: { pequeno?: boolean }) {
  return (
    <span className={cn('inline-flex items-center gap-1 rounded-full border border-amber-300 bg-amber-50 text-amber-800 font-medium', pequeno ? 'px-1.5 py-0.5 text-[10px]' : 'px-2 py-0.5 text-xs')}>
      <Clock className={pequeno ? 'h-2.5 w-2.5' : 'h-3 w-3'} />
      Aguardando o cliente
    </span>
  )
}

export default function Acompanhamento() {
  const q = useQuery({ queryKey: ['aud-acompanhamento'], queryFn: () => api<Dados>('/api/aud/acompanhamento'), staleTime: 60_000 })
  const [epico, setEpico] = useState('')
  const [busca, setBusca] = useState('')
  const [aberta, setAberta] = useState<Historia | null>(null)

  const historias = q.data?.historias ?? []
  const epicos = useMemo(() => {
    const m = new Map<number, { numero: number; nome: string; total: number; avancadas: number }>()
    for (const h of historias) {
      const e = m.get(h.epico.numero) ?? { ...h.epico, total: 0, avancadas: 0 }
      e.total++
      if (avancada(h)) e.avancadas++
      m.set(h.epico.numero, e)
    }
    return [...m.values()].sort((a, b) => a.numero - b.numero)
  }, [historias])

  const filtradas = useMemo(() => {
    const t = norm(busca.trim())
    return historias.filter(h =>
      (!epico || String(h.epico.numero) === epico) &&
      (!t || norm(`${h.id} ${h.titulo} ${h.resumo}`).includes(t)))
  }, [historias, epico, busca])

  if (q.isLoading) return <p className="text-sm text-muted-foreground">Carregando...</p>
  if (q.isError) return <p className="text-sm text-red-600">Não foi possível carregar o acompanhamento: {(q.error as Error).message}</p>

  const totalAvancadas = historias.filter(avancada).length

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-2">
        <div>
          <h1 className="text-xl font-semibold">Acompanhamento do projeto</h1>
          <p className="text-xs text-muted-foreground">
            Release {historias[0]?.release ?? ''} · Atualizado em {q.data ? dataBR(q.data.gerado_em) : ''}
            {q.data?.versao ? ` · versão ${q.data.versao}` : ''}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <input type="search" value={busca} onChange={e => setBusca(e.target.value)} placeholder="Buscar por texto ou número"
            aria-label="Buscar" className="h-8 w-56 max-w-full rounded-md border px-2 text-sm bg-white" />
          <select value={epico} onChange={e => setEpico(e.target.value)} aria-label="Filtrar por épico"
            className="h-8 max-w-full rounded-md border px-2 text-sm bg-white">
            <option value="">Todos os épicos</option>
            {epicos.map(e => <option key={e.numero} value={e.numero}>Épico {e.numero}: {e.nome}</option>)}
          </select>
        </div>
      </div>

      <div className="rounded-lg border bg-white p-3 space-y-3">
        <div>
          <div className="flex justify-between text-xs mb-1">
            <span className="font-medium">Progresso geral</span>
            <span className="text-muted-foreground">{totalAvancadas} de {historias.length} histórias feitas ou em revisão ({pct(totalAvancadas, historias.length)}%)</span>
          </div>
          <Barra valor={pct(totalAvancadas, historias.length)} />
        </div>
        <div className="grid gap-x-6 gap-y-2 sm:grid-cols-2 lg:grid-cols-4">
          {epicos.map(e => (
            <div key={e.numero}>
              <div className="flex justify-between gap-2 text-[11px] mb-0.5">
                <span className="truncate" title={e.nome}>Épico {e.numero}: {e.nome}</span>
                <span className="text-muted-foreground shrink-0">{e.avancadas}/{e.total}</span>
              </div>
              <Barra valor={pct(e.avancadas, e.total)} />
            </div>
          ))}
        </div>
      </div>

      <div className="flex gap-3 overflow-x-auto pb-3 snap-x">
        {COLUNAS.map(col => {
          // Histórias que dependem do cliente ficam só na coluna própria; o status
          // original aparece no cartão.
          const cards = filtradas.filter(h => col === 'Aguardando o cliente' ? h.bloqueada_cliente : !h.bloqueada_cliente && h.status === col)
          return (
            <section key={col} className="snap-start shrink-0 w-[270px] flex-1 min-w-[270px] rounded-lg bg-gray-50 border p-2" aria-label={col}>
              <header className="flex items-center justify-between px-1 pb-2">
                <h2 className="flex items-center gap-1.5 text-sm font-semibold">
                  <span className={cn('h-2 w-2 rounded-full', COR_COLUNA[col])} />
                  {col}
                </h2>
                <span className="text-xs rounded-full bg-white border px-2 py-0.5 text-muted-foreground">{cards.length}</span>
              </header>
              <div className="space-y-2 max-h-[60vh] overflow-y-auto">
                {cards.map(h => (
                  <button key={h.id} type="button" onClick={() => setAberta(h)}
                    className="block w-full text-left rounded-md border bg-white p-2.5 hover:border-primary/50 transition-colors">
                    <div className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
                      <span className="font-mono font-semibold text-foreground">{h.id}</span>
                      <span className="truncate" title={h.epico.nome}>Épico {h.epico.numero}</span>
                    </div>
                    <p className="text-sm mt-0.5 leading-snug">{h.titulo}</p>
                    {h.bloqueada_cliente && <p className="mt-1 text-[11px] text-muted-foreground">Situação: {h.status}</p>}
                  </button>
                ))}
                {cards.length === 0 && <p className="text-xs text-muted-foreground px-1 py-2">Nenhuma história.</p>}
              </div>
            </section>
          )
        })}
      </div>

      <Dialog open={aberta != null} onOpenChange={o => { if (!o) setAberta(null) }}>
        <DialogContent className="max-w-2xl max-h-[85vh] overflow-y-auto">
          {aberta && (
            <>
              <DialogHeader>
                <DialogTitle>{aberta.id} · {aberta.titulo}</DialogTitle>
                <DialogDescription>
                  Épico {aberta.epico.numero}: {aberta.epico.nome} · Release {aberta.release} · {aberta.status}
                </DialogDescription>
              </DialogHeader>
              <div className="space-y-4 text-sm">
                {aberta.bloqueada_cliente && (
                  <div className="space-y-1">
                    <SeloCliente />
                    {aberta.motivo_bloqueio && <p className="text-xs text-muted-foreground">{aberta.motivo_bloqueio}</p>}
                  </div>
                )}
                {aberta.resumo && <p>{aberta.resumo}</p>}
                {aberta.criterios.length > 0 && (
                  <div>
                    <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground mb-2">Critérios de aceite</h3>
                    <ol className="space-y-2">
                      {aberta.criterios.map((c, i) => (
                        <li key={i} className="rounded-md border bg-gray-50 p-2 text-xs whitespace-pre-line leading-relaxed">{c}</li>
                      ))}
                    </ol>
                  </div>
                )}
              </div>
            </>
          )}
        </DialogContent>
      </Dialog>
    </div>
  )
}
