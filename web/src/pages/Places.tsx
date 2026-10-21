import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { Category, Location } from '../lib/types'
import { Card, Confirm, Empty, PageHeader, useToast } from '../components/ui'
import { Icon } from '../components/icons'
import { announceChange } from '../components/events'

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
  const run = async (fn: () => Promise<unknown>, msg?: string) => { try { await fn(); await reload(); announceChange(); if (msg) toast(msg) } catch (e) { toast((e as Error).message, 'err') } }
  const add = (e: React.FormEvent) => { e.preventDefault(); if (!name.trim()) return; run(() => api.post(`${V1}/${kind}`, { name: name.trim() }), 'Added').then(() => setName('')) }
  const move = (id: number, dir: -1 | 1) => {
    const ids = rows.map(x => x.id); const i = ids.indexOf(id); const j = i + dir
    if (j < 0 || j >= ids.length) return
    ;[ids[i], ids[j]] = [ids[j], ids[i]]
    run(() => api.put(`${V1}/${kind}/reorder`, { ids }))
  }
  const param = kind === 'locations' ? 'location' : 'category'
  return (
    <Card title={<><Icon name={kind === 'locations' ? 'pin' : 'tag'} size={16} />{title}<span className="badge">{rows.length}</span></>} flush>
      <form onSubmit={add} className="row manage-add"><input value={name} onChange={e => setName(e.target.value)} placeholder={placeholder} maxLength={60} /><button className="btn primary" disabled={!name.trim()}><Icon name="plus" size={15} />Add</button></form>
      {rows.length === 0 && <div className="pad"><Empty icon={kind === 'locations' ? 'pin' : 'tag'} title={`No ${title.toLowerCase()} yet`} /></div>}
      <ul className="manage-list">
        {rows.map((r, i) => (
          <li key={r.id}>
            <div className="order"><button type="button" className="btn ghost icon-only xs" disabled={i === 0} onClick={() => move(r.id, -1)} aria-label="Move up" title="Move up"><Icon name="chevron-up" size={14} /></button><button type="button" className="btn ghost icon-only xs" disabled={i === rows.length - 1} onClick={() => move(r.id, 1)} aria-label="Move down" title="Move down"><Icon name="chevron-down" size={14} /></button></div>
            {editing?.id === r.id ? (
              <form className="inline-edit" onSubmit={e => { e.preventDefault(); run(() => api.put(`${V1}/${kind}/${r.id}`, { name: editing.name }), 'Renamed').then(() => setEditing(null)) }}>
                <input value={editing.name} onChange={e => setEditing({ id: r.id, name: e.target.value })} autoFocus onKeyDown={e => e.key === 'Escape' && setEditing(null)} />
                <button className="btn primary sm">Save</button><button type="button" className="btn ghost sm" onClick={() => setEditing(null)}>Cancel</button>
              </form>
            ) : (
              <>
                <button type="button" className="item" onClick={() => navigate(`/items?${param}=${r.id}`)} title="Show items">{kind === 'categories' && (r as Category).color && <span className="dot" style={{ background: (r as Category).color }} />}<span className="nm">{r.name}</span><span className="muted small">{r.item_count} item{r.item_count === 1 ? '' : 's'}</span></button>
                <div className="row manage-acts">
                  <button type="button" className="btn ghost sm" onClick={() => setEditing({ id: r.id, name: r.name })}><Icon name="edit" size={14} />Rename</button>
                  <button type="button" className="btn ghost icon-only sm danger-text" onClick={() => setDel(r)} aria-label={`Delete ${r.name}`} title="Delete"><Icon name="trash" size={14} /></button>
                </div>
              </>
            )}
          </li>
        ))}
      </ul>
      {del && <Confirm confirmLabel="Delete" title={`Delete “${del.name}”?`} message={del.item_count > 0 ? `${del.item_count} item${del.item_count === 1 ? ' keeps' : 's keep'} existing, just without a ${kind === 'locations' ? 'place' : 'category'}.` : 'It is not used by any item.'} onCancel={() => setDel(null)} onConfirm={() => { const d = del; setDel(null); run(() => api.del(`${V1}/${kind}/${d.id}`), 'Deleted') }} />}
    </Card>
  )
}
