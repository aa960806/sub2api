<template>
  <section class="card space-y-6 p-5">
    <header class="flex flex-col gap-3 border-b border-gray-200 pb-5 dark:border-dark-700 sm:flex-row sm:items-start sm:justify-between">
      <div>
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('battlePass.adminTitle') }}</h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">先创建并保存草稿，再校验和发布赛季。已发布赛季只读，不能再修改奖励或任务。</p>
      </div>
      <label class="flex cursor-pointer items-center gap-2 text-sm font-medium text-gray-700 dark:text-dark-200">
        <input v-model="enabled" class="rounded border-gray-300 text-primary-600 focus:ring-primary-500" type="checkbox" @change="saveEnabled" />
        {{ t('battlePass.userSideSwitch') }}
      </label>
    </header>

    <p class="text-sm text-gray-500 dark:text-dark-400">
      {{ t(enabled ? 'battlePass.adminEnabledHint' : 'battlePass.adminDisabledHint') }}
    </p>

    <div class="grid gap-3 rounded-lg border border-primary-100 bg-primary-50/50 p-4 text-sm dark:border-primary-900/50 dark:bg-primary-900/10 md:grid-cols-3">
      <div><span class="font-semibold text-primary-700 dark:text-primary-300">1. 开启参与</span><p class="mt-1 text-xs text-gray-600 dark:text-dark-300">用户端开关必须开启。</p></div>
      <div><span class="font-semibold text-primary-700 dark:text-primary-300">2. 保存草稿</span><p class="mt-1 text-xs text-gray-600 dark:text-dark-300">填写必填项后点击“创建并保存草稿”。</p></div>
      <div><span class="font-semibold text-primary-700 dark:text-primary-300">3. 校验并发布</span><p class="mt-1 text-xs text-gray-600 dark:text-dark-300">发布后仍需开启用户访问；用户页以服务端开关为唯一准入依据。</p></div>
    </div>

    <div class="flex flex-wrap items-center justify-between gap-3">
      <div class="flex items-center gap-2 text-sm">
        <span class="font-medium text-gray-800 dark:text-dark-100">{{ selectedSeason ? `当前赛季：${selectedSeason.name}` : '正在新建赛季' }}</span>
        <span v-if="selectedSeason" class="badge" :class="seasonStatusClass(selectedSeason.status)">{{ seasonStatusLabel(selectedSeason) }}</span>
      </div>
      <button class="btn btn-secondary btn-sm" type="button" @click="startNewDraft">新建赛季</button>
    </div>

    <fieldset :disabled="!isDraftEditable" class="space-y-6 disabled:cursor-not-allowed disabled:opacity-65">
      <section class="space-y-3">
        <div>
          <h3 class="text-base font-semibold text-gray-900 dark:text-white">赛季基础信息</h3>
          <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">开始时间必须晚于当前时间；高级通行证价格必须大于 0。</p>
        </div>
        <div class="grid gap-3 md:grid-cols-2">
          <label><span class="input-label">赛季名称 <b class="text-red-500">*</b></span><input v-model="draft.name" aria-label="赛季名称" class="input" placeholder="例如：2026 金秋赛季" required /></label>
          <label><span class="input-label">赛季时区 <b class="text-red-500">*</b></span><input v-model="draft.timezone" aria-label="赛季时区" class="input" placeholder="Asia/Shanghai" required /></label>
          <label><span class="input-label">开始时间 <b class="text-red-500">*</b></span><input v-model="draft.start_at" aria-label="开始时间" class="input" type="datetime-local" required /></label>
          <label><span class="input-label">结束时间 <b class="text-red-500">*</b></span><input v-model="draft.end_at" aria-label="结束时间" class="input" type="datetime-local" required /></label>
          <label><span class="input-label">高级战令价格（余额） <b class="text-red-500">*</b></span><input v-model.number="draft.premium_price" aria-label="高级战令价格（余额）" class="input" min="0.01" step="0.01" type="number" required /></label>
          <div><span class="input-label">最高等级</span><div class="input flex items-center bg-gray-50 text-gray-600 dark:bg-dark-800 dark:text-dark-300">{{ draft.max_level }} 级<span class="ml-2 text-xs">由等级列表自动计算</span></div></div>
        </div>
        <label><span class="input-label">赛季说明</span><textarea v-model="draft.description" class="input min-h-[80px]" placeholder="向用户说明赛季玩法、奖励与注意事项" /></label>
      </section>

      <section class="space-y-3 border-t border-gray-200 pt-5 dark:border-dark-700">
        <div class="flex items-start justify-between gap-4">
          <div><h3 class="text-base font-semibold text-gray-900 dark:text-white">等级</h3><p class="mt-1 text-xs text-gray-500 dark:text-dark-400">经验为累计经验。等级从 1 开始连续编号，删除等级会同步调整后续等级与对应奖励。</p></div>
          <button class="btn btn-secondary btn-sm" type="button" title="新增等级" :disabled="!isDraftEditable" @click="addLevel"><Icon name="plus" size="sm" /><span class="ml-1">新增</span></button>
        </div>
        <div v-for="(level, index) in draft.levels" :key="`lv-${index}`" class="grid items-end gap-2 rounded-lg border border-gray-200 p-3 sm:grid-cols-[120px_minmax(0,1fr)_36px] dark:border-dark-700">
          <div><span class="input-label">等级</span><div class="input bg-gray-50 text-gray-600 dark:bg-dark-800 dark:text-dark-300">Lv. {{ level.level }}</div></div>
          <label><span class="input-label">达到该等级所需累计 EXP</span><input v-model.number="level.required_exp" class="input" min="0" step="1" type="number" /></label>
          <button class="btn-icon h-10 w-10 text-red-600 disabled:cursor-not-allowed disabled:opacity-40" type="button" :disabled="draft.levels.length <= 1" :title="draft.levels.length <= 1 ? '至少保留一个等级' : `删除等级 ${level.level}`" :aria-label="`删除等级 ${level.level}`" @click="removeLevel(index)"><Icon name="trash" size="sm" /></button>
        </div>
      </section>

      <section class="space-y-3 border-t border-gray-200 pt-5 dark:border-dark-700">
        <div class="flex items-start justify-between gap-4">
          <div><h3 class="text-base font-semibold text-gray-900 dark:text-white">任务</h3><p class="mt-1 text-xs text-gray-500 dark:text-dark-400">完成任务会自动获得 EXP；每日任务按赛季时区每日重置。</p></div>
          <div class="flex flex-wrap gap-2">
            <button class="btn btn-secondary btn-sm" type="button" title="填充完整验收配置" :disabled="!isDraftEditable" data-testid="battle-pass-fill-all-tasks" @click="fillAllTaskTypes"><Icon name="refresh" size="sm" /><span class="ml-1">全流程预设</span></button>
            <button class="btn btn-secondary btn-sm" type="button" title="新增任务" :disabled="!isDraftEditable" @click="addTask"><Icon name="plus" size="sm" /><span class="ml-1">新增</span></button>
          </div>
        </div>
        <article v-for="(task, index) in draft.tasks" :key="`task-${index}`" class="space-y-3 rounded-lg border border-gray-200 p-4 dark:border-dark-700">
          <div class="flex items-center justify-between gap-3"><h4 class="text-sm font-medium text-gray-800 dark:text-dark-100">任务 {{ index + 1 }}</h4><button class="btn-icon h-8 w-8 text-red-600 disabled:cursor-not-allowed disabled:opacity-40" type="button" :disabled="draft.tasks.length <= 1" :title="draft.tasks.length <= 1 ? '至少保留一个任务' : '删除任务'" :aria-label="`删除任务 ${index + 1}`" @click="removeTask(index)"><Icon name="trash" size="sm" /></button></div>
          <div class="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
            <label><span class="input-label">任务名称</span><input v-model="task.name" class="input" placeholder="例如：完成 5 次对话" /></label>
            <label><span class="input-label">任务类型</span><select v-model="task.task_type" class="input" data-testid="battle-pass-task-type" @change="normalizeTask(task)"><option v-for="option in taskTypeOptions" :key="option.value" :value="option.value">{{ option.label }}</option></select></label>
            <label><span class="input-label">统计周期</span><select v-model="task.period_type" class="input" @change="normalizeTask(task)"><option value="daily">每日重置</option><option value="season">整个赛季</option></select></label>
            <label><span class="input-label">完成目标</span><input v-model.number="task.target_value" class="input" min="1" step="1" type="number" /></label>
            <label><span class="input-label">完成奖励 EXP</span><input v-model.number="task.exp_reward" class="input" min="1" step="1" type="number" /></label>
            <label><span class="input-label">适用模型</span><select v-model="task.filter_scope" class="input" @change="normalizeTask(task)"><option value="all">全部模型</option><option value="model_family">指定模型系列</option><option value="exact_model">指定模型名称</option></select></label>
            <label class="flex items-end gap-2 pb-2 text-sm text-gray-700 dark:text-dark-200"><input v-model="task.enabled" type="checkbox" class="rounded border-gray-300 text-primary-600 focus:ring-primary-500" /> 启用该任务</label>
          </div>
          <label><span class="input-label">面向用户的任务说明（可选）</span><input v-model="task.description" class="input" placeholder="例如：每天任意模型完成 5 次成功请求" /></label>
          <label v-if="task.filter_scope !== 'all'"><span class="input-label">模型筛选值</span><input :value="task.filter_values.join(', ')" class="input" placeholder="多个值用英文逗号分隔，例如：gpt, claude" @input="setTaskFilters(task, ($event.target as HTMLInputElement).value)" /></label>
        </article>
      </section>

      <section class="space-y-3 border-t border-gray-200 pt-5 dark:border-dark-700">
        <div class="flex items-start justify-between gap-4">
          <div>
            <h3 class="text-base font-semibold text-gray-900 dark:text-white">等级奖励</h3>
            <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">每个等级的免费轨道和高级轨道各只能配置一项奖励。</p>
            <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">称号与徽章领取后会出现在用户战令的“已获得装扮”区域，二者可分别佩戴一项，仅作身份展示，不改变余额、并发或权限。</p>
          </div>
          <button class="btn btn-secondary btn-sm" type="button" title="新增奖励" :disabled="!isDraftEditable" @click="addReward"><Icon name="plus" size="sm" /><span class="ml-1">新增</span></button>
        </div>
        <article v-for="(reward, index) in draft.rewards" :key="`rw-${index}`" class="grid items-end gap-3 rounded-lg border border-gray-200 p-4 md:grid-cols-2 xl:grid-cols-[minmax(120px,0.7fr)_minmax(150px,0.9fr)_minmax(150px,1fr)_minmax(0,1.5fr)_36px] dark:border-dark-700">
          <label><span class="input-label">奖励等级</span><select v-model.number="reward.level" class="input"><option v-for="level in draft.levels" :key="level.level" :value="level.level">Lv. {{ level.level }}</option></select></label>
          <label><span class="input-label">奖励轨道</span><select v-model="reward.track" class="input"><option value="free">免费战令</option><option value="premium">高级战令</option></select></label>
          <label>
            <span class="input-label">奖励类型</span>
            <select v-model="reward.reward_type" class="input" data-testid="battle-pass-reward-type" @change="resetRewardPayload(reward)"><option value="balance">余额</option><option value="concurrency">并发额度</option><option value="title">称号（可佩戴文字）</option><option value="badge">徽章（可佩戴收藏）</option><option value="subscription_days">订阅时长</option></select>
          </label>
          <label v-if="reward.reward_type === 'balance'"><span class="input-label">余额金额</span><input :value="rewardNumber(reward, 'amount')" class="input" min="0.00000001" step="0.01" type="number" @input="setRewardNumber(reward, 'amount', ($event.target as HTMLInputElement).value)" /></label>
          <label v-else-if="reward.reward_type === 'concurrency'"><span class="input-label">增加并发数</span><input :value="rewardNumber(reward, 'amount')" class="input" min="1" step="1" type="number" @input="setRewardNumber(reward, 'amount', ($event.target as HTMLInputElement).value)" /></label>
          <div v-else-if="reward.reward_type === 'subscription_days'" class="grid grid-cols-2 gap-2">
            <label>
              <span class="input-label">订阅分组</span>
              <select :value="rewardNumber(reward, 'group_id')" class="input" data-testid="battle-pass-subscription-group" @change="setRewardNumber(reward, 'group_id', ($event.target as HTMLSelectElement).value)">
                <option v-if="!subscriptionGroups.length && !hasKnownSubscriptionGroup(reward)" :value="0" disabled>{{ subscriptionGroupsError || '暂无已启用的订阅分组' }}</option>
                <option v-if="hasUnknownSubscriptionGroup(reward)" :value="rewardNumber(reward, 'group_id')">已停用或已删除分组 (#{{ rewardNumber(reward, 'group_id') }})</option>
                <option v-for="group in subscriptionGroups" :key="group.id" :value="group.id">{{ group.name }} (#{{ group.id }}) · {{ group.platform }}</option>
              </select>
            </label>
            <label><span class="input-label">订阅天数</span><input :value="rewardNumber(reward, 'days')" class="input" min="1" step="1" type="number" @input="setRewardNumber(reward, 'days', ($event.target as HTMLInputElement).value)" /></label>
          </div>
          <div v-else class="grid grid-cols-2 gap-2"><label><span class="input-label">唯一代码</span><input :value="rewardText(reward, 'code')" class="input" placeholder="仅字母、数字、-、_" @input="setRewardText(reward, 'code', ($event.target as HTMLInputElement).value)" /></label><label><span class="input-label">展示名称</span><input :value="rewardText(reward, 'name')" class="input" placeholder="用户看到的名称" @input="setRewardText(reward, 'name', ($event.target as HTMLInputElement).value)" /></label></div>
          <button class="btn-icon h-10 w-10 text-red-600 disabled:cursor-not-allowed disabled:opacity-40" type="button" :disabled="!canRemoveReward(reward)" :title="canRemoveReward(reward) ? '删除奖励' : `至少保留一项${reward.track === 'free' ? '免费' : '高级'}奖励`" :aria-label="`删除奖励 ${index + 1}`" @click="removeReward(index)"><Icon name="trash" size="sm" /></button>
          <p class="text-xs text-gray-500 dark:text-dark-400 md:col-span-2 xl:col-span-5">{{ rewardTypeDescription(reward.reward_type) }}</p>
        </article>
      </section>
    </fieldset>

    <p v-if="!isDraftEditable" class="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:border-amber-800/60 dark:bg-amber-900/20 dark:text-amber-200">已发布的赛季使用不可变快照，内容不可再编辑。可新建赛季，或使用下方状态操作管理当前赛季。</p>

    <div class="flex flex-wrap gap-2 border-t border-gray-200 pt-5 dark:border-dark-700">
      <button data-testid="battle-pass-save-draft" class="btn btn-secondary" type="button" :disabled="!isDraftEditable || saving || !!actionPending" @click="saveDraft">{{ saving ? '保存中...' : selectedId ? '保存草稿' : '创建并保存草稿' }}</button>
      <button data-testid="battle-pass-validate-draft" class="btn btn-secondary" type="button" :disabled="!canValidateOrPublish || saving || !!actionPending" @click="runValidate">{{ actionPending === 'validate' ? '校验中...' : '校验草稿' }}</button>
      <button data-testid="battle-pass-publish-season" class="btn btn-primary" type="button" :disabled="!canValidateOrPublish || saving || !!actionPending" @click="runPublish">{{ actionPending === 'publish' ? '发布中...' : '发布赛季' }}</button>
      <button class="btn btn-secondary" type="button" :disabled="!canPause || !!actionPending" @click="runPause">暂停赛季</button>
      <button class="btn btn-secondary" type="button" :disabled="!canResume || !!actionPending" @click="runResume">恢复赛季</button>
      <button class="btn btn-secondary" type="button" :disabled="!canEnd || !!actionPending" @click="runEnd">结束赛季</button>
      <p class="w-full text-xs text-gray-500 dark:text-dark-400">{{ actionHint }}</p>
    </div>

    <section v-if="testToolsEnabled" data-testid="battle-pass-test-tools" class="space-y-4 border-t border-gray-200 pt-5 dark:border-dark-700">
      <div class="flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
        <div>
          <h3 class="text-base font-semibold text-gray-900 dark:text-white">本地流程验收</h3>
          <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">任务与经验可模拟；高级战令购买和奖励领取仍在用户端完成。</p>
        </div>
        <div class="flex flex-wrap items-end gap-2">
          <label class="w-36"><span class="input-label">验收用户 ID</span><input v-model.number="testUserId" class="input" min="1" step="1" type="number" /></label>
          <button class="btn btn-secondary btn-sm" type="button" :disabled="!selectedId || !!testPending" @click="loadTestState">刷新</button>
          <button data-testid="battle-pass-test-activate" class="btn btn-secondary btn-sm" type="button" :disabled="!canTestActivate || !!testPending" @click="activateTestSeason">立即开始</button>
          <a class="btn btn-primary btn-sm" href="/battle-pass" target="_blank" rel="noopener">打开用户战令</a>
        </div>
      </div>

      <div v-if="testState" class="space-y-3 rounded-lg border border-gray-200 p-4 dark:border-dark-700">
        <div class="flex flex-wrap items-center justify-between gap-3 text-sm">
          <div class="flex flex-wrap items-center gap-2">
            <span class="font-medium text-gray-900 dark:text-white">{{ testState.user.email }}</span>
            <span class="badge badge-gray">Lv. {{ testState.progress?.level || 1 }}</span>
            <span class="badge badge-gray">{{ testState.progress?.exp || 0 }} EXP</span>
          </div>
          <button data-testid="battle-pass-test-complete-all" class="btn btn-primary btn-sm" type="button" :disabled="testState.season.runtime_status !== 'active' || !!testPending || testState.tasks.every((task) => task.completed)" @click="completeTestTask(0)">完成全部任务</button>
        </div>
        <div class="grid gap-2 lg:grid-cols-2">
          <article v-for="task in testState.tasks" :key="task.id" class="flex items-center justify-between gap-3 rounded-lg border border-gray-200 px-3 py-2 dark:border-dark-700">
            <div class="min-w-0">
              <p class="truncate text-sm font-medium text-gray-800 dark:text-dark-100">{{ task.name }}</p>
              <p class="text-xs text-gray-500 dark:text-dark-400">{{ taskTypeLabel(task.task_type) }} · {{ formatTaskValue(task.current_value) }} / {{ formatTaskValue(task.target_value) }} · +{{ task.exp_reward }} EXP</p>
            </div>
            <span v-if="task.completed" class="badge badge-success shrink-0">已完成</span>
            <button v-else class="btn btn-secondary btn-sm shrink-0" type="button" :disabled="testState.season.runtime_status !== 'active' || !!testPending" @click="completeTestTask(task.id)">完成</button>
          </article>
        </div>
      </div>
      <p v-else class="rounded-lg border border-dashed border-gray-300 px-3 py-4 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-dark-400">选择已发布赛季后刷新验收状态。</p>
    </section>

    <p v-if="message" class="rounded-lg border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-700 dark:border-emerald-800/60 dark:bg-emerald-900/20 dark:text-emerald-200">{{ message }}</p>
    <p v-if="error" role="alert" class="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-800/60 dark:bg-red-900/20 dark:text-red-200">{{ error }}</p>

    <section v-if="seasons.length" class="space-y-2 border-t border-gray-200 pt-5 dark:border-dark-700">
      <h3 class="text-sm font-semibold text-gray-900 dark:text-white">赛季列表</h3>
      <button v-for="season in seasons" :key="season.id" class="flex w-full items-center justify-between gap-3 rounded-lg border px-3 py-2 text-left text-sm" :class="selectedId === season.id ? 'border-primary-500 bg-primary-50/40 dark:bg-primary-900/10' : 'border-gray-200 dark:border-dark-700'" type="button" @click="loadSeason(season.id)"><span class="min-w-0 truncate font-medium text-gray-800 dark:text-dark-100">{{ season.name }}</span><span class="shrink-0 badge" :class="seasonStatusClass(season.status)">{{ seasonStatusLabel(season) }}</span></button>
    </section>
    <TotpStepUpDialog :controller="stepUp" />
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import { useStepUp } from '@/composables/useStepUp'
import { extractApiErrorMessage } from '@/utils/apiError'
import { getAll as getAllGroups } from '@/api/admin/groups'
import { useAppStore } from '@/stores/app'
import type { AdminGroup } from '@/types'
import {
  activateBattlePassSeasonForTest, completeBattlePassTasksForTest, createBattlePassSeason, endBattlePassSeason,
  getBattlePassSeason, getBattlePassSettings, getBattlePassTestState, listBattlePassSeasons,
  pauseBattlePassSeason, publishBattlePassSeason, resumeBattlePassSeason, updateBattlePassSeason, updateBattlePassSettings,
  validateBattlePassSeason, type BattlePassRewardInput, type BattlePassSeason, type BattlePassSeasonDraft,
  type BattlePassTaskInput, type BattlePassTestState,
} from '@/api/battlePass'

const { t } = useI18n()
const stepUp = useStepUp()
const appStore = useAppStore()
const enabled = ref(false)
const persistedEnabled = ref(false)
const testToolsEnabled = ref(false)
const testUserId = ref(1)
const testPending = ref('')
const testState = ref<BattlePassTestState | null>(null)
const error = ref('')
const message = ref('')
const saving = ref(false)
const actionPending = ref<'' | 'validate' | 'publish' | 'pause' | 'resume' | 'end'>('')
const selectedId = ref<number | null>(null)
const seasons = ref<BattlePassSeason[]>([])
const subscriptionGroups = ref<AdminGroup[]>([])
const subscriptionGroupsError = ref('')
const draft = reactive<BattlePassSeasonDraft>(emptyDraft())
const taskTypeOptions = [
  { value: 'request_count', label: 'API 请求次数' }, { value: 'cost_amount', label: '消费金额' }, { value: 'active_days', label: '活跃天数' }, { value: 'distinct_model_families', label: '使用模型系列数' },
  { value: 'image_count', label: '图片生成次数' }, { value: 'video_count', label: '视频生成次数' }, { value: 'recharge_count', label: '充值次数' }, { value: 'recharge_amount', label: '充值金额' }, { value: 'valid_invite_count', label: '有效邀请人数' }, { value: 'invitee_recharge_count', label: '受邀用户充值数' },
]
const validationMessageMap: Record<string, string> = {
  'season name is required': '请填写赛季名称。',
  'start_at must not be in the past': '开始时间必须晚于当前时间。',
  'end_at must be after start_at': '结束时间必须晚于开始时间。',
  'timezone is invalid': '赛季时区无效，请填写例如 Asia/Shanghai。',
  'premium_price must be greater than 0': '高级战令价格必须大于 0。',
  'at least one level is required': '至少需要配置一个等级。',
  'required_exp must strictly increase': '各等级所需累计 EXP 必须严格递增。',
  'only one reward is allowed for each level and track': '同一等级的免费轨和高级轨各只能配置一项奖励。',
  'at least one free reward is required': '至少需要配置一项免费战令奖励。',
  'at least one premium reward is required': '至少需要配置一项高级战令奖励。',
  'at least one task is required': '至少需要配置一个任务。',
  'task name, target_value and exp_reward are required': '每个任务都必须填写名称，完成目标和奖励 EXP 必须大于 0。',
  'season time range overlaps another published season': '赛季时间与另一个已发布赛季重叠。',
  'only draft seasons can be fully edited': '只有草稿赛季可以修改。',
  'only draft seasons can be published': '只有草稿赛季可以发布。',
}
const selectedSeason = computed(() => seasons.value.find((season) => season.id === selectedId.value) ?? null)
const isDraftEditable = computed(() => !selectedSeason.value || selectedSeason.value.status === 'draft')
const canValidateOrPublish = computed(() => selectedSeason.value?.status === 'draft')
const canPause = computed(() => selectedSeason.value?.status === 'scheduled')
const canResume = computed(() => selectedSeason.value?.status === 'paused')
const canEnd = computed(() => selectedSeason.value?.status === 'scheduled' || selectedSeason.value?.status === 'paused')
const canTestActivate = computed(() => selectedSeason.value?.status === 'scheduled' && selectedSeason.value.runtime_status !== 'active')
const actionHint = computed(() => {
  if (saving.value) return '正在保存草稿，请稍候。'
  if (!selectedSeason.value) return '先填写带 * 的必填项并保存草稿；保存成功后“校验草稿”和“发布赛季”会自动启用。'
  if (selectedSeason.value.status === 'draft') return '草稿已保存。修改后请再次保存，再校验并发布。'
  if (selectedSeason.value.status === 'scheduled') return selectedSeason.value.runtime_status === 'active' ? '赛季正在进行，可暂停或结束。' : '赛季已发布，等待开始；可暂停或结束。'
  if (selectedSeason.value.status === 'paused') return '赛季已暂停，可恢复或结束。'
  return '该赛季已结束或归档，只能查看。'
})

function emptyDraft(): BattlePassSeasonDraft {
  const start = new Date(Date.now() + 3600_000); const end = new Date(Date.now() + 31 * 86400_000)
  return { name: '', description: '', timezone: 'Asia/Shanghai', start_at: toLocalInput(start), end_at: toLocalInput(end), premium_price: 9.9, max_level: 1, levels: [{ level: 1, required_exp: 0 }], tasks: [{ name: '', description: '', task_type: 'request_count', period_type: 'daily', target_value: 1, exp_reward: 10, filter_scope: 'all', filter_values: [], display_order: 0, enabled: true }], rewards: [{ level: 1, track: 'free', reward_type: 'balance', payload: { amount: 0.2 } }, { level: 1, track: 'premium', reward_type: 'balance', payload: { amount: 1 } }] }
}
function toLocalInput(value: Date) { const offset = value.getTimezoneOffset(); return new Date(value.getTime() - offset * 60000).toISOString().slice(0, 16) }
function toRFC3339(value: string) { return new Date(value).toISOString() }
function localizeValidationMessage(value: string) { return validationMessageMap[value.trim()] || value }
function battlePassErrorMessage(err: unknown, fallback: string) { return localizeValidationMessage(extractApiErrorMessage(err, fallback)) }
function startNewDraft() { selectedId.value = null; testState.value = null; Object.assign(draft, emptyDraft()); error.value = ''; message.value = '' }
function draftSaveIssue() {
  if (!draft.name.trim()) return '请填写赛季名称。'
  if (!draft.timezone.trim()) return '请填写赛季时区，例如 Asia/Shanghai。'
  const start = new Date(draft.start_at).getTime()
  const end = new Date(draft.end_at).getTime()
  if (!Number.isFinite(start)) return '请选择有效的开始时间。'
  if (!Number.isFinite(end)) return '请选择有效的结束时间。'
  if (start <= Date.now()) return '开始时间必须晚于当前时间。'
  if (end <= start) return '结束时间必须晚于开始时间。'
  if (!Number.isFinite(Number(draft.premium_price)) || Number(draft.premium_price) <= 0) return '高级战令价格必须大于 0。'
  return ''
}
function payload(): BattlePassSeasonDraft { return { ...draft, start_at: toRFC3339(draft.start_at), end_at: toRFC3339(draft.end_at), max_level: draft.levels.length, levels: draft.levels, tasks: draft.tasks.map((task, index) => ({ ...task, display_order: index, enabled: task.enabled !== false })), rewards: draft.rewards } }
function syncLevels() { draft.levels.forEach((level, index) => { level.level = index + 1 }); draft.max_level = draft.levels.length }
function addLevel() { const last = draft.levels.at(-1); draft.levels.push({ level: draft.levels.length + 1, required_exp: (last?.required_exp ?? 0) + 100 }); syncLevels() }
function removeLevel(index: number) { if (draft.levels.length <= 1) return; const removedLevel = draft.levels[index].level; draft.levels.splice(index, 1); draft.rewards = draft.rewards.filter((reward) => reward.level !== removedLevel).map((reward) => ({ ...reward, level: reward.level > removedLevel ? reward.level - 1 : reward.level })); syncLevels(); message.value = `已删除 Lv. ${removedLevel}，并同步移除该等级的奖励。` }
function addTask() { draft.tasks.push({ name: '', description: '', task_type: 'request_count', period_type: 'daily', target_value: 1, exp_reward: 10, filter_scope: 'all', filter_values: [], display_order: draft.tasks.length, enabled: true }) }
function fillAllTaskTypes() {
  const presets: Array<Pick<BattlePassTaskInput, 'name' | 'description' | 'task_type' | 'period_type' | 'target_value'>> = [
    { name: '完成 API 请求', description: '成功完成 2 次 API 请求', task_type: 'request_count', period_type: 'daily', target_value: 2 },
    { name: '累计消费', description: '累计产生 1 余额的实际消费', task_type: 'cost_amount', period_type: 'daily', target_value: 1 },
    { name: '保持活跃', description: '在赛季内活跃 1 天', task_type: 'active_days', period_type: 'season', target_value: 1 },
    { name: '探索模型系列', description: '使用 2 个不同模型系列', task_type: 'distinct_model_families', period_type: 'season', target_value: 2 },
    { name: '生成图片', description: '成功生成 1 张图片', task_type: 'image_count', period_type: 'daily', target_value: 1 },
    { name: '生成视频', description: '成功生成 1 个视频', task_type: 'video_count', period_type: 'daily', target_value: 1 },
    { name: '完成充值', description: '赛季内完成 1 笔有效余额充值', task_type: 'recharge_count', period_type: 'season', target_value: 1 },
    { name: '累计充值金额', description: '赛季内累计有效充值 1 余额', task_type: 'recharge_amount', period_type: 'season', target_value: 1 },
    { name: '完成有效邀请', description: '赛季内完成 1 位有效邀请用户', task_type: 'valid_invite_count', period_type: 'season', target_value: 1 },
    { name: '受邀用户充值', description: '赛季内 1 位受邀用户完成有效充值', task_type: 'invitee_recharge_count', period_type: 'season', target_value: 1 },
  ]
  draft.tasks = presets.map((preset, index) => ({ ...preset, exp_reward: 25, filter_scope: 'all', filter_values: [], display_order: index, enabled: true }))
  draft.levels = [{ level: 1, required_exp: 0 }, { level: 2, required_exp: 100 }, { level: 3, required_exp: 200 }]
  draft.rewards = [
    { level: 1, track: 'free', reward_type: 'balance', payload: { amount: 0.01 } },
    { level: 1, track: 'premium', reward_type: 'balance', payload: { amount: 0.02 } },
    { level: 2, track: 'free', reward_type: 'concurrency', payload: { amount: 1 } },
    { level: 2, track: 'premium', reward_type: 'badge', payload: { code: 'battle-pass-test-badge', name: '验收徽章' } },
    { level: 3, track: 'free', reward_type: 'title', payload: { code: 'battle-pass-test-title', name: '验收先锋' } },
    { level: 3, track: 'premium', reward_type: 'balance', payload: { amount: 0.03 } },
  ]
  syncLevels()
  message.value = '已填充 3 个等级、全部 10 类任务和 6 项双轨奖励；每项任务奖励 25 EXP。'
}
function removeTask(index: number) { if (draft.tasks.length > 1) draft.tasks.splice(index, 1) }
function addReward() { draft.rewards.push({ level: 1, track: draft.rewards.some((reward) => reward.track === 'free') ? 'premium' : 'free', reward_type: 'balance', payload: { amount: 0.1 } }) }
function canRemoveReward(reward: BattlePassRewardInput) { return draft.rewards.filter((item) => item.track === reward.track).length > 1 }
function removeReward(index: number) { if (canRemoveReward(draft.rewards[index])) draft.rewards.splice(index, 1) }
function normalizeTask(task: BattlePassTaskInput) { if (['active_days', 'distinct_model_families', 'recharge_count', 'recharge_amount', 'valid_invite_count', 'invitee_recharge_count'].includes(task.task_type)) task.period_type = 'season'; if (['recharge_count', 'recharge_amount', 'valid_invite_count', 'invitee_recharge_count'].includes(task.task_type)) { task.filter_scope = 'all'; task.filter_values = [] } if (task.filter_scope === 'all') task.filter_values = [] }
function setTaskFilters(task: BattlePassTaskInput, raw: string) { task.filter_values = raw.split(',').map((value) => value.trim()).filter(Boolean) }
function taskTypeLabel(taskType: string) { return taskTypeOptions.find((option) => option.value === taskType)?.label || taskType }
function formatTaskValue(value: number) { return new Intl.NumberFormat(undefined, { maximumFractionDigits: 4 }).format(value) }
function rewardNumber(reward: BattlePassRewardInput, key: string) { return Number(reward.payload?.[key] || 0) }
function setRewardNumber(reward: BattlePassRewardInput, key: string, raw: string) { reward.payload = { ...reward.payload, [key]: Number(raw) } }
function rewardText(reward: BattlePassRewardInput, key: string) { return String(reward.payload?.[key] || '') }
function setRewardText(reward: BattlePassRewardInput, key: string, value: string) { reward.payload = { ...reward.payload, [key]: value } }
function resetRewardPayload(reward: BattlePassRewardInput) { if (reward.reward_type === 'balance') reward.payload = { amount: 0.1 }; else if (reward.reward_type === 'concurrency') reward.payload = { amount: 1 }; else if (reward.reward_type === 'subscription_days') reward.payload = { group_id: subscriptionGroups.value[0]?.id || 0, days: 1 }; else reward.payload = { code: '', name: '' } }
function hasKnownSubscriptionGroup(reward: BattlePassRewardInput) { const groupID = rewardNumber(reward, 'group_id'); return groupID > 0 && subscriptionGroups.value.some((group) => group.id === groupID) }
function hasUnknownSubscriptionGroup(reward: BattlePassRewardInput) { return rewardNumber(reward, 'group_id') > 0 && !hasKnownSubscriptionGroup(reward) }
function rewardTypeDescription(type: string) {
  if (type === 'balance') return '直接增加用户可用余额，可继续用于模型调用或购买高级战令。'
  if (type === 'concurrency') return '永久提高用户并发额度；达到系统上限时只发放到上限。'
  if (type === 'subscription_days') return '为所选订阅类型分组新增或顺延有效期；普通计费分组不会出现在列表中。'
  if (type === 'title') return '解锁一个文字称号，可在用户战令的装扮区佩戴；同一时间可佩戴一个称号。'
  if (type === 'badge') return '解锁一个赛季徽章，可在用户战令的装扮区佩戴；同一时间可佩戴一个徽章。'
  return ''
}
function seasonStatusLabel(season: BattlePassSeason) { return ({ draft: '草稿', scheduled: season.runtime_status === 'active' ? '进行中' : '已发布待开始', paused: '已暂停', ended: '已结束', archived: '已归档' } as Record<string, string>)[season.status] || season.status }
function seasonStatusClass(status: string) { return ({ draft: 'badge-gray', scheduled: 'badge-success', paused: 'badge-warning', ended: 'badge-gray', archived: 'badge-gray' } as Record<string, string>)[status] || 'badge-gray' }
function syncPublicFlag(value: boolean) {
  if (appStore.cachedPublicSettings) {
    appStore.cachedPublicSettings = { ...appStore.cachedPublicSettings, battle_pass_enabled: value }
  }
  try { window.dispatchEvent(new CustomEvent('battle-pass-config-changed')) } catch { /* non-browser test runtime */ }
}
onMounted(() => { void reload() })
async function loadSubscriptionGroups() {
  subscriptionGroupsError.value = ''
  try {
    const groups = await getAllGroups()
    subscriptionGroups.value = groups.filter((group) => group.status === 'active' && group.subscription_type === 'subscription')
  } catch {
    subscriptionGroups.value = []
    subscriptionGroupsError.value = '订阅分组加载失败，请刷新后重试'
  }
}
async function reload() {
  try {
    const [settings, seasonList] = await Promise.all([getBattlePassSettings(), listBattlePassSeasons(), loadSubscriptionGroups()])
    const confirmedEnabled = settings.enabled === true
    enabled.value = confirmedEnabled
    persistedEnabled.value = confirmedEnabled
    syncPublicFlag(confirmedEnabled)
    testToolsEnabled.value = settings.test_tools_enabled === true
    seasons.value = seasonList
  } catch (err) {
    enabled.value = false
    persistedEnabled.value = false
    testToolsEnabled.value = false
    seasons.value = []
    syncPublicFlag(false)
    error.value = battlePassErrorMessage(err, '加载战令配置失败。')
  }
}
async function saveEnabled() {
  error.value = ''
  const desired = enabled.value === true
  try {
    const saved = await stepUp.run(() => updateBattlePassSettings({ enabled: desired }))
    const confirmedEnabled = saved.enabled === true
    enabled.value = confirmedEnabled
    persistedEnabled.value = confirmedEnabled
    syncPublicFlag(confirmedEnabled)
  } catch (err) {
    enabled.value = persistedEnabled.value
    error.value = battlePassErrorMessage(err, '保存参与开关失败。')
  }
}
async function saveDraft() {
  if (saving.value) return
  error.value = ''; message.value = ''
  const issue = draftSaveIssue()
  if (issue) { error.value = issue; return }
  saving.value = true
  try {
    const saved = selectedId.value ? await stepUp.run(() => updateBattlePassSeason(selectedId.value!, payload())) : await stepUp.run(() => createBattlePassSeason(payload()))
    selectedId.value = saved.id
    await reload()
    message.value = '草稿已保存。现在可以校验草稿，校验通过后再发布赛季。'
  } catch (err) { error.value = battlePassErrorMessage(err, '保存草稿失败。') } finally { saving.value = false }
}
async function loadSeason(id: number) { error.value = ''; message.value = ''; testState.value = null; try { const detail = await getBattlePassSeason(id); selectedId.value = detail.id; Object.assign(draft, { name: detail.name, description: detail.description, timezone: detail.timezone, start_at: toLocalInput(new Date(detail.start_at)), end_at: toLocalInput(new Date(detail.end_at)), premium_price: detail.premium_price, max_level: detail.max_level, levels: detail.levels || [], tasks: detail.tasks || [], rewards: detail.rewards || [] }); syncLevels(); if (testToolsEnabled.value && detail.status !== 'draft' && detail.status !== 'archived') await loadTestState() } catch (err) { error.value = battlePassErrorMessage(err, '加载赛季失败。') } }
async function loadTestState() {
  if (!selectedId.value || testUserId.value <= 0 || testPending.value) return
  testPending.value = 'load'; error.value = ''
  try { testState.value = await getBattlePassTestState(selectedId.value, testUserId.value) }
  catch (err) { testState.value = null; error.value = battlePassErrorMessage(err, '加载验收状态失败。') }
  finally { testPending.value = '' }
}
async function activateTestSeason() {
  if (!selectedId.value || testPending.value) return
  testPending.value = 'activate'; error.value = ''
  try { await stepUp.run(() => activateBattlePassSeasonForTest(selectedId.value!)); await reload(); testPending.value = ''; await loadTestState(); message.value = '验收赛季已立即开始。' }
  catch (err) { error.value = battlePassErrorMessage(err, '立即开始验收赛季失败。') }
  finally { testPending.value = '' }
}
async function completeTestTask(taskId: number) {
  if (!selectedId.value || testUserId.value <= 0 || testPending.value) return
  testPending.value = taskId > 0 ? `task-${taskId}` : 'all'; error.value = ''
  try { const result = await stepUp.run(() => completeBattlePassTasksForTest(selectedId.value!, testUserId.value, taskId)); testState.value = result.state; message.value = result.completed_count > 0 ? `已完成 ${result.completed_count} 项验收任务。` : '所选任务已经完成。' }
  catch (err) { error.value = battlePassErrorMessage(err, '完成验收任务失败。') }
  finally { testPending.value = '' }
}
async function runValidate() {
  if (!selectedId.value || actionPending.value) return
  error.value = ''; message.value = ''; actionPending.value = 'validate'
  try {
    const result = await stepUp.run(() => validateBattlePassSeason(selectedId.value!))
    if (result.ok) message.value = '校验通过，可以发布赛季。'
    else error.value = result.errors.map((item) => localizeValidationMessage(item.message)).join('；') || '草稿校验未通过。'
  } catch (err) { error.value = battlePassErrorMessage(err, '校验草稿失败。') } finally { actionPending.value = '' }
}
async function runMutation(kind: 'publish' | 'pause' | 'resume' | 'end', action: () => Promise<unknown>, success: string) {
  if (!selectedId.value || actionPending.value) return
  error.value = ''; message.value = ''; actionPending.value = kind
  try { await stepUp.run(action); await reload(); message.value = success } catch (err) { error.value = battlePassErrorMessage(err, `${success.replace('已', '')}失败。`) } finally { actionPending.value = '' }
}
async function runPublish() { if (selectedId.value) await runMutation('publish', () => publishBattlePassSeason(selectedId.value!), '赛季已发布。') }
async function runPause() { if (selectedId.value) await runMutation('pause', () => pauseBattlePassSeason(selectedId.value!), '赛季已暂停。') }
async function runResume() { if (selectedId.value) await runMutation('resume', () => resumeBattlePassSeason(selectedId.value!), '赛季已恢复。') }
async function runEnd() { if (selectedId.value) await runMutation('end', () => endBattlePassSeason(selectedId.value!), '赛季已结束。') }
</script>
