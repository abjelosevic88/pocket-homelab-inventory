import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { Dashboard, Item, Movement } from '../lib/types'
import { ago, daysUntil, money, qty, splitTags, todayISO } from '../lib/format'
import { Confirm, Empty, Field, Loading, Modal, PageHeader, useAsync, useDebounce, useToast } from '../components/ui'
import { Avatars, PeoplePicker } from '../components/people'

export default function Items() {
  const { settings, users, locations, categories, user, reloadMeta } = useApp()
  const { toast } = useToast()
  const navigate = useNavigate()
  const { id: routeID } = useParams()
  const [params, setParams] = useSearchParams()
  const q = params.get('q') || ''
  const location = params.get('location') || ''
  const category = params.get('category') || ''
  const person = params.get('user') || ''
  const tag = params.get('tag') || ''
  const view = params.get('view') || '' // '' | low | expiring | archived
  const sort = params.get('sort') || 'name'
  const dq = useDebounce(q, 250)
  const set = (k: string, v: string) => { const p = new URLSearchParams(params); if (v) p.set(k, v); else p.delete(k); setParams(p, { replace: true }) }

  const url = useMemo(() => {
    const p = new URLSearchParams()
    if (dq) p.set('q', dq)
    if (location) p.set('location', location)
    if (category) p.set('category', category)
    if (person) p.set('user', person)
    if (tag) p.set('tag', tag)
    if (view === 'low') p.set('low', '1')
    if (view === 'archived') p.set('archived', '1')
    if (view === 'expiring') { p.set('expiring', String(settings?.expiry_warn_days ?? 30)); p.set('sort', 'expiry') } else p.set('sort', sort)
    return `${V1}/items?${p}`
  }, [dq, location, category, person, tag, view, sort, settings?.expiry_warn_days])

  const items = useAsync(() => api.get<Item[]>(url), [url])
  const dash = useAsync(() => api.get<Dashboard>(`${V1}/dashboard`), [items.data])
  const tags = useAsync(() => api.get<string[]>(`${V1}/tags`), [items.data])
  const [edit, setEdit] = useState<Partial<Item> | null>(null)
  const [quick, setQuick] = useState({ name: '', quantity: '1', location_id: '', category_id: '' })
  const quickRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (routeID && routeID !== 'new') { api.get<{ item: Item }>(`${V1}/items/${routeID}`).then(r => setEdit(r.item)).catch(() => navigate('/items', { replace: true })) }
    else if (routeID === 'new') setEdit({})
  }, [routeID]) // eslint-disable-line react-hooks/exhaustive-deps

  const closeEdit = () => { setEdit(null); if (routeID) navigate('/items' + (params.toString() ? `?${params}` : ''), { replace: true }) }
  const refreshAll = async () => { await Promise.all([items.reload(), reloadMeta()]) }

  const adjust = async (it: Item, delta: number) => {
    try {
      const upd = await api.post<Item>(`${V1}/items/${it.id}/adjust`, { delta, note: delta > 0 ? 'added' : 'used' })
      items.setData(d => d ? d.map(x => x.id === upd.id ? upd : x) : d)
      if (upd.low && !it.low) toast(`${upd.name} is running low`)
    } catch (e) { toast((e as Error).message, 'err') }
  }

  const quickAdd = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!quick.name.trim()) return
    try {
      const body: Record<string, unknown> = { name: quick.name.trim(), quantity: Number(quick.quantity) || 0 }
      if (quick.location_id) body.location_id = Number(quick.location_id)
      if (quick.category_id) body.category_id = Number(quick.category_id)
      if (location && !quick.location_id && location !== 'none') body.location_id = Number(location)
      if (category && !quick.category_id && category !== 'none') body.category_id = Number(category)
      if (person && person !== 'none' && person !== 'me') body.user_ids = [Number(person)]
      if (person === 'me' && user) body.user_ids = [user.id]
      const it = await api.post<Item>(`${V1}/items`, body)
      toast(`Added ${it.name}`)
      setQuick(qk => ({ ...qk, name: '', quantity: '1' }))
      quickRef.current?.focus()
      await refreshAll()
    } catch (e) { toast((e as Error).message, 'err') }
  }

  const currency = settings?.currency || 'EUR'
  const d = dash.data
  const activeFilters = Boolean(q || location || category || person || tag || view)

  return (
    <>
      <PageHeader title="Items" sub={d ? `${d.items} items · ${qty(d.quantity_total)} things · ${money(d.total_value, currency)}` : undefined}
        actions={<button className="btn primary" onClick={() => setEdit({})}>+ New item</button>} />

      {d && (
        <div className="stats">
          <button className={`stat ${!view ? 'active' : ''}`} onClick={() => set('view', '')}><div className="n">{d.items}</div><div className="l">Items</div></button>
          <button className={`stat danger ${view === 'low' ? 'active' : ''}`} onClick={() => set('view', view === 'low' ? '' : 'low')}><div className="n">{d.low_stock}</div><div className="l">Running low</div></button>
          <button className={`stat warn ${view === 'expiring' ? 'active' : ''}`} onClick={() => set('view', view === 'expiring' ? '' : 'expiring')}><div className="n">{d.expiring + d.expired}</div><div className="l">Expiring soon</div></button>
          <button className="stat" onClick={() => navigate('/places')}><div className="n">{d.locations}</div><div className="l">Places</div></button>
          <button className={`stat ${view === 'archived' ? 'active' : ''}`} onClick={() => set('view', view === 'archived' ? '' : 'archived')}><div className="n">{d.archived}</div><div className="l">Archived</div></button>
        </div>
      )}

      <div className="toolbar">
        <label className="search"><span className="ico">⌕</span><input value={q} onChange={e => set('q', e.target.value)} placeholder="Search name, brand, serial, barcode, tag, place…" /></label>
        <select value={location} onChange={e => set('location', e.target.value)} aria-label="Place">
          <option value="">All places</option>
          {locations.map(l => <option key={l.id} value={l.id}>{l.name} ({l.item_count})</option>)}
          <option value="none">No place</option>
        </select>
        <select value={category} onChange={e => set('category', e.target.value)} aria-label="Category">
          <option value="">All categories</option>
          {categories.map(c => <option key={c.id} value={c.id}>{c.name} ({c.item_count})</option>)}
          <option value="none">Uncategorised</option>
        </select>
        <select value={sort} onChange={e => set('sort', e.target.value)} aria-label="Sort">
          <option value="name">Name</option>
          <option value="updated">Recently changed</option>
          <option value="created">Newest</option>
          <option value="quantity">Lowest quantity</option>
          <option value="location">By place</option>
          <option value="expiry">Expiry</option>
        </select>
        {activeFilters && <button className="btn ghost sm" onClick={() => setParams(new URLSearchParams(), { replace: true })}>Clear</button>}
      </div>

      {(users.length > 1 || (tags.data && tags.data.length > 0)) && (
        <div className="filters">
          {users.length > 1 && <>
            <span className="lbl">Whose:</span>
            <button className={`chip ${!person ? 'active' : ''}`} onClick={() => set('user', '')}>Anyone</button>
            <button className={`chip ${person === 'me' ? 'active' : ''}`} onClick={() => set('user', person === 'me' ? '' : 'me')}>Mine</button>
            {users.filter(u => u.id !== user?.id).map(u => <button key={u.id} className={`chip ${person === String(u.id) ? 'active' : ''}`} onClick={() => set('user', person === String(u.id) ? '' : String(u.id))}>{u.name || u.email}</button>)}
            <button className={`chip ${person === 'none' ? 'active' : ''}`} onClick={() => set('user', person === 'none' ? '' : 'none')}>Household</button>
            {tags.data && tags.data.length > 0 && <span className="lbl" style={{ marginLeft: 8 }}>Tags:</span>}
          </>}
          {tags.data?.map(t => <button key={t} className={`chip ${tag.toLowerCase() === t.toLowerCase() ? 'active' : ''}`} onClick={() => set('tag', tag.toLowerCase() === t.toLowerCase() ? '' : t)}>#{t}</button>)}
        </div>
      )}

      {view !== 'archived' && (
        <form className="card mb" onSubmit={quickAdd}>
          <div className="card-body quick-add">
            <input ref={quickRef} className="wide" value={quick.name} onChange={e => setQuick({ ...quick, name: e.target.value })} placeholder="Quick add: what did you get?" maxLength={120} />
            <input type="number" inputMode="decimal" step="any" min={0} value={quick.quantity} onChange={e => setQuick({ ...quick, quantity: e.target.value })} aria-label="Quantity" />
            <select value={quick.location_id} onChange={e => setQuick({ ...quick, location_id: e.target.value })} aria-label="Place"><option value="">{location && location !== 'none' ? locations.find(l => String(l.id) === location)?.name : 'Place…'}</option>{locations.map(l => <option key={l.id} value={l.id}>{l.name}</option>)}</select>
            <select value={quick.category_id} onChange={e => setQuick({ ...quick, category_id: e.target.value })} aria-label="Category"><option value="">{category && category !== 'none' ? categories.find(c => String(c.id) === category)?.name : 'Category…'}</option>{categories.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}</select>
            <button className="btn primary" disabled={!quick.name.trim()}>Add</button>
          </div>
        </form>
      )}

      {items.loading && !items.data ? <Loading /> : items.error ? <div className="callout danger">{items.error}</div> : items.data && items.data.length === 0 ? (
        <div className="card"><Empty title={activeFilters ? 'Nothing matches' : 'Nothing here yet'}>{activeFilters ? 'Try clearing a filter.' : 'Type something in the quick-add box above, or press “New item” for the full form.'}</Empty></div>
      ) : (
        <div className="items">
          {items.data?.map(it => <ItemRow key={it.id} it={it} currency={currency} onOpen={() => navigate(`/items/${it.id}${params.toString() ? `?${params}` : ''}`)} onAdjust={d => adjust(it, d)} />)}
        </div>
      )}

      {edit && <ItemModal item={edit} onClose={closeEdit} onSaved={async () => { closeEdit(); await refreshAll() }} />}
    </>
  )
}

function ItemRow({ it, currency, onOpen, onAdjust }: { it: Item; currency: string; onOpen: () => void; onAdjust: (delta: number) => void }) {
  const days = it.expiry_date ? daysUntil(it.expiry_date) : null
  const step = it.unit && ['kg', 'l', 'g', 'ml', 'm'].includes(it.unit.toLowerCase()) ? 0.5 : 1
  return (
    <div className={`item-row ${it.low ? 'low' : days !== null && days <= 0 ? 'expired' : ''} ${it.archived ? 'muted' : ''}`} onClick={onOpen} role="button">
      <div className="thumb">{it.photo ? <img src={`${V1}/items/${it.id}/photo?v=${encodeURIComponent(it.photo)}`} alt="" loading="lazy" /> : <span>▣</span>}</div>
      <div style={{ minWidth: 0 }}>
        <div className="nm">
          <span>{it.name}</span>
          {it.users.length > 0 && <Avatars users={it.users} />}
          {it.low && <span className="badge low">low</span>}
          {days !== null && days <= 0 && <span className="badge warn">expired</span>}
          {days !== null && days > 0 && days <= 30 && <span className="badge warn">{days} d left</span>}
          {it.archived && <span className="badge">archived</span>}
        </div>
        <div className="meta">
          {it.location_name && <span>⌂ {it.location_name}</span>}
          {it.category_name && <><span className="sep" /><span className="badge cat">{it.category_name}</span></>}
          {it.brand && <><span className="sep hide-sm" /><span className="hide-sm">{it.brand}{it.model ? ` ${it.model}` : ''}</span></>}
          {it.price > 0 && <><span className="sep hide-sm" /><span className="hide-sm">{money(it.price, currency)}</span></>}
          {splitTags(it.tags).slice(0, 4).map(t => <span key={t} className="badge tag hide-sm">{t}</span>)}
        </div>
      </div>
      <div className={`qty ${it.low ? 'low' : ''}`} onClick={e => e.stopPropagation()}>
        <button onClick={() => onAdjust(-step)} disabled={it.quantity <= 0 || it.archived} aria-label="Take one">−</button>
        <span className="val">{qty(it.quantity)}{it.unit && <span className="unit">{it.unit}</span>}</span>
        <button onClick={() => onAdjust(step)} disabled={it.archived} aria-label="Add one">+</button>
      </div>
    </div>
  )
}

export function ItemModal({ item, onClose, onSaved }: { item: Partial<Item>; onClose: () => void; onSaved: () => void }) {
  const { settings, users, locations, categories, reloadMeta } = useApp()
  const { toast } = useToast()
  const isNew = !item.id
  const [f, setF] = useState({
    name: item.name || '', notes: item.notes || '', quantity: item.quantity ?? 1, unit: item.unit ?? (settings?.default_unit || ''), min_quantity: item.min_quantity ?? 0,
    location_id: item.location_id ? String(item.location_id) : '', category_id: item.category_id ? String(item.category_id) : '', new_location: '', new_category: '',
    tags: item.tags || '', brand: item.brand || '', model: item.model || '', serial: item.serial || '', barcode: item.barcode || '', price: item.price ?? 0,
    purchase_date: item.purchase_date || '', expiry_date: item.expiry_date || '', url: item.url || '', user_ids: item.user_ids || [] as number[], note: '',
  })
  const [busy, setBusy] = useState(false)
  const [confirm, setConfirm] = useState<'delete' | null>(null)
  const [photo, setPhoto] = useState(item.photo || '')
  const [moves, setMoves] = useState<Movement[] | null>(null)
  const [moreOpen, setMoreOpen] = useState(Boolean(item.brand || item.model || item.serial || item.barcode || item.price || item.purchase_date || item.expiry_date || item.url))
  const currency = settings?.currency || 'EUR'

  useEffect(() => { if (item.id) api.get<Movement[]>(`${V1}/items/${item.id}/movements?limit=15`).then(setMoves).catch(() => setMoves([])) }, [item.id])

  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try {
      const body: Record<string, unknown> = {
        name: f.name, notes: f.notes, quantity: Number(f.quantity) || 0, unit: f.unit, min_quantity: Number(f.min_quantity) || 0, tags: f.tags, brand: f.brand, model: f.model,
        serial: f.serial, barcode: f.barcode, price: Number(f.price) || 0, purchase_date: f.purchase_date, expiry_date: f.expiry_date, url: f.url, user_ids: f.user_ids, note: f.note,
        location_id: f.location_id === 'new' ? null : f.location_id ? Number(f.location_id) : null, category_id: f.category_id === 'new' ? null : f.category_id ? Number(f.category_id) : null,
        location: f.location_id === 'new' ? f.new_location : '', category: f.category_id === 'new' ? f.new_category : '',
      }
      if (item.id) await api.put(`${V1}/items/${item.id}`, body); else await api.post(`${V1}/items`, body)
      toast(item.id ? 'Saved' : `Added ${f.name}`)
      await reloadMeta()
      onSaved()
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }

  const upload = async (file: File) => {
    if (!item.id) return
    const fd = new FormData(); fd.append('photo', file)
    setBusy(true)
    try {
      const res = await fetch(`${V1}/items/${item.id}/photo`, { method: 'POST', body: fd, credentials: 'same-origin' })
      if (!res.ok) throw new Error((await res.json().catch(() => ({ error: res.statusText }))).error)
      const it = await res.json() as Item
      setPhoto(it.photo); toast('Photo saved')
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const removePhoto = async () => { if (!item.id) return; await api.del(`${V1}/items/${item.id}/photo`); setPhoto(''); toast('Photo removed') }
  const archive = async (a: boolean) => { if (!item.id) return; try { await api.post(`${V1}/items/${item.id}/${a ? 'archive' : 'unarchive'}`); toast(a ? 'Archived' : 'Restored'); onSaved() } catch (e) { toast((e as Error).message, 'err') } }
  const del = async () => { if (!item.id) return; try { await api.del(`${V1}/items/${item.id}`); toast('Deleted'); onSaved() } catch (e) { toast((e as Error).message, 'err') } }

  return (
    <Modal title={isNew ? 'New item' : item.name || 'Edit item'} onClose={onClose} size="lg">
      <form onSubmit={save} className="grid" style={{ gap: 14 }}>
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} required autoFocus={isNew} placeholder="AA batteries, Cordless drill, Olive oil…" maxLength={120} /></Field>
        <div className="three-col tight">
          <Field label="Quantity"><input type="number" inputMode="decimal" step="any" min={0} value={f.quantity} onChange={e => setF({ ...f, quantity: Number(e.target.value) })} /></Field>
          <Field label="Unit"><input value={f.unit} onChange={e => setF({ ...f, unit: e.target.value })} placeholder="pcs, kg, l, box…" maxLength={16} list="units" /><datalist id="units"><option value="pcs" /><option value="kg" /><option value="g" /><option value="l" /><option value="ml" /><option value="box" /><option value="pack" /><option value="roll" /><option value="pair" /><option value="set" /></datalist></Field>
          <Field label="Alert below" help="0 = no low-stock alert"><input type="number" inputMode="decimal" step="any" min={0} value={f.min_quantity} onChange={e => setF({ ...f, min_quantity: Number(e.target.value) })} /></Field>
        </div>
        <div className="two-col tight">
          <Field label="Place">
            <select value={f.location_id} onChange={e => setF({ ...f, location_id: e.target.value })}>
              <option value="">— none —</option>
              {locations.map(l => <option key={l.id} value={l.id}>{l.name}</option>)}
              <option value="new">+ New place…</option>
            </select>
            {f.location_id === 'new' && <input className="mt" style={{ marginTop: 6 }} value={f.new_location} onChange={e => setF({ ...f, new_location: e.target.value })} placeholder="Name of the new place" autoFocus required />}
          </Field>
          <Field label="Category">
            <select value={f.category_id} onChange={e => setF({ ...f, category_id: e.target.value })}>
              <option value="">— none —</option>
              {categories.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}
              <option value="new">+ New category…</option>
            </select>
            {f.category_id === 'new' && <input style={{ marginTop: 6 }} value={f.new_category} onChange={e => setF({ ...f, new_category: e.target.value })} placeholder="Name of the new category" autoFocus required />}
          </Field>
        </div>
        {users.length > 1 && <Field label="Whose is it?" help="Pick one or more people; leave on “Everyone” for shared household things."><PeoplePicker users={users} value={f.user_ids} onChange={ids => setF({ ...f, user_ids: ids })} /></Field>}
        <Field label="Tags" help="Comma separated."><input value={f.tags} onChange={e => setF({ ...f, tags: e.target.value })} placeholder="spare, camping, important" /></Field>
        <Field label="Notes"><textarea value={f.notes} onChange={e => setF({ ...f, notes: e.target.value })} rows={2} placeholder="Where exactly, what it is for, anything worth remembering." /></Field>

        <details className="more" open={moreOpen} onToggle={e => setMoreOpen((e.target as HTMLDetailsElement).open)}>
          <summary>Details: brand, serial, price, dates, barcode</summary>
          <div className="grid mt" style={{ gap: 12 }}>
            <div className="three-col tight">
              <Field label="Brand"><input value={f.brand} onChange={e => setF({ ...f, brand: e.target.value })} /></Field>
              <Field label="Model"><input value={f.model} onChange={e => setF({ ...f, model: e.target.value })} /></Field>
              <Field label="Serial number"><input value={f.serial} onChange={e => setF({ ...f, serial: e.target.value })} className="mono" /></Field>
            </div>
            <div className="three-col tight">
              <Field label={`Price per unit (${currency})`}><input type="number" inputMode="decimal" step="any" min={0} value={f.price} onChange={e => setF({ ...f, price: Number(e.target.value) })} /></Field>
              <Field label="Bought on"><input type="date" value={f.purchase_date} onChange={e => setF({ ...f, purchase_date: e.target.value })} max={todayISO()} /></Field>
              <Field label="Expires / warranty ends"><input type="date" value={f.expiry_date} onChange={e => setF({ ...f, expiry_date: e.target.value })} /></Field>
            </div>
            <div className="two-col tight">
              <Field label="Barcode / SKU" help="Scan it with your phone keyboard; GET /api/v1/lookup?barcode= finds it."><input value={f.barcode} onChange={e => setF({ ...f, barcode: e.target.value })} className="mono" inputMode="numeric" /></Field>
              <Field label="Link" help="Manual, shop page, receipt…"><input type="url" value={f.url} onChange={e => setF({ ...f, url: e.target.value })} placeholder="https://" /></Field>
            </div>
          </div>
        </details>

        {item.id && (
          <Field label="Photo">
            <div className="photo-box">
              <div className="thumb">{photo ? <img src={`${V1}/items/${item.id}/photo?v=${encodeURIComponent(photo)}`} alt="" /> : <span>▣</span>}</div>
              <div className="grid" style={{ gap: 8 }}>
                <label className="btn">{photo ? 'Replace photo…' : 'Add photo…'}<input type="file" accept="image/*" capture="environment" hidden onChange={e => e.target.files?.[0] && upload(e.target.files[0])} /></label>
                {photo && <button type="button" className="btn ghost danger-text sm" onClick={removePhoto}>Remove</button>}
              </div>
            </div>
          </Field>
        )}
        {isNew && <p className="muted small">You can add a photo after saving.</p>}

        {item.id && item.quantity !== undefined && Number(f.quantity) !== item.quantity && (
          <Field label="Why did the quantity change?" help="Optional, shown in the history."><input value={f.note} onChange={e => setF({ ...f, note: e.target.value })} placeholder="stock-take, gave away, broke…" /></Field>
        )}

        <div className="form-actions">
          {item.id && <>
            <button type="button" className="btn ghost danger-text" onClick={() => setConfirm('delete')}>Delete</button>
            <button type="button" className="btn ghost" onClick={() => archive(!item.archived)}>{item.archived ? 'Restore' : 'Archive'}</button>
          </>}
          <span className="spacer" />
          <button type="button" className="btn" onClick={onClose}>Cancel</button>
          <button className="btn primary" disabled={busy}>{isNew ? 'Add item' : 'Save'}</button>
        </div>
      </form>

      {item.id && moves && moves.length > 0 && (
        <div className="mt">
          <h3 className="muted small" style={{ textTransform: 'uppercase', letterSpacing: '.04em', marginBottom: 6 }}>History</h3>
          <ul className="history">
            {moves.map(m => <li key={m.id}><span className={`d ${m.delta > 0 ? 'plus' : 'minus'}`}>{m.delta > 0 ? '+' : ''}{qty(m.delta)}</span><span>→ {qty(m.quantity_after)}{item.unit ? ` ${item.unit}` : ''}{m.note ? ` · ${m.note}` : ''}{m.user_name ? ` · ${m.user_name}` : ''}</span><span className="when">{ago(m.created_at)}</span></li>)}
          </ul>
        </div>
      )}
      {confirm === 'delete' && <Confirm title={`Delete “${item.name}”?`} message="This removes the item, its photo and its history. Archive instead if you may want it back." onCancel={() => setConfirm(null)} onConfirm={del} />}
    </Modal>
  )
}

