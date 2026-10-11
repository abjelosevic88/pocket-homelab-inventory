import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { Category, Location } from '../lib/types'
import { Card, Confirm, Empty, PageHeader, useToast } from '../components/ui'

type Kind = 'locations' | 'categories'

export default function Places() {
  const { locations, categories, reloadMeta } = useApp()
  return (
    <>
      <PageHeader title="Places & categories" sub="Places are where things live (Garage, Pantry, Office drawer). Categories are what they are (Tools, Food, Medicine). Both are flat on purpose." />
      <div className="two-col">
        <PlaceList kind="locations" title="Places" rows={locations} reload={reloadMeta} placeholder="Add a place: Garage, Pantry, Car boot…" />
        <PlaceList kind="categories" title="Categories" rows={categories} reload={reloadMeta} placeholder="Add a category: Tools, Cables, Spices…" />
      </div>
    </>
  )
}

function PlaceList({ kind, title, rows, reload, placeholder }: { kind: Kind; title: string; rows: (Location | Category)[]; reload: () => Promise<void>; placeholder: string }) {
  const { toast } = useToast()
  const navigate = useNavigate()
  const [name, setName] = useState('')
  const [editing, setEditing] = useState<{ id: number; name: string } | null>(null)
  const [del, setDel] = useState<Location | Category | null>(null)
  const run = async (fn: () => Promise<unknown>, msg?: string) => { try { await fn(); await reload(); if (msg) toast(msg) } catch (e) { toast((e as Error).message, 'err') } }
  const add = (e: React.FormEvent) => { e.preventDefault(); if (!name.trim()) return; run(() => api.post(`${V1}/${kind}`, { name: name.trim() }), 'Added').then(() => setName('')) }
  const move = (id: number, dir: -1 | 1) => {
    const ids = rows.map(x => x.id); const i = ids.indexOf(id); const j = i + dir
    if (j < 0 || j >= ids.length) return
    ;[ids[i], ids[j]] = [ids[j], ids[i]]
    run(() => api.put(`${V1}/${kind}/reorder`, { ids }))
  }
  const param = kind === 'locations' ? 'location' : 'category'
  return (
    <Card title={`${title} (${rows.length})`} flush>
      <form onSubmit={add} className="row" style={{ padding: 12, borderBottom: '1px solid var(--border)' }}><input value={name} onChange={e => setName(e.target.value)} placeholder={placeholder} maxLength={60} /><button className="btn primary" disabled={!name.trim()}>Add</button></form>
      {rows.length === 0 && <div className="pad"><Empty title={`No ${title.toLowerCase()} yet`} /></div>}
      <ul className="manage-list">
        {rows.map((r, i) => (
          <li key={r.id}>
            <div className="order"><button className="btn ghost sm" disabled={i === 0} onClick={() => move(r.id, -1)} aria-label="Move up">↑</button><button className="btn ghost sm" disabled={i === rows.length - 1} onClick={() => move(r.id, 1)} aria-label="Move down">↓</button></div>
            {editing?.id === r.id ? (
              <form className="inline-edit" onSubmit={e => { e.preventDefault(); run(() => api.put(`${V1}/${kind}/${r.id}`, { name: editing.name }), 'Renamed').then(() => setEditing(null)) }}>
                <input value={editing.name} onChange={e => setEditing({ id: r.id, name: e.target.value })} autoFocus onKeyDown={e => e.key === 'Escape' && setEditing(null)} />
                <button className="btn primary sm">Save</button><button type="button" className="btn ghost sm" onClick={() => setEditing(null)}>Cancel</button>
              </form>
            ) : (
              <>
                <button className="item" onClick={() => navigate(`/items?${param}=${r.id}`)} title="Show items"><span className="nm">{r.name}</span><span className="muted small">{r.item_count} item{r.item_count === 1 ? '' : 's'}</span></button>
                <div className="row">
                  <button className="btn ghost sm" onClick={() => setEditing({ id: r.id, name: r.name })}>Rename</button>
                  <button className="btn ghost sm danger-text" onClick={() => setDel(r)}>Delete</button>
                </div>
              </>
            )}
          </li>
        ))}
      </ul>
      {del && <Confirm title={`Delete “${del.name}”?`} message={del.item_count > 0 ? `${del.item_count} item${del.item_count === 1 ? ' keeps' : 's keep'} existing, just without a ${kind === 'locations' ? 'place' : 'category'}.` : 'It is not used by any item.'} onCancel={() => setDel(null)} onConfirm={() => { const d = del; setDel(null); run(() => api.del(`${V1}/${kind}/${d.id}`), 'Deleted') }} />}
    </Card>
  )
}
