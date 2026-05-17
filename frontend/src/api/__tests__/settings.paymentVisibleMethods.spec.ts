import { describe, expect, it } from 'vitest'

import {
  getPaymentVisibleMethodSourceOptions,
  normalizePaymentVisibleMethodSource,
} from '@/api/admin/settings'

describe('admin settings payment visible method helpers', () => {
  it('normalizes aliases into canonical source keys per visible method', () => {
    expect(normalizePaymentVisibleMethodSource('alipay', 'official')).toBe('official_alipay')
    expect(normalizePaymentVisibleMethodSource('alipay', 'alipay_direct')).toBe('official_alipay')
    expect(normalizePaymentVisibleMethodSource('alipay', 'easypay')).toBe('easypay_alipay')
    expect(normalizePaymentVisibleMethodSource('alipay', 'xunhupay')).toBe('xunhupay_alipay')

    expect(normalizePaymentVisibleMethodSource('wxpay', 'official')).toBe('official_wxpay')
    expect(normalizePaymentVisibleMethodSource('wxpay', 'wechat')).toBe('official_wxpay')
    expect(normalizePaymentVisibleMethodSource('wxpay', 'easypay')).toBe('easypay_wxpay')
    expect(normalizePaymentVisibleMethodSource('wxpay', 'xunhupay')).toBe('xunhupay_wxpay')
  })

  it('rejects unknown or cross-method source values', () => {
    expect(normalizePaymentVisibleMethodSource('alipay', 'official_wxpay')).toBe('')
    expect(normalizePaymentVisibleMethodSource('wxpay', 'official_alipay')).toBe('')
    expect(normalizePaymentVisibleMethodSource('alipay', 'unknown')).toBe('')
    expect(normalizePaymentVisibleMethodSource('wxpay', null)).toBe('')
  })

  it('includes xunhupay options in the method-scoped source lists', () => {
    const alipay = getPaymentVisibleMethodSourceOptions('alipay')
    const wxpay = getPaymentVisibleMethodSourceOptions('wxpay')

    expect(alipay.map((item) => item.value)).toEqual([
      '',
      'official_alipay',
      'easypay_alipay',
      'xunhupay_alipay',
    ])
    expect(wxpay.map((item) => item.value)).toEqual([
      '',
      'official_wxpay',
      'easypay_wxpay',
      'xunhupay_wxpay',
    ])
    expect(alipay.at(-1)).toMatchObject({
      value: 'xunhupay_alipay',
      labelEn: 'XunhuPay Alipay',
    })
    expect(wxpay.at(-1)).toMatchObject({
      value: 'xunhupay_wxpay',
      labelEn: 'XunhuPay WeChat Pay',
    })
  })
})
