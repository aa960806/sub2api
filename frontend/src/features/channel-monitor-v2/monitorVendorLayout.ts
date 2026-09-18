import type { MonitorConfig, MonitorMatrixRow } from '@/api/channelMonitorV2'

export interface MonitorVendorSection {
  platform: string
  rows: MonitorMatrixRow[]
}

/** Arrange only server-authorized matrix rows; display order never grants access. */
export function buildMonitorVendorSections(
  rows: MonitorMatrixRow[],
  platforms: MonitorConfig['platforms'] = [],
): MonitorVendorSection[] {
  const sections = new Map<string, MonitorVendorSection>()
  for (const row of rows) {
    if (row.group_id == null || row.group_id <= 0) continue
    let section = sections.get(row.platform)
    if (!section) {
      section = { platform: row.platform, rows: [] }
      sections.set(row.platform, section)
    }
    section.rows.push(row)
  }

  const vendorOrder = new Map(platforms.map((platform, index) => [platform.platform, index]))
  const groupOrders = new Map(platforms.map(platform => [
    platform.platform,
    new Map((platform.group_order ?? []).map((id, index) => [id, index])),
  ]))
  const rank = (value?: number) => value ?? Number.MAX_SAFE_INTEGER
  for (const section of sections.values()) {
    const order = groupOrders.get(section.platform)
    section.rows.sort((a, b) =>
      rank(order?.get(a.group_id!)) - rank(order?.get(b.group_id!))
      || a.group_id! - b.group_id!,
    )
  }
  return [...sections.values()].sort((a, b) =>
    rank(vendorOrder.get(a.platform)) - rank(vendorOrder.get(b.platform))
    || a.platform.localeCompare(b.platform),
  )
}
