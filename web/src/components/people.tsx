import type { User, UserRef } from '../lib/types'
import { colorFor, initials } from '../lib/format'

export function Avatar({ user, size = '' }: { user: UserRef | User; size?: '' | 'lg' }) {
  const name = user.name || ('email' in user ? user.email : '')
  return <span className={`avatar ${size}`} style={{ background: colorFor(user.id) }} title={name}>{initials(name)}</span>
}

export function Avatars({ users }: { users: UserRef[] }) {
  if (!users.length) return null
  return <span className="avatars" title={users.map(u => u.name).join(', ')}>{users.map(u => <Avatar key={u.id} user={u} />)}</span>
}

/** Toggle chips for linking an item to people. */
export function PeoplePicker({ users, value, onChange }: { users: User[]; value: number[]; onChange: (ids: number[]) => void }) {
  if (users.length === 0) return <p className="muted small">No other users yet — add them under Settings → Users.</p>
  const toggle = (id: number) => onChange(value.includes(id) ? value.filter(x => x !== id) : [...value, id])
  return (
    <div className="people">
      <button type="button" className={`chip ${value.length === 0 ? 'active' : ''}`} onClick={() => onChange([])}>Everyone</button>
      {users.map(u => <button type="button" key={u.id} className={`chip ${value.includes(u.id) ? 'active' : ''}`} onClick={() => toggle(u.id)}><Avatar user={u} />{u.name || u.email}</button>)}
    </div>
  )
}
