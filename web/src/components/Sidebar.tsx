// Left column: new item, sections, views, places and categories, settings — the Pocket Calendar sidebar.
import { useState } from 'react'
import { Link, NavLink, useLocation, useNavigate } from 'react-router-dom'
import { useApp } from '../lib/app-context'
import type { Category, Dashboard, Item, Location } from '../lib/types'
import { Icon } from './icons'
import type { NameKind } from './Layout'
import { openShortcuts } from './events'
import { type Anchor, type MenuItem, Menu, rectOf } from './ui'

export function Sidebar({ dash, tags, onCreate, onNewName, onNavigate }: { dash: Dashboard | null; tags: string[]; onCreate: (preset?: Partial<Item>) => void; onNewName: (k: NameKind) => void; onNavigate: () => void }) {
  const { status, settings, locations, categories } = useApp()
  const [menu, setMenu] = useState<Anchor | null>(null)
  const loc = useLocation()
  const q = new URLSearchParams(loc.search)
  const onItems = loc.pathname === '/items' || loc.pathname === '/'
  // "Items" is active on the plain list; a view or a place lights up its own row instead
  const filtered = ['view', 'location', 'category', 'tag'].some(k => q.get(k))
  const title = settings?.title || status?.title || 'Pocket Inventory'
  const createItems: MenuItem[] = [
    { label: 'Item', icon: 'package', hint: 'N', onSelect: () => onCreate() },
    'sep',
    { label: 'Place', icon: 'pin', onSelect: () => onNewName('locations') },
    { label: 'Category', icon: 'tag', onSelect: () => onNewName('categories') },
  ]
  const hasPantry = tags.some(t => t.toLowerCase() === 'pantry')
  const expiring = dash ? dash.expiring + dash.expired : 0
  const views: { key: string; to: string; label: string; icon: string; on: boolean; badge?: number; tone?: string; count?: number }[] = [
    { key: 'low', to: '/items?view=low', label: 'Running low', icon: 'alert', on: q.get('view') === 'low', badge: dash?.low_stock, tone: 'danger' },
    { key: 'expiring', to: '/items?view=expiring', label: 'Expiring soon', icon: 'hourglass', on: q.get('view') === 'expiring', badge: expiring, tone: 'warn' },
    ...(hasPantry ? [{ key: 'pantry', to: '/items?tag=pantry', label: 'Pantry', icon: 'pantry', on: (q.get('tag') || '').toLowerCase() === 'pantry' }] : []),
    { key: 'archived', to: '/items?view=archived', label: 'Archived', icon: 'archive', on: q.get('view') === 'archived', count: dash?.archived },
  ]

  return (
    <aside className="sidebar" aria-label="Inventory">
      <div className="sb-brand">
        <img src="/favicon.svg" alt="" className="sb-logo" />
        <span className="sb-title">{title !== 'Pocket Inventory' ? title : <>Pocket <b>Inventory</b></>}</span>
        {status?.version && <span className="sb-version" title={`Version ${status.version}`}>{/^\d/.test(status.version) ? 'v' + status.version : status.version}</span>}
      </div>
      <button type="button" className="btn primary create-btn" aria-haspopup="menu" aria-expanded={!!menu} onClick={e => setMenu(menu ? null : rectOf(e.currentTarget))}>
        <Icon name="plus" size={17} strokeWidth={2.2} />New<Icon name="chevron-down" size={15} className="create-caret" />
      </button>
      {menu && <Menu anchor={menu} items={createItems} onClose={() => setMenu(null)} width={220} />}

      <nav className="sb-nav" aria-label="Sections">
        <NavLink to="/items" onClick={onNavigate} className={() => `sb-nav-item ${onItems && !filtered ? 'active' : ''}`}>
          <Icon name="package" size={16} />Items{dash && <span className="sb-count">{dash.items}</span>}
        </NavLink>
        <NavLink to="/shopping" onClick={onNavigate} className={({ isActive }) => `sb-nav-item ${isActive ? 'active' : ''}`}>
          <Icon name="cart" size={16} />Shopping list{!!dash?.low_stock && <span className="badge danger" title={`${dash.low_stock} at or below the alert level`}>{dash.low_stock}</span>}
        </NavLink>
        <NavLink to="/places" onClick={onNavigate} className={({ isActive }) => `sb-nav-item ${isActive ? 'active' : ''}`}>
          <Icon name="pin" size={16} />Places & categories
        </NavLink>
        <NavLink to="/activity" onClick={onNavigate} className={({ isActive }) => `sb-nav-item ${isActive ? 'active' : ''}`}>
          <Icon name="activity" size={16} />Activity
        </NavLink>
      </nav>

      <Group title="Views">
        <ul className="cal-list">
          {views.map(v => (
            <li key={v.key} className={`cal-row place-row ${v.on ? 'is-active' : ''}`}>
              <Link to={v.to} onClick={onNavigate} className="cal-check">
                <Icon name={v.icon} size={15} />
                <span className="cal-name ellipsis">{v.label}</span>
                {!!v.badge && <span className={`badge ${v.tone}`}>{v.badge}</span>}
                {!!v.count && <span className="place-meta">{v.count}</span>}
              </Link>
            </li>
          ))}
        </ul>
      </Group>

      <Group title="Places" add={() => onNewName('locations')} addLabel="Add a place">
        {locations.length === 0
          ? <p className="sb-empty">No places yet. <button type="button" className="link-btn" onClick={() => onNewName('locations')}>Add one</button></p>
          : <ul className="cal-list">{locations.map(l => <PlaceRow key={l.id} kind="location" row={l} active={q.get('location') === String(l.id)} onCreate={onCreate} onNavigate={onNavigate} />)}</ul>}
      </Group>

      {categories.length > 0 && (
        <Group title="Categories" add={() => onNewName('categories')} addLabel="Add a category" closed>
          <ul className="cal-list">{categories.map(c => <PlaceRow key={c.id} kind="category" row={c} active={q.get('category') === String(c.id)} onCreate={onCreate} onNavigate={onNavigate} />)}</ul>
        </Group>
      )}

      <div className="sb-foot">
        <Link to="/settings" className="sb-foot-link" onClick={onNavigate}><Icon name="settings" size={15} />Settings</Link>
        <button type="button" className="sb-foot-link" onClick={() => { onNavigate(); openShortcuts() }}><Icon name="keyboard" size={15} />Shortcuts</button>
      </div>
    </aside>
  )
}

function Group({ title, add, addLabel, closed, children }: { title: string; add?: () => void; addLabel?: string; closed?: boolean; children: React.ReactNode }) {
  const key = `pi-sb-${title.toLowerCase()}`
  const [open, setOpen] = useState(() => { const v = localStorage.getItem(key); return v === null ? !closed : v === '1' })
  const toggle = () => setOpen(o => { localStorage.setItem(key, o ? '0' : '1'); return !o })
  return (
    <section className="sb-sec">
      <h4 className="sb-h">
        <button type="button" className="sb-h-btn" aria-expanded={open} onClick={toggle}><Icon name={open ? 'chevron-down' : 'chevron-right'} size={13} />{title}</button>
        {add && <button type="button" className="btn ghost icon-only xs" aria-label={addLabel} title={addLabel} onClick={add}><Icon name="plus" size={14} /></button>}
      </h4>
      {open && children}
    </section>
  )
}

function PlaceRow({ kind, row, active, onCreate, onNavigate }: { kind: 'location' | 'category'; row: Location | Category; active: boolean; onCreate: (preset?: Partial<Item>) => void; onNavigate: () => void }) {
  const navigate = useNavigate()
  const [menu, setMenu] = useState<Anchor | null>(null)
  const to = `/items?${kind}=${row.id}`
  const color = kind === 'category' ? (row as Category).color : ''
  const items: MenuItem[] = [
    { label: 'Show items', icon: 'package', onSelect: () => { onNavigate(); navigate(to) } },
    { label: kind === 'location' ? 'New item here' : 'New item in this category', icon: 'plus', onSelect: () => onCreate(kind === 'location' ? { location_id: row.id } : { category_id: row.id }) },
    'sep',
    { label: 'Rename, reorder or delete', icon: 'edit', onSelect: () => { onNavigate(); navigate('/places') } },
  ]
  return (
    <li className={`cal-row place-row ${active ? 'is-active' : ''}`}>
      <Link to={to} onClick={onNavigate} className="cal-check" title={`${row.name} · ${row.item_count} item${row.item_count === 1 ? '' : 's'}`}>
        {kind === 'location' ? <Icon name="pin" size={15} /> : color ? <span className="dot" style={{ background: color }} /> : <Icon name="tag" size={15} />}
        <span className="cal-name ellipsis">{row.name}</span>
        {row.item_count > 0 && <span className="place-meta">{row.item_count}</span>}
      </Link>
      <button type="button" className="btn ghost icon-only xs cal-more" aria-label={`Options for ${row.name}`} aria-haspopup="menu" aria-expanded={!!menu} onClick={e => setMenu(rectOf(e.currentTarget))}><Icon name="more" size={15} /></button>
      {menu && <Menu anchor={menu} items={items} onClose={() => setMenu(null)} width={230} />}
    </li>
  )
}
