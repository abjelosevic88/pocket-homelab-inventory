// The app shell: sidebar, top bar, search palette, shortcuts — the Pocket Calendar layout.
import { useEffect, useRef, useState } from 'react'
import { Outlet, useLocation, useMatch, useNavigate, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { colorFor } from '../lib/format'
import type { Category, Dashboard, Item, Location } from '../lib/types'
import { Icon } from './icons'
import { Sidebar } from './Sidebar'
import { CommandPalette, type PaletteCommand } from './CommandPalette'
import { ShortcutsSheet } from './ShortcutsSheet'
import { ItemModal } from '../pages/Items'
import { announceChange, openShortcuts, useOnChange } from './events'
import { Avatar, Field, Modal, type MenuItem, MenuButton, modalOpen, useIsMobile, useMediaQuery, useToast } from './ui'

const SIDEBAR_KEY = 'pi-sidebar'

/** The figures behind the sidebar badges; reloads whenever something changes. */
function useDashboard() {
  const [d, setD] = useState<Dashboard | null>(null)
  const load = () => { api.get<Dashboard>(`${V1}/dashboard`).then(setD).catch(() => {}) }
  useEffect(load, [])
  useOnChange(load)
  return d
}

export type NameKind = 'locations' | 'categories'

export default function Layout() {
  const { reloadMeta } = useApp()
  const navigate = useNavigate()
  const loc = useLocation()
  const mobile = useIsMobile()
  const narrow = useMediaQuery('(max-width: 1099px)')
  const drawer = mobile || narrow
  const [sidebarOpen, setSidebarOpen] = useState(() => !drawer && localStorage.getItem(SIDEBAR_KEY) !== '0')
  useEffect(() => { setSidebarOpen(!drawer && localStorage.getItem(SIDEBAR_KEY) !== '0') }, [drawer])
  const drawerOpenedAt = useRef(0)
  const toggleSidebar = () => setSidebarOpen(o => { drawerOpenedAt.current = performance.now(); const n = !o; if (!drawer) localStorage.setItem(SIDEBAR_KEY, n ? '1' : '0'); return n })
  const closeDrawer = () => { if (drawer) setSidebarOpen(false) }
  useEffect(() => { document.querySelector('.content')?.scrollTo({ top: 0 }) }, [loc.pathname])

  const dash = useDashboard()
  const [tags, setTags] = useState<string[]>([])
  const loadTags = () => { api.get<string[]>(`${V1}/tags`).then(t => setTags(t || [])).catch(() => {}) }
  useEffect(loadTags, [])
  useOnChange(loadTags)

  const [palette, setPalette] = useState(false)
  const [shortcuts, setShortcuts] = useState(false)
  const [newItem, setNewItem] = useState<Partial<Item> | null>(null)
  const [nameModal, setNameModal] = useState<NameKind | null>(null)

  const create = (preset: Partial<Item> = {}) => { closeDrawer(); setNewItem(preset) }
  const createRef = useRef(create)
  createRef.current = create

  useEffect(() => {
    const sc = () => setShortcuts(true)
    const ni = (e: Event) => createRef.current((e as CustomEvent<Partial<Item>>).detail || {})
    window.addEventListener('pi:shortcuts', sc)
    window.addEventListener('pi:new-item', ni)
    return () => { window.removeEventListener('pi:shortcuts', sc); window.removeEventListener('pi:new-item', ni) }
  }, [])
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); setPalette(p => !p); return }
      if (e.metaKey || e.ctrlKey || e.altKey) return
      if (t.closest('input, textarea, select, [contenteditable="true"]') || modalOpen() || document.querySelector('.menu, .popover, .sel-pop')) return
      const go = (to: string) => { e.preventDefault(); navigate(to) }
      switch (e.key) {
        case '/': e.preventDefault(); setPalette(true); break
        case '?': e.preventDefault(); setShortcuts(true); break
        case 'n': e.preventDefault(); createRef.current(); break
        case 'i': go('/items'); break
        case 's': go('/shopping'); break
        case 'l': go('/items?view=low'); break
        case 'p': go('/places'); break
        case 'a': go('/activity'); break
        case ',': go('/settings'); break
        case '[': e.preventDefault(); toggleSidebar(); break
      }
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }) // eslint-disable-line react-hooks/exhaustive-deps

  const commands: PaletteCommand[] = [
    { id: 'new', label: 'New item', icon: 'plus', hint: 'N', keywords: 'add create', run: () => create() },
    { id: 'new-place', label: 'New place', icon: 'pin', keywords: 'add location room', run: () => setNameModal('locations') },
    { id: 'new-cat', label: 'New category', icon: 'tag', keywords: 'add', run: () => setNameModal('categories') },
    { id: 'items', label: 'Items', icon: 'package', hint: 'I', run: () => navigate('/items') },
    { id: 'low', label: 'Running low', icon: 'alert', hint: 'L', keywords: 'low stock', run: () => navigate('/items?view=low') },
    { id: 'expiring', label: 'Expiring soon', icon: 'hourglass', keywords: 'expiry warranty', run: () => navigate('/items?view=expiring') },
    { id: 'pantry', label: 'Pantry', icon: 'pantry', keywords: 'food kitchen', run: () => navigate('/items?tag=pantry') },
    { id: 'archived', label: 'Archived items', icon: 'archive', run: () => navigate('/items?view=archived') },
    { id: 'shopping', label: 'Shopping list', icon: 'cart', hint: 'S', keywords: 'buy', run: () => navigate('/shopping') },
    { id: 'places', label: 'Places & categories', icon: 'pin', hint: 'P', run: () => navigate('/places') },
    { id: 'activity', label: 'Activity', icon: 'activity', hint: 'A', keywords: 'history', run: () => navigate('/activity') },
    { id: 'settings', label: 'Settings', icon: 'settings', hint: ',', run: () => navigate('/settings') },
    { id: 'shortcuts', label: 'Keyboard shortcuts', icon: 'keyboard', hint: '?', run: () => setShortcuts(true) },
  ]

  return (
    <div className={`app ${sidebarOpen ? 'with-sidebar' : 'no-sidebar'} ${mobile ? 'is-mobile' : ''} ${drawer ? 'drawer-mode' : ''}`}>
      {drawer && sidebarOpen && <div className="drawer-backdrop" onClick={e => { if (e.timeStamp - drawerOpenedAt.current > 400) setSidebarOpen(false) }} />}
      <div className={`sidebar-wrap ${sidebarOpen ? 'open' : ''}`}>
        <Sidebar dash={dash} tags={tags} onCreate={create} onNewName={k => { closeDrawer(); setNameModal(k) }} onNavigate={closeDrawer} />
      </div>
      <div className="main">
        <TopBar onMenu={toggleSidebar} onPalette={() => setPalette(true)} mobile={mobile} compact={narrow} sidebarOpen={sidebarOpen} />
        <main className="content"><div className="content-inner"><Outlet /></div></main>
      </div>
      {mobile && <button type="button" className="fab" aria-label="New item" title="New item" onClick={() => create()}><Icon name="plus" size={26} strokeWidth={2} /></button>}
      {palette && <CommandPalette onClose={() => setPalette(false)} commands={commands} />}
      {shortcuts && <ShortcutsSheet onClose={() => setShortcuts(false)} />}
      {newItem && <ItemModal item={newItem} onClose={() => setNewItem(null)} onSaved={() => { setNewItem(null); announceChange() }} />}
      {nameModal && <NameModal kind={nameModal} onClose={() => setNameModal(null)} onSaved={async () => { setNameModal(null); await reloadMeta(); announceChange() }} />}
    </div>
  )
}

/** Add a place or a category by name — the same call the Places page makes. */
function NameModal({ kind, onClose, onSaved }: { kind: NameKind; onClose: () => void; onSaved: () => void }) {
  const { toast } = useToast()
  const [name, setName] = useState('')
  const [busy, setBusy] = useState(false)
  const place = kind === 'locations'
  const save = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    setBusy(true)
    try { await api.post<Location | Category>(`${V1}/${kind}`, { name: name.trim() }); toast(`Added ${name.trim()}`); onSaved() } catch (err) { toast((err as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={place ? 'New place' : 'New category'} onClose={onClose} size="sm">
      <form onSubmit={save} className="stack">
        <Field label="Name" help={place ? 'Where things live: Garage, Pantry, Office drawer…' : 'What things are: Tools, Food, Medicine…'}>
          <input value={name} onChange={e => setName(e.target.value)} autoFocus maxLength={60} placeholder={place ? 'Garage, Pantry, Car boot…' : 'Tools, Cables, Spices…'} />
        </Field>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy || !name.trim()}>Add</button></div>
      </form>
    </Modal>
  )
}

const SECTION: Record<string, string> = { '': 'Items', items: 'Items', shopping: 'Shopping list', places: 'Places & categories', activity: 'Activity' }
const VIEW: Record<string, string> = { low: 'Running low', expiring: 'Expiring soon', archived: 'Archived' }

function TopBar({ onMenu, onPalette, mobile, compact, sidebarOpen }: { onMenu: () => void; onPalette: () => void; mobile: boolean; compact: boolean; sidebarOpen: boolean }) {
  const { user, users, locations, categories, themeMode, setThemeMode, status, setUser } = useApp()
  const navigate = useNavigate()
  const loc = useLocation()
  const [params] = useSearchParams()
  const section = SECTION[loc.pathname.split('/')[1] || ''] || ''
  // an open item names itself in the breadcrumb
  const im = useMatch('/items/:id')
  const itemID = im?.params.id
  const [itemName, setItemName] = useState('')
  useEffect(() => {
    setItemName('')
    if (!itemID) return
    if (itemID === 'new') { setItemName('New item'); return }
    let live = true
    api.get<{ item: Item }>(`${V1}/items/${itemID}`).then(r => { if (live) setItemName(r.item.name) }).catch(() => {})
    return () => { live = false }
  }, [itemID])
  // a filtered item list names its filter
  let sub = ''
  if (section === 'Items') {
    const l = params.get('location'), c = params.get('category'), v = params.get('view'), t = params.get('tag'), u = params.get('user'), q = params.get('q')
    if (l) sub = l === 'none' ? 'No place' : locations.find(x => String(x.id) === l)?.name || ''
    else if (c) sub = c === 'none' ? 'Uncategorised' : categories.find(x => String(x.id) === c)?.name || ''
    else if (v) sub = VIEW[v] || ''
    else if (t) sub = t.toLowerCase() === 'pantry' ? 'Pantry' : `#${t}`
    else if (u) sub = u === 'me' ? 'Mine' : u === 'none' ? 'Not shared' : users.find(x => String(x.id) === u)?.name || ''
    else if (q) sub = `“${q}”`
  }
  const crumb = itemName || sub

  const signOut = async () => { await api.post(`${V1}/auth/logout`).catch(() => {}); setUser(null); navigate('/login') }
  const avatarMenu = (): MenuItem[] => [
    { heading: user?.name || user?.email || '' },
    { label: 'Profile', icon: 'user', onSelect: () => navigate('/settings/profile') },
    { label: 'Settings', icon: 'settings', onSelect: () => navigate('/settings'), hint: ',' },
    { label: 'Keyboard shortcuts', icon: 'keyboard', onSelect: openShortcuts, hint: '?' },
    'sep',
    { heading: 'Appearance' },
    { label: 'System', checked: themeMode === 'system', onSelect: () => setThemeMode('system') },
    { label: 'Light', checked: themeMode === 'light', onSelect: () => setThemeMode('light') },
    { label: 'Dark', checked: themeMode === 'dark', onSelect: () => setThemeMode('dark') },
    ...(status?.auth_mode === 'local' ? ['sep' as const, { label: 'Sign out', icon: 'log-out', onSelect: signOut }] : []),
  ]
  return (
    <header className="topbar">
      <button type="button" className="btn ghost icon-only" onClick={onMenu} aria-label={sidebarOpen ? 'Hide the sidebar' : 'Show the sidebar'} title="Menu ([)"><Icon name={mobile || compact ? 'menu' : 'sidebar'} size={18} /></button>
      <h1 className="range-title">
        {crumb ? <><button type="button" className="crumb" onClick={() => navigate('/items')}>{section}</button><Icon name="chevron-right" size={16} className="crumb-sep" /><span className="ellipsis">{crumb}</span></> : section}
      </h1>
      <span className="spacer" />
      <button type="button" className="btn ghost icon-only" onClick={onPalette} aria-label="Search" title="Search (⌘K or /)"><Icon name="search" size={18} /></button>
      <MenuButton label="Account" className="avatar-btn" align="right" width={220} items={avatarMenu}>
        <Avatar name={user?.name} email={user?.email} size={30} color={user ? colorFor(user.id) : undefined} />
      </MenuButton>
    </header>
  )
}
