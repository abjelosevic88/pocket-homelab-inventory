import React, { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { APIToken, Settings as SettingsT, User } from '../lib/types'
import { Confirm, Field, Loading, Modal, Segmented, SettingRow, Tabs, useAsync, useIsMobile, useToast } from '../components/ui'
import { Icon } from '../components/icons'
import { Avatar } from '../components/people'
import { ShortcutsSheet } from '../components/ShortcutsSheet'

type TabId = 'general' | 'profile' | 'people' | 'tokens' | 'data' | 'about'

const TABS: { id: TabId; label: string; icon: string; sub: string }[] = [
  { id: 'general', label: 'General', icon: 'settings', sub: 'Currency, units, expiry warnings, the page it opens on, and the app title.' },
  { id: 'profile', label: 'Profile', icon: 'user', sub: 'Your name, email and password.' },
  { id: 'people', label: 'People', icon: 'users', sub: 'Everyone in the household who has an account on this box.' },
  { id: 'tokens', label: 'API tokens', icon: 'key', sub: 'Personal tokens for Homepage widgets, iOS Shortcuts and scripts.' },
  { id: 'data', label: 'Backup & data', icon: 'database', sub: 'Download the database, export your inventory, import a JSON or CSV file.' },
  { id: 'about', label: 'About', icon: 'info', sub: 'Version, numbers and the server.' },
]
// other names for the same sections (old tab names, ?tab= links)
const ALIAS: Record<string, TabId> = { users: 'people', api: 'tokens', backup: 'data' }

export default function Settings() {
  const { tab: raw } = useParams()
  const navigate = useNavigate()
  const mobile = useIsMobile()
  const legacy = new URLSearchParams(window.location.search).get('tab') || ''
  const want = raw || legacy
  const tab: TabId = (TABS.find(t => t.id === (ALIAS[want] || want))?.id) || 'general'
  const current = TABS.find(t => t.id === tab)!
  const go = (id: TabId) => navigate(`/settings/${id}`, { replace: true })
  const [shortcuts, setShortcuts] = useState(false)

  useEffect(() => { document.querySelector('.set-content')?.scrollTo({ top: 0 }) }, [tab])
  // /settings?tab=users and the like land on /settings/people
  useEffect(() => { if (!raw && legacy) navigate(`/settings/${tab}`, { replace: true }) }, [raw, legacy, tab, navigate])
  useEffect(() => {
    const h = (e: KeyboardEvent) => { if (e.key === 'Escape' && !document.querySelector('.modal-backdrop, .menu, .sel-pop')) navigate('/') }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [navigate])

  return (
    <div className="set-page">
      <header className="set-header">
        <button type="button" className="btn ghost icon-only" aria-label="Back to the inventory" title="Back to the inventory (esc)" onClick={() => navigate('/')}><Icon name="chevron-left" size={18} /></button>
        <img src="/favicon.svg" alt="" width={24} height={24} className="set-logo" />
        <h1>Settings</h1>
      </header>
      {mobile && <div className="set-tabs-mobile"><Tabs tabs={TABS.map(t => ({ id: t.id, label: t.label, icon: t.icon }))} value={tab} onChange={go} /></div>}
      <div className="set-body">
        {!mobile && (
          <nav className="set-nav" aria-label="Settings sections">
            {TABS.map(t => (
              <button key={t.id} type="button" className={`set-nav-item ${t.id === tab ? 'active' : ''}`} aria-current={t.id === tab ? 'page' : undefined} onClick={() => go(t.id)}>
                <Icon name={t.icon} size={16} />{t.label}
              </button>
            ))}
          </nav>
        )}
        <main className="set-content">
          <div className="set-inner" key={tab}>
            <div className="set-title">
              <h2>{current.label}</h2>
              <p>{current.sub}</p>
            </div>
            {tab === 'general' && <General />}
            {tab === 'profile' && <Profile />}
            {tab === 'people' && <Users />}
            {tab === 'tokens' && <Tokens />}
            {tab === 'data' && <Data />}
            {tab === 'about' && <About onShortcuts={() => setShortcuts(true)} />}
          </div>
        </main>
      </div>
      {shortcuts && <ShortcutsSheet onClose={() => setShortcuts(false)} />}
    </div>
  )
}

/** A settings card: title, explanation, actions on the right — Pocket Calendar's section. */
function Section({ title, sub, icon, actions, children }: { title: React.ReactNode; sub?: React.ReactNode; icon?: string; actions?: React.ReactNode; children?: React.ReactNode }) {
  return (
    <section className="set-section">
      <div className="set-section-head">
        {icon && <span className="set-app-ico"><Icon name={icon} size={17} /></span>}
        <div className="grow"><h3>{title}</h3>{sub && <p>{sub}</p>}</div>
        {actions && <div className="row">{actions}</div>}
      </div>
      {children && <div className="set-section-body">{children}</div>}
    </section>
  )
}

function useSettingsForm() {
  const { settings, setSettings } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState<SettingsT | null>(settings)
  const [busy, setBusy] = useState(false)
  // Settings may arrive after the tab mounted (direct load of /settings): adopt them once.
  useEffect(() => { if (!f && settings) setF(settings) }, [settings]) // eslint-disable-line react-hooks/exhaustive-deps
  const save = async () => {
    if (!f) return
    setBusy(true)
    try { const s = await api.put<SettingsT>(`${V1}/settings`, f); setSettings(s); setF(s); toast('Saved') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return { f, setF, save, busy }
}

function General() {
  const { f, setF, save, busy } = useSettingsForm()
  const { user } = useApp()
  const isAdmin = user?.role === 'admin'
  if (!f) return <Loading />
  return (
    <div className="stack loose">
      <Section title="Your preferences" sub="Yours alone — everyone else keeps theirs.">
        <SettingRow title="Currency" help="ISO code for prices and the total value."><input value={f.currency} onChange={e => setF({ ...f, currency: e.target.value.toUpperCase() })} maxLength={3} placeholder="EUR" style={{ width: 96 }} aria-label="Currency" /></SettingRow>
        <SettingRow title="Default unit" help="Pre-filled for new items."><input value={f.default_unit} onChange={e => setF({ ...f, default_unit: e.target.value })} maxLength={16} placeholder="pcs" style={{ width: 96 }} aria-label="Default unit" /></SettingRow>
        <SettingRow title="Expiry warning" help="Days ahead an expiry or warranty date is flagged."><span className="row"><input type="number" min={0} max={3650} value={f.expiry_warn_days} onChange={e => setF({ ...f, expiry_warn_days: Number(e.target.value) })} style={{ width: 96 }} aria-label="Expiry warning in days" /><span className="muted small">days</span></span></SettingRow>
        <SettingRow title="Open on" help="The page you land on."><Segmented value={f.landing} onChange={v => setF({ ...f, landing: v })} options={[{ value: 'items', label: 'Items' }, { value: 'shopping', label: 'Shopping list' }]} size="sm" /></SettingRow>
      </Section>
      {isAdmin && (
        <Section title="This box" sub="Applies to everyone on this box.">
          <SettingRow title="App title" help="Shown to everyone in the sidebar, the browser tab and on the sign-in page."><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} maxLength={40} style={{ width: 220 }} aria-label="App title" /></SettingRow>
        </Section>
      )}
      <div className="row"><button type="button" className="btn primary" onClick={save} disabled={busy}>Save changes</button></div>
      <Section title="How it fits together">
        <dl className="set-dl">
          <div><dt>Items</dt><dd className="muted">One row per thing: how many, where it lives, what it is and whose it is. − and + record a change in the history.</dd></div>
          <div><dt>Shopping list</dt><dd className="muted">Fills itself: set “Alert below” on an item and it shows up when you run out. “Bought” puts the quantity back.</dd></div>
          <div><dt>Places & categories</dt><dd className="muted">Where things live and what they are. Both are flat on purpose, and both are yours.</dd></div>
          <div><dt>Pantry</dt><dd className="muted">Items tagged <code>pantry</code> are what Pocket Kitchen reads as your pantry.</dd></div>
        </dl>
      </Section>
    </div>
  )
}

function Users() {
  const { user, users, reloadMeta, status } = useApp()
  const { toast } = useToast()
  const [edit, setEdit] = useState<Partial<User> | null>(null)
  const [del, setDel] = useState<User | null>(null)
  const isAdmin = user?.role === 'admin'
  const run = async (fn: () => Promise<unknown>, msg: string) => { try { await fn(); await reloadMeta(); toast(msg) } catch (e) { toast((e as Error).message, 'err') } }
  return (
    <Section title="Household" actions={isAdmin && <button type="button" className="btn primary sm" onClick={() => setEdit({ role: 'member' })}><Icon name="plus" size={14} />Add person</button>}
      sub="Everyone signs in with their own account and keeps their own inventory.">
      <p className="muted small mb">Items, places and categories are private to the person who added them. Share an item with others from its “Whose is it? / shared with” field; they then see it and can use it. Every quantity change records who made it. Admins manage accounts and the app title, nothing more: they do not see anyone else's items.</p>
      <div className="set-people">
        {users.map(u => (
          <div key={u.id} className="set-person">
            <Avatar user={u} size={34} />
            <div className="grow">
              <div className="set-person-name">{u.name || <span className="muted">—</span>}{u.id === user?.id && <span className="badge">you</span>}{u.role === 'admin' && <span className="badge accent">admin</span>}</div>
              <div className="muted small ellipsis">{u.email}</div>
            </div>
            {isAdmin && <span className="row" style={{ justifyContent: 'flex-end' }}><button type="button" className="btn ghost sm" onClick={() => setEdit(u)}>Edit</button>{u.id !== user?.id && <button type="button" className="btn ghost sm danger-text" onClick={() => setDel(u)}>Remove</button>}</span>}
          </div>
        ))}
      </div>
      {status?.auth_mode !== 'local' && <p className="muted small mt">Auth mode is <code>{status?.auth_mode}</code>: users are created automatically on first visit; passwords are not used here.</p>}
      {edit && <UserModal user={edit} local={status?.auth_mode === 'local'} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); reloadMeta() }} />}
      {del && <Confirm confirmLabel="Remove" title={`Remove ${del.name || del.email}?`} message="Their account, sessions and their own items, places and categories are deleted. Items other people shared with them stay with their owners; their past changes there show without a name." onCancel={() => setDel(null)} onConfirm={() => { const d = del; setDel(null); run(() => api.del(`${V1}/users/${d.id}`), 'User removed') }} />}
    </Section>
  )
}

function UserModal({ user, local, onClose, onSaved }: { user: Partial<User>; local: boolean; onClose: () => void; onSaved: () => void }) {
  const { toast } = useToast()
  const [f, setF] = useState({ name: user.name || '', email: user.email || '', password: '', role: user.role || 'member' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try {
      if (user.id) await api.put(`${V1}/users/${user.id}`, f); else await api.post(`${V1}/users`, f)
      toast(user.id ? 'User saved' : 'User added'); onSaved()
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={user.id ? 'Edit person' : 'Add person'} onClose={onClose}>
      <form onSubmit={save} className="stack">
        <Field label="Name" help="Shown on items and in the history."><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} autoFocus required maxLength={60} /></Field>
        <Field label="Email" help="Used to sign in."><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} required /></Field>
        {local && <Field label={user.id ? 'New password (leave empty to keep)' : 'Password'} help="At least 8 characters. Tell them to change it under Settings → Profile."><input type="password" value={f.password} onChange={e => setF({ ...f, password: e.target.value })} minLength={user.id ? 0 : 8} required={!user.id} autoComplete="new-password" /></Field>}
        <Field label="Role" group><Segmented value={f.role} onChange={v => setF({ ...f, role: v })} options={[{ value: 'member', label: 'Member' }, { value: 'admin', label: 'Admin' }]} /></Field>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>{user.id ? 'Save' : 'Add person'}</button></div>
      </form>
    </Modal>
  )
}

function Profile() {
  const { user, setUser, status } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState({ name: user?.name || '', email: user?.email || '', current_password: '', new_password: '' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { setUser(await api.put(`${V1}/auth/me`, f)); setF(x => ({ ...x, current_password: '', new_password: '' })); toast('Profile saved') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <form onSubmit={save} className="stack loose">
      {user && (
        <div className="set-profile">
          <Avatar user={user} size={56} />
          <div className="grow"><h3>{user.name || user.email}</h3><p className="muted small">{user.email} · {user.role}</p></div>
        </div>
      )}
      <Section title="You">
        <div className="stack">
          <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} /></Field>
          <Field label="Email"><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} /></Field>
        </div>
      </Section>
      {status?.auth_mode === 'local' && (
        <Section title="Password" sub="Leave empty to keep the current one.">
          <div className="stack">
            <Field label="Current password"><input type="password" value={f.current_password} onChange={e => setF({ ...f, current_password: e.target.value })} autoComplete="current-password" /></Field>
            <Field label="New password" help="At least 8 characters."><input type="password" value={f.new_password} onChange={e => setF({ ...f, new_password: e.target.value })} autoComplete="new-password" /></Field>
          </div>
        </Section>
      )}
      <div className="row"><button className="btn primary" disabled={busy}>Save changes</button></div>
    </form>
  )
}

function Tokens() {
  const { toast } = useToast()
  const { data, reload } = useAsync(() => api.get<APIToken[]>(`${V1}/tokens`), [])
  const [name, setName] = useState('')
  const [fresh, setFresh] = useState('')
  const [del, setDel] = useState<APIToken | null>(null)
  const create = async (e: React.FormEvent) => {
    e.preventDefault()
    try { const r = await api.post<{ token: string }>(`${V1}/tokens`, { name: name || 'token' }); setFresh(r.token); setName(''); reload() } catch (e) { toast((e as Error).message, 'err') }
  }
  return (
    <Section title="Personal tokens" icon="key" sub={<>Use as <code>Authorization: Bearer &lt;token&gt;</code>. Good for Homepage widgets (<code>GET /api/v1/dashboard</code>), barcode lookups from iOS Shortcuts (<code>GET /api/v1/lookup?barcode=…</code>) and quantity changes (<code>POST /api/v1/items/{'{id}'}/adjust</code>). See <code>docs/api.md</code>.</>}>
      <form onSubmit={create} className="row mb"><input value={name} onChange={e => setName(e.target.value)} placeholder="Token name (e.g. Homepage)" /><button className="btn primary"><Icon name="plus" size={15} />Create</button></form>
      {fresh && <div className="set-fresh mb"><div className="row"><Icon name="check-circle" size={16} /><b>Copy it now — it is shown once.</b></div><pre className="mono">{fresh}</pre></div>}
      {data && data.length > 0 ? (
        <table className="table compact">
          <thead><tr><th>Name</th><th>Prefix</th><th>Last used</th><th>Created</th><th /></tr></thead>
          <tbody>{data.map(t => <tr key={t.id}><td>{t.name}</td><td className="mono">{t.prefix}…</td><td className="muted">{t.last_used_at ? t.last_used_at.slice(0, 16).replace('T', ' ') : 'never'}</td><td className="muted">{t.created_at.slice(0, 10)}</td><td className="right"><button type="button" className="btn ghost sm danger-text" onClick={() => setDel(t)}>Revoke</button></td></tr>)}</tbody>
        </table>
      ) : <p className="muted">No tokens yet.</p>}
      {del && <Confirm confirmLabel="Revoke" title={`Revoke “${del.name}”?`} message="Anything using this token stops working immediately." onCancel={() => setDel(null)} onConfirm={async () => { await api.del(`${V1}/tokens/${del.id}`); setDel(null); reload() }} />}
    </Section>
  )
}

function Data() {
  const { toast } = useToast()
  const { reloadMeta, user } = useApp()
  const [busy, setBusy] = useState(false)
  const importJSON = async (file: File) => {
    setBusy(true)
    try { const r = await api.post<{ created: number; updated: number }>(`${V1}/import`, JSON.parse(await file.text())); toast(`Imported: ${r.created} new, ${r.updated} updated`); reloadMeta() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const importCSV = async (file: File) => {
    setBusy(true)
    try {
      const fd = new FormData(); fd.append('file', file)
      const res = await fetch(`${V1}/import.csv`, { method: 'POST', body: fd, credentials: 'same-origin' })
      const j = await res.json()
      if (!res.ok) throw new Error(j.error || res.statusText)
      toast(`Imported: ${j.created} new, ${j.updated} updated`); reloadMeta()
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <div className="stack loose">
      {user?.role === 'admin' && (
        <Section title="Backup" icon="database" sub="Admin only: the database holds every account's data.">
          <p className="muted">A consistent copy of the SQLite database (<code>VACUUM INTO</code>). Photos live next to it in <code>photos/</code>. Restore by stopping the app and replacing <code>pocket-inventory.db</code> in the data directory.</p>
          <a className="btn primary" href={`${V1}/backup`}><Icon name="download" size={15} />Download database</a>
        </Section>
      )}
      <Section title="Export & import" icon="upload" sub="Your own data: the items you added (not ones shared with you), your places and categories.">
        <p className="muted">JSON (full, re-importable) or CSV (one row per item — open it in a spreadsheet, or use it to bulk-load a new inventory). Imports go into your inventory.</p>
        <div className="row mb wrap"><a className="btn" href={`${V1}/export.json`}><Icon name="download" size={15} />Export JSON</a><a className="btn" href={`${V1}/export.csv`}><Icon name="download" size={15} />Export CSV</a></div>
        <div className="row wrap">
          <label className="btn ghost"><Icon name="upload" size={15} />{busy ? 'Importing…' : 'Import JSON…'}<input type="file" accept="application/json" hidden onChange={e => e.target.files?.[0] && importJSON(e.target.files[0])} /></label>
          <label className="btn ghost"><Icon name="upload" size={15} />{busy ? 'Importing…' : 'Import CSV…'}<input type="file" accept=".csv,text/csv" hidden onChange={e => e.target.files?.[0] && importCSV(e.target.files[0])} /></label>
        </div>
        <p className="muted small mt">Import merges: places and categories are matched by name, items by name + place (updated if found, created if not). CSV needs a <code>name</code> column; <code>quantity, unit, min_quantity, location, category, owners, tags, brand, model, serial, barcode, price, purchase_date, expiry_date, url, notes</code> are optional.</p>
      </Section>
    </div>
  )
}

function About({ onShortcuts }: { onShortcuts: () => void }) {
  const { data, error } = useAsync(() => api.get<Record<string, unknown>>(`${V1}/settings/system`), [])
  if (error) return <div className="callout danger"><Icon name="alert" size={15} /><span>{error}</span></div>
  if (!data) return <Loading />
  const counts = (data.counts || {}) as Record<string, number>
  const tiles: [string, number | undefined, string][] = [['Items', counts.items, 'package'], ['Places', counts.locations, 'pin'], ['Categories', counts.categories, 'tag'], ['Changes', counts.movements, 'activity']]
  const rows: [string, React.ReactNode][] = [
    ['Version', <code key="v">{String(data.version)}</code>],
    ['Uptime', String(data.uptime)],
    ['Runtime', `${String(data.go)} · ${String(data.os)}/${String(data.arch)}`],
    ['Time zone', String(data.timezone)],
    ['Auth mode', String(data.auth_mode)],
  ]
  if (data.db_path !== undefined) rows.push(['Database', <span key="db"><code>{String(data.db_path)}</code> · {(Number(data.db_bytes) / 1024).toFixed(0)} KB</span>])
  if (data.photos !== undefined) rows.push(['Photos', `${String(data.photos)} files · ${(Number(data.photo_bytes) / 1024 / 1024).toFixed(1)} MB`])
  if (data.counts !== undefined) rows.push(['Rows (all accounts)', `${counts.items} items · ${counts.locations} places · ${counts.categories} categories · ${counts.movements} movements · ${counts.users} users`])
  return (
    <div className="stack loose">
      <div className="set-about-hero">
        <img src="/favicon.svg" alt="" width={48} height={48} />
        <div>
          <h3>Pocket Inventory</h3>
          <p className="muted small">A home inventory for households: what you own, how many, where it is and whose it is. Quick add, one-tap plus/minus, a shopping list that fills itself from low-stock alerts, expiry and warranty reminders, photos, barcodes, a history of who changed what. One binary, one SQLite file, one volume.</p>
        </div>
      </div>
      {data.counts !== undefined && (
        <div className="set-stats">
          {tiles.map(([label, n, icon]) => (
            <div key={label} className="set-stat"><Icon name={icon} size={16} /><span className="set-stat-n tnum">{n ?? 0}</span><span className="set-stat-l">{label}</span></div>
          ))}
        </div>
      )}
      <Section title="This server">
        <dl className="set-dl">
          {rows.map(([k, v]) => <div key={k}><dt>{k}</dt><dd>{v}</dd></div>)}
        </dl>
      </Section>
      <p className="muted small">Open source (MIT). Sibling projects: the other Pocket apps — Pocket Kitchen reads your pantry from here.</p>
      <div className="row wrap">
        <button type="button" className="btn" onClick={onShortcuts}><Icon name="keyboard" size={15} />Keyboard shortcuts</button>
        <a className="btn" href="https://github.com/abjelosevic88/pocket-homelab-inventory" target="_blank" rel="noreferrer"><Icon name="external" size={15} />Project on GitHub</a>
      </div>
    </div>
  )
}
