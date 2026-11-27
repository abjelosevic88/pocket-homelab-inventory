-- One day of the activity log is read by time (Pocket Day asks "what happened
-- here on this day"), without going through the whole log.
CREATE INDEX IF NOT EXISTS movements_created ON movements(created_at);
