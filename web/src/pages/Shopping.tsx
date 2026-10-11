import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { Item, Shopping as ShoppingT } from '../lib/types'
import { daysUntil, qty } from '../lib/format'
import { Card, Empty, Loading, Modal, PageHeader, useAsync, useToast } from '../components/ui'
import { Avatars } from '../components/people'

export default function Shopping() {
  const { settings } = useApp()
  const { toast } = useToast()
  const navigate = useNavigate()
  const data = useAsync(() => api.get<ShoppingT>(`${V1}/shopping`), [])
  const [restock, setRestock] = useState<Item | null>(null)

  const bought = async (it: Item, amount: number, note = 'bought') => {
    try { await api.post(`${V1}/items/${it.id}/adjust`, { delta: amount, note }); toast(`${it.name}: +${qty(amount)}`); data.reload() } catch (e) { toast((e as Error).message, 'err') }
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
      <PageHeader title="Shopping list" sub={`${d.low_count} item${d.low_count === 1 ? '' : 's'} at or below the alert level`} actions={d.low_count > 0 && <button className="btn" onClick={copy}>Copy as text</button>} />
      {d.low_count === 0 ? (
        <Card><Empty title="Nothing to buy">Set “Alert below” on an item and it shows up here when you run out.</Empty></Card>
      ) : d.groups.map(g => (
        <div className="shop-group" key={g.location || '-'}>
          <h3>{g.location || 'No place'}</h3>
          {g.items.map(it => {
            const need = Math.max(it.min_quantity - it.quantity, 0)
            return (
              <div className="shop-row" key={it.id}>
                <div className="nm" style={{ cursor: 'pointer' }} onClick={() => navigate(`/items/${it.id}`)}>{it.name} {it.users.length > 0 && <Avatars users={it.users} />}<div className="muted small">{qty(it.quantity)}{it.unit ? ` ${it.unit}` : ''} left · alert at {qty(it.min_quantity)}{it.brand ? ` · ${it.brand}` : ''}</div></div>
                <span className="need">need {qty(need || 1)}{it.unit ? ` ${it.unit}` : ''}</span>
                <button className="btn primary sm" onClick={() => bought(it, need || 1)}>Bought {qty(need || 1)}</button>
                <button className="btn ghost sm" onClick={() => setRestock(it)}>Other…</button>
              </div>
            )
          })}
        </div>
      ))}

      {d.expiring.length > 0 && (
        <div className="shop-group">
          <h3>Expiring within {settings?.expiry_warn_days ?? 30} days</h3>
          {d.expiring.map(it => {
            const days = daysUntil(it.expiry_date)
            return (
              <div className="shop-row" key={it.id}>
                <div className="nm" style={{ cursor: 'pointer' }} onClick={() => navigate(`/items/${it.id}`)}>{it.name}<div className="muted small">{it.location_name || 'no place'} · {qty(it.quantity)}{it.unit ? ` ${it.unit}` : ''}</div></div>
                <span className={`badge ${days <= 0 ? 'low' : 'warn'}`}>{days <= 0 ? `expired ${-days} d ago` : `${days} d left`}</span>
                <button className="btn ghost sm" onClick={() => navigate(`/items/${it.id}`)}>Open</button>
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
      <form onSubmit={e => { e.preventDefault(); onDone(n, note) }} className="grid" style={{ gap: 12 }}>
        <label className="field"><span className="label">How many did you add?</span><input type="number" inputMode="decimal" step="any" min={0} value={n} onChange={e => setN(Number(e.target.value))} autoFocus /></label>
        <label className="field"><span className="label">Note</span><input value={note} onChange={e => setNote(e.target.value)} /></label>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary">Add</button></div>
      </form>
    </Modal>
  )
}
