import { describe, expect, it } from 'vitest'
import { FLOOR_NAME_MAX, ROOM_NAME_MAX, truncate } from './RoomLabels'

describe('truncate', () => {
  it('leaves a name that fits alone', () => {
    expect(truncate('search_products', ROOM_NAME_MAX)).toBe('search_products')
    expect(truncate('x'.repeat(ROOM_NAME_MAX), ROOM_NAME_MAX)).toBe('x'.repeat(ROOM_NAME_MAX))
  })

  it('cuts a long tool name to 22 characters and an ellipsis', () => {
    const name = 'coinranking_prochart_set_viewport'
    expect(truncate(name, ROOM_NAME_MAX)).toBe('coinranking_prochart_s…')
    expect(truncate(name, ROOM_NAME_MAX)).toHaveLength(ROOM_NAME_MAX + 1)
  })

  it('gives a floor name the longer allowance', () => {
    const title = 'Bitcoin (BTC) Price Chart - Pro Drawing & TA | Coinranking'
    expect(truncate(title, FLOOR_NAME_MAX)).toBe('Bitcoin (BTC) Price Chart - Pro Drawing …')
    expect(truncate(title, FLOOR_NAME_MAX)).toHaveLength(FLOOR_NAME_MAX + 1)
  })
})
