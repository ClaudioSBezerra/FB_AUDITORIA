import { useQuery } from '@tanstack/react-query'
import { api } from './util'

interface L { id: number; usuario: string; entidade: string; entidade_id: string; acao: string; payload: unknown; criado_em: string }

// Trilha imutável: somente leitura (o banco barra UPDATE/DELETE).
export default function AdmTrilha() {
  const q = useQuery({ queryKey: ['aud-trilha'], queryFn: () => api<L[]>('/api/aud/audit-log?limit=200') })
  return (
    <div className="rounded-lg border bg-white overflow-x-auto">
      <table className="w-full text-xs">
        <thead><tr className="border-b bg-gray-50 text-muted-foreground">
          {['Quando', 'Quem', 'Entidade', 'Ação', 'Detalhe'].map(h => <th key={h} className="text-left p-2.5 font-medium">{h}</th>)}</tr></thead>
        <tbody>
          {(q.data ?? []).map(l => (
            <tr key={l.id} className="border-b last:border-0 align-top">
              <td className="p-2.5 whitespace-nowrap">{new Date(l.criado_em).toLocaleString('pt-BR')}</td>
              <td className="p-2.5">{l.usuario}</td>
              <td className="p-2.5">{l.entidade}{l.entidade_id ? ` · ${l.entidade_id}` : ''}</td>
              <td className="p-2.5">{l.acao}</td>
              <td className="p-2.5 font-mono text-[11px] text-muted-foreground break-all">{l.payload ? JSON.stringify(l.payload) : ''}</td>
            </tr>
          ))}
          {q.data?.length === 0 && <tr><td colSpan={5} className="p-6 text-center text-muted-foreground">Sem registros.</td></tr>}
        </tbody>
      </table>
    </div>
  )
}
