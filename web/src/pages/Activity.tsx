import { useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import type { Activity as ActivityT } from '../lib/types'
import { ago, qty } from '../lib/format'
import { Card, Empty, Loading, PageHeader, useAsync } from '../components/ui'
import { useOnChange } from '../components/events'

export default function Activity() {
  const navigate = useNavigate()
  const { data, reload } = useAsync(() => api.get<ActivityT[]>(`${V1}/activity?limit=100`), [])
  useOnChange(() => { reload() })
  if (!data) return <Loading />
  return (
    <>
      <PageHeader title="Activity" sub="Every quantity change, newest first — and who made it." />
      <Card flush>
        {data.length === 0 ? <div className="pad"><Empty icon="activity" title="Nothing happened yet" /></div> : (
          <ul className="history activity-feed">
            {data.map(m => (
              <li key={m.id}>
                <span className={`d ${m.delta > 0 ? 'plus' : 'minus'}`}>{m.delta > 0 ? '+' : ''}{qty(m.delta)}</span>
                <span className="grow"><a href={`/items/${m.item_id}`} onClick={e => { e.preventDefault(); navigate(`/items/${m.item_id}`) }}>{m.item_name}</a> → {qty(m.quantity_after)}{m.unit ? ` ${m.unit}` : ''}{m.note ? <span className="muted"> · {m.note}</span> : null}{m.user_name ? <span className="muted"> · {m.user_name}</span> : null}</span>
                <span className="when" title={m.created_at}>{ago(m.created_at)}</span>
              </li>
            ))}
          </ul>
        )}
      </Card>
    </>
  )
}
