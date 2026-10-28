import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from './util'

// Recorte de filiais das telas do DRE: o Grupo (todas), uma filial do ERP ou um
// agrupamento. O valor é 'g' (Grupo), 'f:<filial>' ou 'a:<agrupamento>'.
// Os agrupamentos vêm de /api/aud/agrupamentos-filiais; se o perfil não puder
// lê-los, a lista mostra só o Grupo e as filiais.

export function paramRecorte(valor: string): string {
  if (valor.startsWith('f:')) return `filial=${encodeURIComponent(valor.slice(2))}`
  if (valor.startsWith('a:')) return `agrupamento=${encodeURIComponent(valor.slice(2))}`
  return 'grupo=1'
}

export function useRecorte() {
  const [recorte, setRecorte] = useState('g')
  return { recorte, setRecorte, param: paramRecorte(recorte) }
}

interface Agrupamento { codigo: string; nome: string }

export function SeletorRecorte({ valor, onChange, somenteFilial = false }: { valor: string; onChange: (v: string) => void; somenteFilial?: boolean }) {
  const filiais = useQuery({ queryKey: ['dre-filiais'], queryFn: () => api<string[]>('/api/aud/dre/filiais') })
  const agrupamentos = useQuery({
    queryKey: ['agrupamentos-filiais-recorte'], enabled: !somenteFilial, retry: false,
    queryFn: () => api<Agrupamento[]>('/api/aud/agrupamentos-filiais'),
  })
  const agrs = somenteFilial || agrupamentos.isError ? [] : agrupamentos.data ?? []
  return (
    <div>
      <label className="text-xs text-muted-foreground block mb-1">Recorte</label>
      <select className="h-9 rounded-md border px-2 text-sm bg-white" value={valor} onChange={e => onChange(e.target.value)}>
        <option value="g">Grupo (todas as filiais)</option>
        {agrs.length > 0 && (
          <optgroup label="Agrupamentos">
            {agrs.map(a => <option key={a.codigo} value={`a:${a.codigo}`}>{a.codigo} · {a.nome}</option>)}
          </optgroup>
        )}
        <optgroup label="Filiais">
          {(filiais.data ?? []).map(f => <option key={f} value={`f:${f}`}>{f}</option>)}
        </optgroup>
      </select>
    </div>
  )
}
