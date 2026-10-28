import { useEffect, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useAuth } from '@/contexts/AuthContext'
import { PERFIS } from '@/lib/perfis'
import { api } from './aud/util'
import { HierarchyCascadeSelects } from './AdminUsers'

// Novo usuário da empresa (história 4.9, FR-19): só o administrador de plataforma. O
// usuário recebe um link para definir a própria senha; sem SMTP, o link aparece aqui
// para o administrador repassar (vale 72 horas e uma só vez).

const AREAS: ReadonlyArray<readonly [string, string]> = [
  ['empresa', 'Administrativo'], ['configuracoes', 'Configurações'], ['comercial', 'Comercial'], ['operacao', 'Operação'], ['acompanhamento', 'Acompanhamento'],
]

interface Depto { id: string; codigo: string; nome: string; grupo: string }
interface Criado {
  id: string; email: string; email_enviado: boolean; validade_horas: number
  link?: string; caminho?: string; motivo_sem_envio?: string
}

const vazio = { nome: '', email: '', perfil: 'gestor', areas: ['empresa'] as string[] }

export default function NovoUsuarioEmpresa({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { token } = useAuth()
  const qc = useQueryClient()
  const [f, setF] = useState(vazio)
  const [envId, setEnvId] = useState('')
  const [groupId, setGroupId] = useState('')
  const [companyId, setCompanyId] = useState('')
  const [deptos, setDeptos] = useState<Depto[]>([])
  const [escolhidos, setEscolhidos] = useState<string[]>([])
  const [salvando, setSalvando] = useState(false)
  const [criado, setCriado] = useState<Criado | null>(null)

  useEffect(() => {
    setEscolhidos([])
    if (!companyId) { setDeptos([]); return }
    api<Depto[]>(`/api/admin/users/empresa?company_id=${companyId}`).then(setDeptos).catch(() => setDeptos([]))
  }, [companyId])

  function fechar() {
    setF(vazio); setEnvId(''); setGroupId(''); setCompanyId(''); setEscolhidos([]); setCriado(null)
    onClose()
  }

  async function criar() {
    if (!f.nome.trim() || !f.email.trim()) { toast.error('Informe nome e e-mail'); return }
    if (!companyId) { toast.error('Escolha a empresa'); return }
    if (f.areas.length === 0) { toast.error('Marque ao menos uma área'); return }
    setSalvando(true)
    try {
      const r = await api<Criado>('/api/admin/users/empresa', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          email: f.email.trim(), full_name: f.nome.trim(), perfil: f.perfil, areas: f.areas, company_id: companyId,
          ...(f.perfil === 'gestor' ? { departamentos: escolhidos } : {}),
        }),
      })
      setCriado(r)
      qc.invalidateQueries({ queryKey: ['admin-users'] })
    } catch (e: any) { toast.error(e.message) } finally { setSalvando(false) }
  }

  const link = criado ? (criado.link || (criado.caminho ? window.location.origin + criado.caminho : '')) : ''
  const alternar = (lista: string[], v: string) => lista.includes(v) ? lista.filter(x => x !== v) : [...lista, v]

  return (
    <Dialog open={open} onOpenChange={o => { if (!o) fechar() }}>
      <DialogContent className="max-w-lg max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Novo usuário da empresa</DialogTitle>
          <DialogDescription>O usuário define a própria senha por um link; nenhuma senha é informada aqui.</DialogDescription>
        </DialogHeader>

        {criado ? (
          <div className="space-y-3 text-sm">
            {criado.email_enviado ? (
              <p className="rounded-md border border-emerald-300 bg-emerald-50 p-3 text-emerald-900">
                Convite enviado para <strong>{criado.email}</strong>. O link vale {criado.validade_horas} horas e só pode ser usado uma vez.
              </p>
            ) : (
              <div className="space-y-2 rounded-md border border-amber-300 bg-amber-50 p-3 text-amber-900">
                <p>
                  O e-mail não foi enviado ({criado.motivo_sem_envio ?? 'sem envio'}). Repasse este link a <strong>{criado.email}</strong>;
                  ele vale {criado.validade_horas} horas e só pode ser usado uma vez.
                </p>
                <Input readOnly value={link} onFocus={e => e.currentTarget.select()} />
                <Button size="sm" variant="outline"
                  onClick={() => navigator.clipboard?.writeText(link).then(() => toast.success('Link copiado'), () => toast.error('Copie o link manualmente'))}>
                  Copiar link
                </Button>
              </div>
            )}
            <DialogFooter><Button onClick={fechar}>Concluir</Button></DialogFooter>
          </div>
        ) : (
          <>
            <div className="grid gap-3 py-2">
              <div className="grid grid-cols-4 items-center gap-4">
                <Label className="text-right">Nome</Label>
                <Input className="col-span-3" value={f.nome} onChange={e => setF({ ...f, nome: e.target.value })} />
              </div>
              <div className="grid grid-cols-4 items-center gap-4">
                <Label className="text-right">E-mail</Label>
                <Input className="col-span-3" type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} />
              </div>
              <div className="grid grid-cols-4 items-center gap-4">
                <Label className="text-right">Perfil</Label>
                <select className="col-span-3 h-9 rounded-md border px-2 text-sm bg-white" value={f.perfil} onChange={e => setF({ ...f, perfil: e.target.value })}>
                  {PERFIS.map(([v, r]) => <option key={v} value={v}>{r}</option>)}
                </select>
              </div>
              <div className="grid grid-cols-4 items-start gap-4">
                <Label className="text-right pt-1">Áreas</Label>
                <div className="col-span-3 flex flex-wrap gap-x-4 gap-y-1">
                  {AREAS.map(([v, r]) => (
                    <label key={v} className="flex items-center gap-1.5 text-sm">
                      <input type="checkbox" checked={f.areas.includes(v)} onChange={() => setF({ ...f, areas: alternar(f.areas, v) })} />{r}
                    </label>
                  ))}
                </div>
              </div>
              <div className="border-t pt-3">
                <Label className="text-sm font-semibold mb-2 block">Empresa</Label>
                {token && (
                  <HierarchyCascadeSelects token={token} envId={envId} groupId={groupId} companyId={companyId}
                    onEnvChange={setEnvId} onGroupChange={setGroupId} onCompanyChange={setCompanyId} />
                )}
              </div>
              {f.perfil === 'gestor' && (
                <div className="grid grid-cols-4 items-start gap-4">
                  <Label className="text-right pt-1">Departamentos</Label>
                  <div className="col-span-3 space-y-1">
                    {!companyId && <p className="text-xs text-muted-foreground">Escolha a empresa.</p>}
                    {companyId && deptos.length === 0 && <p className="text-xs text-muted-foreground">A empresa ainda não tem departamentos; o Gerente não verá nada até receber algum.</p>}
                    {deptos.map(d => (
                      <label key={d.id} className="flex items-center gap-1.5 text-sm">
                        <input type="checkbox" checked={escolhidos.includes(d.id)} onChange={() => setEscolhidos(alternar(escolhidos, d.id))} />
                        {d.codigo} · {d.nome}
                      </label>
                    ))}
                  </div>
                </div>
              )}
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={fechar}>Cancelar</Button>
              <Button onClick={criar} disabled={salvando}>{salvando ? 'Criando…' : 'Criar usuário'}</Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
