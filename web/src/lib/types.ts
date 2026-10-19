export interface User { id: number; email: string; name: string; role: 'admin' | 'member'; created_at: string }
export interface UserRef { id: number; name: string }

export interface Settings {
  title: string
  currency: string
  default_unit: string
  landing: 'items' | 'shopping'
  expiry_warn_days: number
  setup_complete: boolean
}

export interface Location { id: number; name: string; description: string; sort_order: number; item_count: number; created_at: string }
export interface Category { id: number; name: string; color: string; sort_order: number; item_count: number; created_at: string }

export interface Item {
  id: number
  name: string
  notes: string
  category_id: number | null
  location_id: number | null
  quantity: number
  unit: string
  min_quantity: number
  barcode: string
  brand: string
  model: string
  serial: string
  purchase_date: string
  price: number
  expiry_date: string
  tags: string
  url: string
  photo: string
  archived: boolean
  created_at: string
  updated_at: string
  owner_id: number
  user_ids: number[]
  users: UserRef[]
  category_name: string
  location_name: string
  low: boolean
}

export interface Movement { id: number; item_id: number; user_id: number | null; user_name: string; delta: number; quantity_after: number; note: string; created_at: string }
export interface Activity extends Movement { item_name: string; unit: string }

export interface Dashboard {
  items: number; quantity_total: number; low_stock: number; total_value: number; currency: string
  locations: number; categories: number; users: number; expiring: number; expired: number; updated_today: number; archived: number
}

export interface Shopping { groups: { location: string; items: Item[] }[]; low_count: number; expiring: Item[]; today: string }

export interface APIToken { id: number; name: string; prefix: string; last_used_at: string | null; created_at: string }
