// Small window events between the shell and the pages.
import { useEffect, useRef } from 'react'
import type { Item } from '../lib/types'

/** Something was added or changed: the sidebar counts and the open page reload. */
export const announceChange = () => window.dispatchEvent(new CustomEvent('pi:changed'))
export function useOnChange(cb: () => void) {
  const ref = useRef(cb)
  ref.current = cb
  useEffect(() => {
    const h = () => ref.current()
    window.addEventListener('pi:changed', h)
    return () => window.removeEventListener('pi:changed', h)
  }, [])
}
/** Open the new-item form from anywhere; `preset` pre-fills it (a place, a category). */
export const openNewItem = (preset: Partial<Item> = {}) => window.dispatchEvent(new CustomEvent('pi:new-item', { detail: preset }))
export const openShortcuts = () => window.dispatchEvent(new CustomEvent('pi:shortcuts'))
