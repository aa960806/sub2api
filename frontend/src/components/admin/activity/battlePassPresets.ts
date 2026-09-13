import type { BattlePassLevelInput, BattlePassRewardInput, BattlePassTaskInput } from '@/api/battlePass'

/**
 * Production-friendly 30-level season preset. It only returns child records;
 * callers still choose the season dates, price, and publish state.
 */
export interface BattlePassThirtyLevelPreset {
  levels: BattlePassLevelInput[]
  tasks: BattlePassTaskInput[]
  rewards: BattlePassRewardInput[]
}

export function createBattlePassThirtyLevelPreset(): BattlePassThirtyLevelPreset {
  const levels = Array.from({ length: 30 }, (_, index) => ({
    level: index + 1,
    // 80 EXP per level keeps a 30-day season achievable for regular users.
    required_exp: index * 80,
  }))

  const taskPresets: Array<Pick<BattlePassTaskInput, 'name' | 'description' | 'task_type' | 'period_type' | 'target_value' | 'exp_reward'>> = [
    { name: '每日完成 API 请求', description: '每天成功完成 10 次 API 请求', task_type: 'request_count', period_type: 'daily', target_value: 10, exp_reward: 30 },
    { name: '每日累计消费', description: '每天累计产生 1 余额的实际消费', task_type: 'cost_amount', period_type: 'daily', target_value: 1, exp_reward: 20 },
    { name: '每日生成图片', description: '每天成功生成 1 张图片', task_type: 'image_count', period_type: 'daily', target_value: 1, exp_reward: 15 },
    { name: '每日生成视频', description: '每天成功生成 1 个视频', task_type: 'video_count', period_type: 'daily', target_value: 1, exp_reward: 15 },
    { name: '赛季活跃', description: '赛季内至少活跃 10 天', task_type: 'active_days', period_type: 'season', target_value: 10, exp_reward: 120 },
    { name: '探索模型系列', description: '赛季内使用 3 个不同模型系列', task_type: 'distinct_model_families', period_type: 'season', target_value: 3, exp_reward: 120 },
    { name: '完成充值', description: '赛季内完成 1 笔有效余额充值', task_type: 'recharge_count', period_type: 'season', target_value: 1, exp_reward: 100 },
    { name: '累计充值金额', description: '赛季内累计有效充值 10 余额', task_type: 'recharge_amount', period_type: 'season', target_value: 10, exp_reward: 150 },
    { name: '完成有效邀请', description: '赛季内完成 2 位有效邀请用户', task_type: 'valid_invite_count', period_type: 'season', target_value: 2, exp_reward: 120 },
    { name: '受邀用户充值', description: '赛季内 1 位受邀用户完成有效充值', task_type: 'invitee_recharge_count', period_type: 'season', target_value: 1, exp_reward: 100 },
  ]
  const tasks = taskPresets.map((task, displayOrder) => ({
    ...task,
    filter_scope: 'all',
    filter_values: [],
    display_order: displayOrder,
    enabled: true,
  }))

  const rewards: BattlePassRewardInput[] = []
  const add = (level: number, track: 'free' | 'premium', reward_type: string, payload: Record<string, unknown>) => rewards.push({ level, track, reward_type, payload })
  // Keep one reward slot on every level and track (60 total), with modest
  // balance credits and milestone concurrency boosts. This makes the track
  // predictable while keeping the total premium value below the default 9.9
  // balance price. No subscription group is assumed by the preset.
  for (let level = 1; level <= 30; level += 1) {
    if (level % 10 === 0) add(level, 'free', 'concurrency', { amount: 1 })
    else add(level, 'free', 'balance', { amount: Number((0.03 + level * 0.002).toFixed(2)) })
    if (level % 10 === 0) add(level, 'premium', 'concurrency', { amount: 2 })
    else add(level, 'premium', 'balance', { amount: Number((0.08 + level * 0.004).toFixed(2)) })
  }

  return { levels, tasks, rewards }
}
