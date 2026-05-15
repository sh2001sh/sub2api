import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => {
        const messages: Record<string, string> = {
          'payment.perMonth': '月',
          'payment.perYear': '年',
          'payment.days': '天',
          'payment.subscribeNow': '立即开通',
          'payment.renewNow': '续费',
          'payment.planCard.rate': '倍率',
          'payment.planCard.quota': '额度',
          'payment.planCard.unlimited': '不限',
          'payment.planCard.models': '模型',
          'payment.planCard.dailyLimit': '日限额',
          'payment.planCard.weeklyLimit': '周限额',
          'payment.planCard.monthlyLimit': '月限额',
        }
        return messages[key] || key
      },
    }),
  }
})

import SubscriptionPlanCard from '../SubscriptionPlanCard.vue'

describe('SubscriptionPlanCard', () => {
  it('renders monthly validity for plural months units', () => {
    const wrapper = mount(SubscriptionPlanCard, {
      props: {
        plan: {
          id: 1,
          group_id: 1,
          group_platform: 'anthropic',
          name: 'Standard',
          description: '',
          price: 75,
          validity_days: 1,
          validity_unit: 'months',
          features: [],
          for_sale: true,
          sort_order: 10,
        },
      },
    })

    const text = wrapper.text()
    expect(text).toContain('月')
    expect(text).not.toContain('1天')
  })
})
