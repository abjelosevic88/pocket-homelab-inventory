import type { User, UserRef } from '../lib/types'
import { colorFor, initials } from '../lib/format'

/** A person's initials in their household colour (stable per account). */
export function Avatar({ user, size = 22 }: { user: UserRef | User; size?: number }) {
  const name = user.name || ('email' in user ? user.email : '')
  return <span className="avatar" style={{ width: size, height: size, fontSize: Math.round(size * 0.42), background: colorFor(user.id) }} title={name}>{initials(name)}</span>
}

export function Avatars({ users }: { users: UserRef[] }) {
  if (!users.length) return null
  return <span className="avatars" title={users.map(u => u.name).join(', ')}>{users.map(u => <Avatar key={u.id} user={u} size={20} />)}</span>
}

/** Toggle chips for linking an item to people. */
export function PeoplePicker({ users, value, onChange }: { users: User[]; value: number[]; onChange: (ids: number[]) => void }) {
  if (users.length === 0) return <p className="muted small">No other users yet — add them under Settings → People.</p>
  const toggle = (id: number) => onChange(value.includes(id) ? value.filter(x => x !== id) : [...value, id])
  return (
    <div className="people">
      <button type="button" className={`chip ${value.length === 0 ? 'active' : ''}`} onClick={() => onChange([])}>Everyone</button>
      {users.map(u => <button type="button" key={u.id} className={`chip ${value.includes(u.id) ? 'active' : ''}`} onClick={() => toggle(u.id)}><Avatar user={u} size={18} />{u.name || u.email}</button>)}
    </div>
  )
}
