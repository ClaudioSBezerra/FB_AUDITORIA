import { useRef, useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { useAuth } from '@/contexts/AuthContext'
import { api } from './aud/util'

const FRASE = 'VIRAR PARA PRODUCAO'

// Administração do ambiente da empresa ativa (Teste/Produção). O backend recusa
// tudo igualmente; esta tela só reflete o estado.
export default function ConfigAmbienteEmpresa() {
  const { ambienteEmpresa, company, recarregarAcesso } = useAuth()
  const [virou, setVirou] = useState(false)
  const emTeste = ambienteEmpresa === 'teste' && !virou
  const emProducao = ambienteEmpresa === 'producao' || virou
  const trava = useRef(false)
  const [senhaLimpar, setSenhaLimpar] = useState('')
  const [senhaVirar, setSenhaVirar] = useState('')
  const [frase, setFrase] = useState('')
  const [ocupado, setOcupado] = useState(false)
  const [contas, setContas] = useState<string[] | null>(null)

  async function post<T>(url: string, corpo: object): Promise<T> {
    return api<T>(url, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(corpo) })
  }

  async function limpar() {
    if (trava.current) return
    if (!window.confirm('Apagar todos os dados de teste desta empresa? Isso não pode ser desfeito.')) return
    trava.current = true
    setOcupado(true)
    try {
      await post('/api/aud/ambiente/limpar', { senha: senhaLimpar })
      toast.success('Dados de teste apagados'); setSenhaLimpar('')
    } catch (e: any) { toast.error(e.message) } finally { trava.current = false; setOcupado(false) }
  }

  async function virar() {
    if (trava.current) return
    trava.current = true
    setOcupado(true)
    try {
      const r = await post<{ contas_de_arquivo: string[] }>('/api/aud/ambiente/virar', { senha: senhaVirar, confirmacao: frase })
      setContas(r.contas_de_arquivo ?? [])
      setVirou(true)
      toast.success('Empresa agora está em Produção')
      setSenhaVirar(''); setFrase(''); recarregarAcesso()
    } catch (e: any) {
      toast.error(e.message)
      if (/já está em Produção/.test(e.message)) { setVirou(true); recarregarAcesso() }
    } finally { trava.current = false; setOcupado(false) }
  }

  return (
    <div className="max-w-2xl space-y-4">
      <div className="rounded-lg border bg-white p-4 space-y-1">
        <h2 className="text-sm font-semibold">Ambiente de {company ?? 'empresa'}</h2>
        <p className="text-sm">{emProducao ? 'Produção' : emTeste ? 'Teste' : 'Carregando…'}</p>
      </div>

      {contas && contas.length > 0 && (
        <div className="rounded-lg border border-amber-300 bg-amber-50 p-4 space-y-1 text-sm">
          <p className="font-semibold">Atenção: {contas.length} conta(s) ainda têm lançamentos vindos de arquivo.</p>
          <p className="text-xs">{contas.join(', ')}</p>
        </div>
      )}

      {emTeste && (
        <>
          <div className="rounded-lg border bg-white p-4 space-y-3">
            <h2 className="text-sm font-semibold">Apagar dados de teste</h2>
            <p className="text-xs text-muted-foreground">
              Apaga lançamentos, cargas, orçado e fechamentos. Mantém a estrutura do DRE, o Mapa e os usuários; a trilha nunca é apagada.
            </p>
            <input type="password" value={senhaLimpar} onChange={e => setSenhaLimpar(e.target.value)} placeholder="Sua senha" className="border rounded px-2 py-1 text-sm block" />
            <Button size="sm" variant="destructive" disabled={!senhaLimpar || ocupado} onClick={limpar}>Apagar dados de teste</Button>
          </div>

          <div className="rounded-lg border bg-white p-4 space-y-3">
            <h2 className="text-sm font-semibold">Virar para Produção</h2>
            <p className="text-xs text-muted-foreground">
              A virada é irreversível. Informe a senha e digite <code>{FRASE}</code>.
            </p>
            <input type="password" value={senhaVirar} onChange={e => setSenhaVirar(e.target.value)} placeholder="Sua senha" className="border rounded px-2 py-1 text-sm block" />
            <input type="text" value={frase} onChange={e => setFrase(e.target.value)} placeholder={FRASE} className="border rounded px-2 py-1 text-sm block" />
            <Button size="sm" disabled={!senhaVirar || frase !== FRASE || ocupado} onClick={virar}>Virar para Produção</Button>
          </div>
        </>
      )}
    </div>
  )
}
