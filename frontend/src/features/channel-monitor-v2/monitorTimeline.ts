/**
 * Build a stable timeline for the V3 monitor.
 *
 * The API returns only buckets that have data. The UI still needs one fixed
 * slot per requested interval, including the trailing (current) slot when
 * aggregation is lagging. `endAt` is the exclusive upper bound supplied by
 * the API, so the final slot starts at `endAt - interval`.
 */
export interface MonitorTimelineSlot<T> {
  timestamp: number
  bucket?: T
}

interface TimelineBucket {
  bucket_start: string
}

function validTimestamp(value: string | undefined): number | null {
  if (!value) return null
  const timestamp = Date.parse(value)
  return Number.isFinite(timestamp) ? timestamp : null
}

function inferIntervalMs(timestamps: number[], bucketSeconds?: number): number {
  if (Number.isFinite(bucketSeconds) && (bucketSeconds ?? 0) > 0) {
    return Math.max(1000, Math.round((bucketSeconds as number) * 1000))
  }

  for (let index = 1; index < timestamps.length; index += 1) {
    const difference = timestamps[index] - timestamps[index - 1]
    if (difference > 0) return difference
  }
  return 5 * 60 * 1000
}

export function buildMonitorTimeline<T extends TimelineBucket>(
  buckets: T[],
  length: number,
  endAt?: string,
  bucketSeconds?: number,
): MonitorTimelineSlot<T>[] {
  const count = Math.max(0, Math.floor(length))
  if (count === 0) return []

  const entries = buckets
    .map(bucket => ({ bucket, timestamp: validTimestamp(bucket.bucket_start) }))
    .filter((entry): entry is { bucket: T; timestamp: number } => entry.timestamp !== null)
    .sort((a, b) => a.timestamp - b.timestamp)
  const timestamps = entries.map(entry => entry.timestamp)
  const intervalMs = inferIntervalMs(timestamps, bucketSeconds)
  const requestedEnd = validTimestamp(endAt)
  const lastTimestamp = timestamps.at(-1)
  const resolvedEnd = requestedEnd ?? (lastTimestamp != null ? lastTimestamp + intervalMs : Date.now())
  const start = resolvedEnd - count * intervalMs
  const slots = Array.from({ length: count }, (_, index) => ({
    timestamp: start + index * intervalMs,
  })) as MonitorTimelineSlot<T>[]

  // Match by interval index rather than by string formatting so offsets in
  // serialized timestamps cannot create duplicate Vue keys or lose a bucket.
  for (const entry of entries) {
    const index = Math.round((entry.timestamp - start) / intervalMs)
    if (index < 0 || index >= count) continue
    if (Math.abs(entry.timestamp - (start + index * intervalMs)) > intervalMs / 2) continue
    slots[index].bucket = entry.bucket
  }
  return slots
}
