import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { Item, Shopping as ShoppingT } from '../lib/types'
import { daysUntil, qty } from '../lib/format'
import { Card, Empty, Loading, Modal, PageHeader, useAsync, useToast } from '../components/ui'
import { Avatars } from '../components/people'
import { Icon } from '../components/icons'
import { announceChange, useOnChange } from '../components/events'

export default function Shopping() {
  const { settings } = useApp()
  const { toast } = useToast()
  const navigate = useNavigate()
  const data = useAsync(() => api.get<ShoppingT>(`${V1}/shopping`), [])
  const [restock, setRestock] = useState<Item | null>(null)
  useOnChange(() => { data.reload() })

  const bought = async (it: Item, amount: number, note = 'bought') => {
    try { await api.post(`${V1}/items/${it.id}/adjust`, { delta: amount, note }); toast(`${it.name}: +${qty(amount)}`); announceChange() } catch (e) { toast((e as Error).message, 'err') }
  }
  const copy = async () => {
    if (!data.data) return
    const lines = data.data.groups.flatMap(g => g.items.map(it => `- ${it.name} (${qty(Math.max(it.min_quantity - it.quantity, 0))}${it.unit ? ' ' + it.unit : ''})${g.location ? ` — ${g.location}` : ''}`))
    try { await navigator.clipboard.writeText(lines.join('\n')); toast('Copied to clipboard') } catch { toast('Clipboard not available', 'err') }
  }

  if (!data.data) return <Loading />
  const d = data.data
  return (
    <>
      <PageHeader title="Shopping list" sub={`${d.low_count} item${d.low_count === 1 ? '' : 's'} at or below the alert level`} actions={d.low_count > 0 && <button type="button" className="btn" onClick={copy}><Icon name="copy" size={15} />Copy as text</button>} />
      {d.low_count === 0 ? (
        <Card><Empty icon="cart" title="Nothing to buy">Set “Alert below” on an item and it shows up here when you run out.</Empty></Card>
      ) : d.groups.map(g => (
        <div className="shop-group" key={g.location || '-'}>
          <h3><Icon name="pin" size={14} />{g.location || 'No place'}<span className="shop-count">{g.items.length}</span></h3>
          {g.items.map(it => {
            const need = Math.max(it.min_quantity - it.quantity, 0)
            return (
              <div className="shop-row" key={it.id}>
                <div className="nm" role="button" tabIndex={0} onClick={() => navigate(`/items/${it.id}`)} onKeyDown={e => { if (e.key === 'Enter') navigate(`/items/${it.id}`) }}><span className="row nm-line">{it.name} {it.users.length > 0 && <Avatars users={it.users} />}</span><div className="muted small">{qty(it.quantity)}{it.unit ? ` ${it.unit}` : ''} left · alert at {qty(it.min_quantity)}{it.brand ? ` · ${it.brand}` : ''}</div></div>
                <span className="badge danger need">need {qty(need || 1)}{it.unit ? ` ${it.unit}` : ''}</span>
                <span className="shop-acts">
                  <button type="button" className="btn primary sm" onClick={() => bought(it, need || 1)}><Icon name="check" size={14} strokeWidth={2.2} />Bought {qty(need || 1)}</button>
                  <button type="button" className="btn ghost sm" onClick={() => setRestock(it)}>Other…</button>
                </span>
              </div>
            )
          })}
        </div>
      ))}

      {d.expiring.length > 0 && (
        <div className="shop-group">
          <h3><Icon name="hourglass" size={14} />Expiring within {settings?.expiry_warn_days ?? 30} days<span className="shop-count">{d.expiring.length}</span></h3>
          {d.expiring.map(it => {
            const days = daysUntil(it.expiry_date)
            return (
              <div className="shop-row" key={it.id}>
                <div className="nm" role="button" tabIndex={0} onClick={() => navigate(`/items/${it.id}`)} onKeyDown={e => { if (e.key === 'Enter') navigate(`/items/${it.id}`) }}><span className="row nm-line">{it.name}</span><div className="muted small">{it.location_name || 'no place'} · {qty(it.quantity)}{it.unit ? ` ${it.unit}` : ''}</div></div>
                <span className={`badge ${days <= 0 ? 'danger' : 'warn'}`}>{days <= 0 ? `expired ${-days} d ago` : `${days} d left`}</span>
                <span className="shop-acts"><button type="button" className="btn ghost sm" onClick={() => navigate(`/items/${it.id}`)}>Open<Icon name="chevron-right" size={14} /></button></span>
              </div>
            )
          })}
        </div>
      )}

      {restock && <RestockModal item={restock} onClose={() => setRestock(null)} onDone={(n, note) => { setRestock(null); bought(restock, n, note) }} />}
    </>
  )
}

function RestockModal({ item, onClose, onDone }: { item: Item; onClose: () => void; onDone: (n: number, note: string) => void }) {
  const [n, setN] = useState(Math.max(item.min_quantity - item.quantity, 1))
  const [note, setNote] = useState('bought')
  return (
    <Modal title={`Restock ${item.name}`} onClose={onClose}>
      <form onSubmit={e => { e.preventDefault(); onDone(n, note) }} className="stack">
        <label className="field"><span className="label">How many did you add?</span><input type="number" inputMode="decimal" step="any" min={0} value={n} onChange={e => setN(Number(e.target.value))} autoFocus /></label>
        <label className="field"><span className="label">Note</span><input value={note} onChange={e => setNote(e.target.value)} /></label>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary">Add</button></div>
      </form>
    </Modal>
  )
}
