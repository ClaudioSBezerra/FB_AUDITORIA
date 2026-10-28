import { useQuery } from '@tanstack/react-query'
import { api } from './util'

interface F { id: number; competencia: string; versao: number; filial: string; fechado_por: string; fechado_em: string; observacao: string; ativo: boolean; reaberto_em: string; reaberto_por: string; motivo_reabertura: string }

export default function DREFechamentos() {
  const q = useQuery({ queryKey: ['dre-fechamentos'], queryFn: () => api<F[]>('/api/aud/dre/fechamentos') })
  return (
    <div className="max-w-4xl space-y-3">
      <p className="text-sm text-muted-foreground">Cada fechamento congela os números do mês. O fechamento é do grupo inteiro. Para refazer, reabra o período com um motivo; o novo fechamento cria uma nova versão e as anteriores permanecem.</p>
      <div className="rounded-lg border bg-white overflow-hidden">
        <table className="w-full text-sm">
          <thead><tr className="border-b bg-gray-50 text-xs text-muted-foreground">
            {['Competência', 'Versão', 'Filial', 'Fechado por', 'Em', 'Observação', 'Situação'].map(h => <th key={h} className="text-left p-2.5 font-medium">{h}</th>)}</tr></thead>
          <tbody>
            {(q.data ?? []).map(f => (
              <tr key={f.id} className="border-b last:border-0">
                <td className="p-2.5">{f.competencia}</td><td className="p-2.5">v{f.versao}</td>
                <td className="p-2.5">{f.filial || 'Consolidado'}</td><td className="p-2.5">{f.fechado_por}</td>
                <td className="p-2.5 whitespace-nowrap">{new Date(f.fechado_em).toLocaleString('pt-BR')}</td>
                <td className="p-2.5">{f.observacao}</td>
                <td className="p-2.5">{f.ativo ? 'Ativo' : `Reaberto${f.reaberto_em ? ` em ${new Date(f.reaberto_em).toLocaleString('pt-BR')}` : ''}${f.reaberto_por ? ` por ${f.reaberto_por}` : ''}: ${f.motivo_reabertura}`}</td>
              </tr>
            ))}
            {q.data?.length === 0 && <tr><td colSpan={7} className="p-6 text-center text-muted-foreground">Nenhum período fechado.</td></tr>}
          </tbody>
        </table>
      </div>
    </div>
  )
}
