import { useEffect, useState } from 'react'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { APIToken, Settings as SettingsT, User } from '../lib/types'
import { Card, Confirm, Field, Loading, Modal, PageHeader, Segmented, Tabs, useAsync, useToast } from '../components/ui'
import { Avatar } from '../components/people'

type Tab = 'general' | 'users' | 'profile' | 'api' | 'data' | 'about'

export default function Settings() {
  const [tab, setTab] = useState<Tab>('general')
  return (
    <>
      <PageHeader title="Settings" />
      <Tabs tabs={[{ id: 'general', label: 'General' }, { id: 'users', label: 'Users' }, { id: 'profile', label: 'Profile' }, { id: 'api', label: 'API tokens' }, { id: 'data', label: 'Backup & data' }, { id: 'about', label: 'About' }]} value={tab} onChange={setTab} />
      <div className="mt">
        {tab === 'general' && <General />}
        {tab === 'users' && <Users />}
        {tab === 'profile' && <Profile />}
        {tab === 'api' && <Tokens />}
        {tab === 'data' && <Data />}
        {tab === 'about' && <About />}
      </div>
    </>
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
  if (!f) return <Loading />
  return (
    <Card title="Preferences">
      <div className="grid" style={{ gap: 14, maxWidth: 560 }}>
        <Field label="App title" help="Shown in the sidebar and browser tab."><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} maxLength={40} /></Field>
        <div className="two-col tight">
          <Field label="Currency" help="ISO code for prices and the total value."><input value={f.currency} onChange={e => setF({ ...f, currency: e.target.value.toUpperCase() })} maxLength={3} placeholder="EUR" /></Field>
          <Field label="Default unit" help="Pre-filled for new items."><input value={f.default_unit} onChange={e => setF({ ...f, default_unit: e.target.value })} maxLength={16} placeholder="pcs" /></Field>
        </div>
        <Field label="Expiry warning" help="Days ahead an expiry / warranty date is flagged."><input type="number" min={0} max={3650} value={f.expiry_warn_days} onChange={e => setF({ ...f, expiry_warn_days: Number(e.target.value) })} style={{ maxWidth: 120 }} /></Field>
        <Field label="Open on"><Segmented value={f.landing} onChange={v => setF({ ...f, landing: v })} options={[{ value: 'items', label: 'Items' }, { value: 'shopping', label: 'Shopping list' }]} /></Field>
        <div><button className="btn primary" onClick={save} disabled={busy}>Save</button></div>
      </div>
    </Card>
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
    <Card title="Household" actions={isAdmin && <button className="btn primary sm" onClick={() => setEdit({ role: 'member' })}>+ Add user</button>}>
      <p className="muted">Everyone here signs in with their own account and sees the same inventory. Items can be linked to one or more people (“whose is it?”), and every quantity change records who made it. Admins manage users; members can do everything else.</p>
      <table className="table compact">
        <thead><tr><th /><th>Name</th><th>Email</th><th>Role</th><th /></tr></thead>
        <tbody>
          {users.map(u => (
            <tr key={u.id}>
              <td style={{ width: 32 }}><Avatar user={u} /></td>
              <td>{u.name || <span className="muted">—</span>}{u.id === user?.id && <span className="badge" style={{ marginLeft: 6 }}>you</span>}</td>
              <td className="muted">{u.email}</td>
              <td><span className={`badge ${u.role === 'admin' ? 'cat' : ''}`}>{u.role}</span></td>
              <td className="right">{isAdmin && <span className="row" style={{ justifyContent: 'flex-end' }}><button className="btn ghost sm" onClick={() => setEdit(u)}>Edit</button>{u.id !== user?.id && <button className="btn ghost sm danger-text" onClick={() => setDel(u)}>Remove</button>}</span>}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {status?.auth_mode !== 'local' && <p className="muted small mt">Auth mode is <code>{status?.auth_mode}</code>: users are created automatically on first visit; passwords are not used here.</p>}
      {edit && <UserModal user={edit} local={status?.auth_mode === 'local'} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); reloadMeta() }} />}
      {del && <Confirm title={`Remove ${del.name || del.email}?`} message="Their account and sessions are deleted. Items stay; they are just no longer linked to this person and their past changes show without a name." onCancel={() => setDel(null)} onConfirm={() => { const d = del; setDel(null); run(() => api.del(`${V1}/users/${d.id}`), 'User removed') }} />}
    </Card>
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
    <Modal title={user.id ? 'Edit user' : 'Add user'} onClose={onClose}>
      <form onSubmit={save} className="grid" style={{ gap: 14 }}>
        <Field label="Name" help="Shown on items and in the history."><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} autoFocus required maxLength={60} /></Field>
        <Field label="Email" help="Used to sign in."><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} required /></Field>
        {local && <Field label={user.id ? 'New password (leave empty to keep)' : 'Password'} help="At least 8 characters. Tell them to change it under Settings → Profile."><input type="password" value={f.password} onChange={e => setF({ ...f, password: e.target.value })} minLength={user.id ? 0 : 8} required={!user.id} autoComplete="new-password" /></Field>}
        <Field label="Role"><Segmented value={f.role} onChange={v => setF({ ...f, role: v })} options={[{ value: 'member', label: 'Member' }, { value: 'admin', label: 'Admin' }]} /></Field>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>{user.id ? 'Save' : 'Add user'}</button></div>
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
    <Card title="Profile">
      <form onSubmit={save} className="grid" style={{ gap: 14, maxWidth: 480 }}>
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} /></Field>
        <Field label="Email"><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} /></Field>
        {status?.auth_mode === 'local' && <>
          <hr />
          <Field label="Current password"><input type="password" value={f.current_password} onChange={e => setF({ ...f, current_password: e.target.value })} autoComplete="current-password" /></Field>
          <Field label="New password" help="At least 8 characters. Leave empty to keep the current one."><input type="password" value={f.new_password} onChange={e => setF({ ...f, new_password: e.target.value })} autoComplete="new-password" /></Field>
        </>}
        <div><button className="btn primary" disabled={busy}>Save</button></div>
      </form>
    </Card>
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
    <Card title="API tokens">
      <p className="muted">Use as <code>Authorization: Bearer &lt;token&gt;</code>. Good for Homepage widgets (<code>GET /api/v1/dashboard</code>), barcode lookups from iOS Shortcuts (<code>GET /api/v1/lookup?barcode=…</code>) and quantity changes (<code>POST /api/v1/items/{'{id}'}/adjust</code>). See <code>docs/api.md</code>.</p>
      <form onSubmit={create} className="row mb"><input value={name} onChange={e => setName(e.target.value)} placeholder="Token name (e.g. Homepage)" /><button className="btn primary">Create</button></form>
      {fresh && <div className="callout success mb"><b>Copy it now — it is shown once.</b><pre className="mono" style={{ margin: '8px 0 0' }}>{fresh}</pre></div>}
      {data && data.length > 0 ? (
        <table className="table compact">
          <thead><tr><th>Name</th><th>Prefix</th><th>Last used</th><th>Created</th><th /></tr></thead>
          <tbody>{data.map(t => <tr key={t.id}><td>{t.name}</td><td className="mono">{t.prefix}…</td><td className="muted">{t.last_used_at ? t.last_used_at.slice(0, 16).replace('T', ' ') : 'never'}</td><td className="muted">{t.created_at.slice(0, 10)}</td><td className="right"><button className="btn ghost sm danger-text" onClick={() => setDel(t)}>Revoke</button></td></tr>)}</tbody>
        </table>
      ) : <p className="muted">No tokens yet.</p>}
      {del && <Confirm title={`Revoke “${del.name}”?`} message="Anything using this token stops working immediately." onCancel={() => setDel(null)} onConfirm={async () => { await api.del(`${V1}/tokens/${del.id}`); setDel(null); reload() }} />}
    </Card>
  )
}

function Data() {
  const { toast } = useToast()
  const { reloadMeta } = useApp()
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
    <div className="two-col">
      <Card title="Backup">
        <p className="muted">A consistent copy of the SQLite database (<code>VACUUM INTO</code>). Photos live next to it in <code>photos/</code>. Restore by stopping the app and replacing <code>pocket-inventory.db</code> in the data directory.</p>
        <a className="btn primary" href={`${V1}/backup`}>Download database</a>
      </Card>
      <Card title="Export & import">
        <p className="muted">JSON (full, re-importable) or CSV (one row per item — open it in a spreadsheet, or use it to bulk-load a new inventory).</p>
        <div className="row mb wrap"><a className="btn" href={`${V1}/export.json`}>Export JSON</a><a className="btn" href={`${V1}/export.csv`}>Export CSV</a></div>
        <div className="row wrap">
          <label className="btn ghost">{busy ? 'Importing…' : 'Import JSON…'}<input type="file" accept="application/json" hidden onChange={e => e.target.files?.[0] && importJSON(e.target.files[0])} /></label>
          <label className="btn ghost">{busy ? 'Importing…' : 'Import CSV…'}<input type="file" accept=".csv,text/csv" hidden onChange={e => e.target.files?.[0] && importCSV(e.target.files[0])} /></label>
        </div>
        <p className="muted small mt">Import merges: places and categories are matched by name, items by name + place (updated if found, created if not). CSV needs a <code>name</code> column; <code>quantity, unit, min_quantity, location, category, owners, tags, brand, model, serial, barcode, price, purchase_date, expiry_date, url, notes</code> are optional.</p>
      </Card>
    </div>
  )
}

function About() {
  const { data } = useAsync(() => api.get<Record<string, unknown>>(`${V1}/settings/system`), [])
  const counts = (data?.counts || {}) as Record<string, number>
  return (
    <Card title="About">
      <p><b>Pocket Inventory</b> is a home inventory for households: what you own, how many, where it is and whose it is. Quick add, one-tap plus/minus, a shopping list that fills itself from low-stock alerts, expiry and warranty reminders, photos, barcodes, a history of who changed what. One binary, one SQLite file, one volume.</p>
      <p className="muted small">Open source (MIT) — <a href="https://github.com/abjelosevic88/pocket-homelab-inventory" target="_blank" rel="noreferrer">github.com/abjelosevic88/pocket-homelab-inventory</a>. Sibling projects: Pocket Habits and Pocket Invoicing.</p>
      {data && (
        <table className="table compact mt">
          <tbody>
            <tr><th>Version</th><td className="mono">{String(data.version)}</td></tr>
            <tr><th>Runtime</th><td className="mono">{String(data.go)} · {String(data.os)}/{String(data.arch)}</td></tr>
            <tr><th>Auth mode</th><td className="mono">{String(data.auth_mode)}</td></tr>
            <tr><th>Database</th><td className="mono">{String(data.db_path)} · {(Number(data.db_bytes) / 1024).toFixed(0)} KB</td></tr>
            <tr><th>Photos</th><td className="mono">{String(data.photos)} files · {(Number(data.photo_bytes) / 1024 / 1024).toFixed(1)} MB</td></tr>
            <tr><th>Timezone</th><td className="mono">{String(data.timezone)}</td></tr>
            <tr><th>Uptime</th><td className="mono">{String(data.uptime)}</td></tr>
            <tr><th>Rows</th><td className="mono">{counts.items} items · {counts.locations} places · {counts.categories} categories · {counts.movements} movements · {counts.users} users</td></tr>
          </tbody>
        </table>
      )}
    </Card>
  )
}
