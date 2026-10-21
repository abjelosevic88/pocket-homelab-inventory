// ⌘K / "/" — jump to an item, a place or a category, or run a command from the keyboard.
import React, { useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { qty } from '../lib/format'
import type { Item } from '../lib/types'
import { Icon } from './icons'
import { useDebounce } from './ui'

export interface PaletteCommand { id: string; label: string; icon: string; hint?: string; keywords?: string; run: () => void }

export function CommandPalette({ onClose, commands }: { onClose: () => void; commands: PaletteCommand[] }) {
  const { locations, categories } = useApp()
  const navigate = useNavigate()
  const [q, setQ] = useState('')
  const [hi, setHi] = useState(0)
  const list = useRef<HTMLDivElement>(null)
  const dq = useDebounce(q.trim(), 150)
  // items come from the server: matches for what you typed, else the ones changed last
  const [items, setItems] = useState<Item[]>([])
  useEffect(() => {
    let live = true
    const url = dq ? `${V1}/items?q=${encodeURIComponent(dq)}&sort=name` : `${V1}/items?sort=updated`
    api.get<Item[]>(url).then(r => { if (live) setItems((r || []).slice(0, dq ? 8 : 5)) }).catch(() => { if (live) setItems([]) })
    return () => { live = false }
  }, [dq])

  type Row = { key: string; group: string; icon: React.ReactNode; label: React.ReactNode; sub?: React.ReactNode; hint?: string; run: () => void }
  const rows = useMemo<Row[]>(() => {
    const out: Row[] = []
    const n = q.trim().toLowerCase()
    const words = n.split(/\s+/).filter(Boolean)
    const hit = (text: string) => words.every(w => text.toLowerCase().includes(w))
    for (const it of items) {
      out.push({
        key: `i${it.id}`, group: n ? 'Items' : 'Recently changed',
        icon: it.photo ? <img className="pal-thumb" src={`${V1}/items/${it.id}/photo?v=${encodeURIComponent(it.photo)}`} alt="" /> : <Icon name="package" size={16} />,
        label: it.name, sub: [`${qty(it.quantity)}${it.unit ? ' ' + it.unit : ''}`, it.location_name, it.low && 'running low', it.archived && 'archived'].filter(Boolean).join(' · '),
        run: () => navigate(`/items/${it.id}`),
      })
    }
    if (n) {
      for (const l of locations) if (hit(l.name)) out.push({ key: `l${l.id}`, group: 'Places', icon: <Icon name="pin" size={16} />, label: l.name, sub: `${l.item_count} item${l.item_count === 1 ? '' : 's'}`, run: () => navigate(`/items?location=${l.id}`) })
      for (const c of categories) if (hit(c.name)) out.push({ key: `c${c.id}`, group: 'Categories', icon: <Icon name="tag" size={16} />, label: c.name, sub: `${c.item_count} item${c.item_count === 1 ? '' : 's'}`, run: () => navigate(`/items?category=${c.id}`) })
      out.push({ key: 'search', group: 'Search', icon: <Icon name="search" size={16} />, label: <>All items matching “{q.trim()}”</>, run: () => navigate(`/items?q=${encodeURIComponent(q.trim())}`) })
    }
    for (const c of commands) {
      if (n && !hit(`${c.label} ${c.keywords || ''}`)) continue
      out.push({ key: c.id, group: 'Commands', icon: <Icon name={c.icon} size={16} />, label: c.label, hint: c.hint, run: c.run })
    }
    return out
  }, [q, items, locations, categories, commands, navigate])
  useEffect(() => setHi(0), [q])
  useEffect(() => { list.current?.querySelector(`[data-i="${hi}"]`)?.scrollIntoView({ block: 'nearest' }) }, [hi])

  const run = (r: Row | undefined) => { if (!r) return; onClose(); r.run() }
  let last = ''
  return createPortal(
    <div className="palette-backdrop" onMouseDown={e => { if (e.target === e.currentTarget) onClose() }}>
      <div className="palette-box" role="dialog" aria-label="Search and commands">
        <div className="pal-input">
          <Icon name="search" size={18} />
          <input autoFocus value={q} onChange={e => setQ(e.target.value)} placeholder="Find an item, a place, or run a command…" aria-label="Search"
            onKeyDown={e => {
              if (e.key === 'ArrowDown') { e.preventDefault(); setHi(h => Math.min(rows.length - 1, h + 1)) }
              else if (e.key === 'ArrowUp') { e.preventDefault(); setHi(h => Math.max(0, h - 1)) }
              else if (e.key === 'Enter') { e.preventDefault(); run(rows[hi]) }
              else if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); onClose() }
            }} />
          <kbd>esc</kbd>
        </div>
        <div className="pal-list" ref={list} role="listbox">
          {rows.length === 0 && <div className="pal-empty">Nothing matches.</div>}
          {rows.map((r, i) => {
            const head = r.group !== last ? <div className="pal-group" key={'g' + r.group}>{r.group}</div> : null
            last = r.group
            return (
              <React.Fragment key={r.key}>
                {head}
                <div data-i={i} role="option" aria-selected={i === hi} className={`pal-row ${i === hi ? 'hi' : ''}`} onMouseEnter={() => setHi(i)} onClick={() => run(r)}>
                  <span className="pal-ico">{r.icon}</span>
                  <span className="grow"><span className="pal-label">{r.label}</span>{r.sub && <span className="pal-sub">{r.sub}</span>}</span>
                  {r.hint && <kbd>{r.hint}</kbd>}
                  {i === hi && <Icon name="chevron-right" size={14} className="muted" />}
                </div>
              </React.Fragment>
            )
          })}
        </div>
      </div>
    </div>, document.body)
}
