import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { Dashboard, Item, Movement } from '../lib/types'
import { ago, daysUntil, money, qty, splitTags, todayISO } from '../lib/format'
import { Confirm, Empty, Field, Loading, Modal, PageHeader, useAsync, useDebounce, useToast } from '../components/ui'
import { Icon } from '../components/icons'
import { announceChange, useOnChange } from '../components/events'
import { Avatars, PeoplePicker } from '../components/people'
import { Select } from '../components/Select'
import type { Category, Location } from '../lib/types'

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
  // Anything added or changed (here or from the shell's New menu) reloads the list;
  // the figures and tags follow the list.
  useOnChange(() => { items.reload() })
  const refreshAll = async () => { await reloadMeta(); announceChange() }

  const adjust = async (it: Item, delta: number) => {
    try {
      const upd = await api.post<Item>(`${V1}/items/${it.id}/adjust`, { delta, note: delta > 0 ? 'added' : 'used' })
      items.setData(d => d ? d.map(x => x.id === upd.id ? upd : x) : d)
      if (upd.low && !it.low) toast(`${upd.name} is running low`)
      if (upd.low !== it.low) announceChange()
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
  const placeOpts = [{ value: '', label: 'All places' }, ...locations.map(l => ({ value: String(l.id), label: l.name, hint: `${l.item_count}` })), { value: 'none', label: 'No place' }]
  const catOpts = [{ value: '', label: 'All categories' }, ...categories.map(c => ({ value: String(c.id), label: c.name, hint: `${c.item_count}` })), { value: 'none', label: 'Uncategorised' }]
  const createPlace = async (name: string) => { const l = await api.post<Location>(`${V1}/locations`, { name }); await reloadMeta(); return String(l.id) }
  const createCategory = async (name: string) => { const c = await api.post<Category>(`${V1}/categories`, { name }); await reloadMeta(); return String(c.id) }
  const activeFilters = Boolean(q || location || category || person || tag || view)

  return (
    <>
      <PageHeader title="Items" sub={d ? `${d.items} items · ${qty(d.quantity_total)} things · ${money(d.total_value, currency)}` : undefined}
        actions={<button type="button" className="btn primary hide-mobile" onClick={() => setEdit({})}><Icon name="plus" size={16} />New item</button>} />

      {d && (
        <div className="stats">
          <button type="button" className={`stat ${!view ? 'active' : ''}`} onClick={() => set('view', '')}><Icon name="package" size={16} /><div className="n">{d.items}</div><div className="l">Items</div></button>
          <button type="button" className={`stat danger ${d.low_stock ? 'has' : ''} ${view === 'low' ? 'active' : ''}`} onClick={() => set('view', view === 'low' ? '' : 'low')}><Icon name="alert" size={16} /><div className="n">{d.low_stock}</div><div className="l">Running low</div></button>
          <button type="button" className={`stat warn ${d.expiring + d.expired ? 'has' : ''} ${view === 'expiring' ? 'active' : ''}`} onClick={() => set('view', view === 'expiring' ? '' : 'expiring')}><Icon name="hourglass" size={16} /><div className="n">{d.expiring + d.expired}</div><div className="l">Expiring soon</div></button>
          <button type="button" className="stat" onClick={() => navigate('/places')}><Icon name="pin" size={16} /><div className="n">{d.locations}</div><div className="l">Places</div></button>
          <button type="button" className={`stat ${view === 'archived' ? 'active' : ''}`} onClick={() => set('view', view === 'archived' ? '' : 'archived')}><Icon name="archive" size={16} /><div className="n">{d.archived}</div><div className="l">Archived</div></button>
        </div>
      )}

      <div className="toolbar">
        <label className="search"><Icon name="search" size={15} /><input value={q} onChange={e => set('q', e.target.value)} placeholder="Search name, brand, serial, barcode, tag, place…" /></label>
        <Select value={location} onChange={v => set('location', v)} options={placeOpts} ariaLabel="Place" placeholder="All places" />
        <Select value={category} onChange={v => set('category', v)} options={catOpts} ariaLabel="Category" placeholder="All categories" />
        <Select value={sort} onChange={v => set('sort', v)} ariaLabel="Sort" options={[{ value: 'name', label: 'Sort: name' }, { value: 'updated', label: 'Sort: recently changed' }, { value: 'created', label: 'Sort: newest' }, { value: 'quantity', label: 'Sort: lowest quantity' }, { value: 'location', label: 'Sort: by place' }, { value: 'expiry', label: 'Sort: expiry' }]} />
        {activeFilters && <button type="button" className="btn ghost" onClick={() => setParams(new URLSearchParams(), { replace: true })}><Icon name="x" size={15} />Clear</button>}
      </div>

      {(users.length > 1 || (tags.data && tags.data.length > 0)) && (
        <div className="filters">
          {users.length > 1 && <>
            <span className="lbl"><Icon name="users" size={14} />Whose</span>
            <button type="button" className={`chip ${!person ? 'active' : ''}`} onClick={() => set('user', '')}>All</button>
            <button type="button" className={`chip ${person === 'me' ? 'active' : ''}`} onClick={() => set('user', person === 'me' ? '' : 'me')}>Mine</button>
            {users.filter(u => u.id !== user?.id).map(u => <button type="button" key={u.id} className={`chip ${person === String(u.id) ? 'active' : ''}`} onClick={() => set('user', person === String(u.id) ? '' : String(u.id))}>{u.name || u.email}</button>)}
            <button type="button" className={`chip ${person === 'none' ? 'active' : ''}`} onClick={() => set('user', person === 'none' ? '' : 'none')} title="Items you have not shared with anyone">Not shared</button>
            {tags.data && tags.data.length > 0 && <span className="lbl filters-gap"><Icon name="tag" size={14} />Tags</span>}
          </>}
          {tags.data?.map(t => <button type="button" key={t} className={`chip ${tag.toLowerCase() === t.toLowerCase() ? 'active' : ''}`} onClick={() => set('tag', tag.toLowerCase() === t.toLowerCase() ? '' : t)}>#{t}</button>)}
        </div>
      )}

      {view !== 'archived' && (
        <form className="card mb quick-card" onSubmit={quickAdd}>
          <div className="card-body quick-add">
            <span className="quick-ico" aria-hidden="true"><Icon name="zap" size={16} /></span>
            <input ref={quickRef} className="wide" value={quick.name} onChange={e => setQuick({ ...quick, name: e.target.value })} placeholder="Quick add: what did you get?" maxLength={120} />
            <input type="number" inputMode="decimal" step="any" min={0} value={quick.quantity} onChange={e => setQuick({ ...quick, quantity: e.target.value })} aria-label="Quantity" />
            <Select value={quick.location_id} onChange={v => setQuick({ ...quick, location_id: v })} ariaLabel="Place" placeholder={location && location !== 'none' ? (locations.find(l => String(l.id) === location)?.name || 'Place…') : 'Place…'} options={locations.map(l => ({ value: String(l.id), label: l.name }))} onCreate={createPlace} createLabel="New place" clearLabel="No place" />
            <Select value={quick.category_id} onChange={v => setQuick({ ...quick, category_id: v })} ariaLabel="Category" placeholder={category && category !== 'none' ? (categories.find(c => String(c.id) === category)?.name || 'Category…') : 'Category…'} options={categories.map(c => ({ value: String(c.id), label: c.name }))} onCreate={createCategory} createLabel="New category" clearLabel="No category" />
            <button className="btn primary" disabled={!quick.name.trim()}><Icon name="plus" size={15} />Add</button>
          </div>
        </form>
      )}

      {items.loading && !items.data ? <Loading /> : items.error ? <div className="callout danger"><Icon name="alert" size={15} /><span>{items.error}</span></div> : items.data && items.data.length === 0 ? (
        <div className="card"><Empty icon={activeFilters ? 'search' : 'package'} title={activeFilters ? 'Nothing matches' : 'Nothing here yet'}>{activeFilters ? 'Try clearing a filter.' : 'Type something in the quick-add box above, or press “New item” for the full form.'}</Empty></div>
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
      <div className="thumb">{it.photo ? <img src={`${V1}/items/${it.id}/photo?v=${encodeURIComponent(it.photo)}`} alt="" loading="lazy" /> : <Icon name="package" size={20} />}</div>
      <div className="item-main">
        <div className="nm">
          <span className="item-name">{it.name}</span>
          {it.users.length > 0 && <Avatars users={it.users} />}
          {it.low && <span className="badge danger">low</span>}
          {days !== null && days <= 0 && <span className="badge warn">expired</span>}
          {days !== null && days > 0 && days <= 30 && <span className="badge warn">{days} d left</span>}
          {it.archived && <span className="badge">archived</span>}
        </div>
        <div className="meta">
          {it.location_name && <span className="meta-place"><Icon name="pin" size={13} />{it.location_name}</span>}
          {it.category_name && <><span className="sep" /><span className="badge cat">{it.category_name}</span></>}
          {it.brand && <><span className="sep hide-sm" /><span className="hide-sm">{it.brand}{it.model ? ` ${it.model}` : ''}</span></>}
          {it.price > 0 && <><span className="sep hide-sm" /><span className="hide-sm">{money(it.price, currency)}</span></>}
          {splitTags(it.tags).slice(0, 4).map(t => <span key={t} className="badge tag hide-sm">{t}</span>)}
        </div>
      </div>
      <div className={`qty ${it.low ? 'low' : ''}`} onClick={e => e.stopPropagation()}>
        <button type="button" onClick={() => onAdjust(-step)} disabled={it.quantity <= 0 || it.archived} aria-label="Take one" title="Take one"><Icon name="minus" size={15} strokeWidth={2} /></button>
        <span className="val">{qty(it.quantity)}{it.unit && <span className="unit">{it.unit}</span>}</span>
        <button type="button" onClick={() => onAdjust(step)} disabled={it.archived} aria-label="Add one" title="Add one"><Icon name="plus" size={15} strokeWidth={2} /></button>
      </div>
    </div>
  )
}

export function ItemModal({ item, onClose, onSaved }: { item: Partial<Item>; onClose: () => void; onSaved: () => void }) {
  const { settings, users, locations, categories, reloadMeta, user } = useApp()
  const { toast } = useToast()
  const isNew = !item.id
  // Someone else's item shared with you: you can use and edit it, but its place,
  // category, who it is shared with and deleting it stay with its owner.
  const isOwner = isNew || !item.owner_id || item.owner_id === user?.id
  const [f, setF] = useState({
    name: item.name || '', notes: item.notes || '', quantity: item.quantity ?? 1, unit: item.unit ?? (settings?.default_unit || ''), min_quantity: item.min_quantity ?? 0,
    location_id: item.location_id ? String(item.location_id) : '', category_id: item.category_id ? String(item.category_id) : '',
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
        location_id: f.location_id ? Number(f.location_id) : null, category_id: f.category_id ? Number(f.category_id) : null,
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
        {!isOwner && <p className="muted small">Shared with you by its owner. Place, category and sharing are theirs to change.</p>}
        {isOwner ? <div className="two-col tight">
          <Field label="Place" help="Type a new name to create a place on the spot.">
            <Select value={f.location_id} onChange={v => setF({ ...f, location_id: v })} placeholder="— none —" options={locations.map(l => ({ value: String(l.id), label: l.name }))} onCreate={async name => { const l = await api.post<Location>(`${V1}/locations`, { name }); await reloadMeta(); return String(l.id) }} createLabel="New place" clearLabel="No place" />
          </Field>
          <Field label="Category">
            <Select value={f.category_id} onChange={v => setF({ ...f, category_id: v })} placeholder="— none —" options={categories.map(c => ({ value: String(c.id), label: c.name }))} onCreate={async name => { const c = await api.post<Category>(`${V1}/categories`, { name }); await reloadMeta(); return String(c.id) }} createLabel="New category" clearLabel="No category" />
          </Field>
        </div> : <div className="two-col tight">
          <Field label="Place"><input value={item.location_name || '—'} disabled /></Field>
          <Field label="Category"><input value={item.category_name || '—'} disabled /></Field>
        </div>}
        {isOwner && users.length > 1 && <Field label="Whose is it? / shared with" help="Your items are yours alone. Pick people to share this one with: they see it and can use it."><PeoplePicker users={users} value={f.user_ids} onChange={ids => setF({ ...f, user_ids: ids })} /></Field>}
        <Field label="Tags" help="Comma separated."><input value={f.tags} onChange={e => setF({ ...f, tags: e.target.value })} placeholder="spare, camping, important" /></Field>
        <Field label="Notes"><textarea value={f.notes} onChange={e => setF({ ...f, notes: e.target.value })} rows={2} placeholder="Where exactly, what it is for, anything worth remembering." /></Field>

        <details className="more" open={moreOpen} onToggle={e => setMoreOpen((e.target as HTMLDetailsElement).open)}>
          <summary><Icon name="chevron-right" size={15} className="more-caret" />Details: brand, serial, price, dates, barcode</summary>
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
              <div className="thumb">{photo ? <img src={`${V1}/items/${item.id}/photo?v=${encodeURIComponent(photo)}`} alt="" /> : <Icon name="image" size={28} />}</div>
              <div className="grid" style={{ gap: 8 }}>
                <label className="btn"><Icon name="camera" size={15} />{photo ? 'Replace photo…' : 'Add photo…'}<input type="file" accept="image/*" capture="environment" hidden onChange={e => e.target.files?.[0] && upload(e.target.files[0])} /></label>
                {photo && <button type="button" className="btn ghost danger-text sm" onClick={removePhoto}><Icon name="trash" size={14} />Remove</button>}
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
            {isOwner && <button type="button" className="btn ghost danger-text" onClick={() => setConfirm('delete')}><Icon name="trash" size={15} />Delete</button>}
            <button type="button" className="btn ghost" onClick={() => archive(!item.archived)}><Icon name={item.archived ? 'undo' : 'archive'} size={15} />{item.archived ? 'Restore' : 'Archive'}</button>
          </>}
          <span className="spacer" />
          <button type="button" className="btn" onClick={onClose}>Cancel</button>
          <button className="btn primary" disabled={busy}>{isNew ? 'Add item' : 'Save'}</button>
        </div>
      </form>

      {item.id && moves && moves.length > 0 && (
        <div className="mt">
          <h3 className="section-label"><Icon name="activity" size={14} />History</h3>
          <ul className="history">
            {moves.map(m => <li key={m.id}><span className={`d ${m.delta > 0 ? 'plus' : 'minus'}`}>{m.delta > 0 ? '+' : ''}{qty(m.delta)}</span><span className="grow">→ {qty(m.quantity_after)}{item.unit ? ` ${item.unit}` : ''}{m.note ? ` · ${m.note}` : ''}{m.user_name ? ` · ${m.user_name}` : ''}</span><span className="when">{ago(m.created_at)}</span></li>)}
          </ul>
        </div>
      )}
      {confirm === 'delete' && <Confirm confirmLabel="Delete" title={`Delete “${item.name}”?`} message="This removes the item, its photo and its history. Archive instead if you may want it back." onCancel={() => setConfirm(null)} onConfirm={del} />}
    </Modal>
  )
}

