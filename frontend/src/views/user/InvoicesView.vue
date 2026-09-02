<template>
  <AppLayout>
    <div class="mx-auto max-w-[1440px] space-y-5">
      <header class="flex flex-col gap-4 border-b border-gray-200 px-5 pb-5 sm:flex-row sm:items-center sm:justify-between dark:border-dark-700">
        <div>
          <h1 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('invoice.user.title') }}</h1>
          <p class="mt-0.5 text-sm text-gray-500 dark:text-dark-400">{{ t('invoice.user.description') }}</p>
        </div>
        <div class="inline-flex w-fit rounded-lg border border-gray-200 bg-gray-50 p-1 dark:border-dark-700 dark:bg-dark-900">
          <button v-if="config.enabled" type="button" class="px-4 py-2 text-sm font-medium" :class="tab === 'apply' ? activeTabClass : inactiveTabClass" @click="tab = 'apply'">
            {{ t('invoice.user.applyTab') }}
          </button>
          <button type="button" class="px-4 py-2 text-sm font-medium" :class="tab === 'history' ? activeTabClass : inactiveTabClass" @click="tab = 'history'">
            {{ t('invoice.user.historyTab') }}
          </button>
        </div>
      </header>

      <div v-if="loadingConfig" class="card px-5 py-14 text-center text-sm text-gray-500 dark:text-dark-400">{{ t('common.loading') }}</div>
      <div v-else-if="!config.enabled && !config.has_history" class="card px-5 py-14 text-center">
        <Icon name="document" size="xl" class="mx-auto text-gray-400" />
        <p class="mt-3 font-medium text-gray-800 dark:text-dark-100">{{ t('invoice.user.disabled') }}</p>
      </div>
      <div v-else-if="!config.enabled" class="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-200">
        {{ t('invoice.user.disabledHistory') }}
      </div>

      <template v-if="!loadingConfig && tab === 'apply' && config.enabled">
        <ol class="grid grid-cols-3 border-y border-gray-200 py-3 dark:border-dark-700" :aria-label="t('invoice.user.progressLabel')">
          <li v-for="(step, index) in applicationSteps" :key="step" class="flex min-w-0 items-center gap-2 px-2 sm:px-4" :class="index < 2 ? 'border-r border-gray-200 dark:border-dark-700' : ''">
            <span class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-xs font-semibold" :class="stepComplete[index] ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/50 dark:text-emerald-300' : 'bg-gray-100 text-gray-500 dark:bg-dark-800 dark:text-dark-300'">
              <Icon v-if="stepComplete[index]" name="check" size="xs" :stroke-width="2.25" />
              <span v-else>{{ index + 1 }}</span>
            </span>
            <span class="truncate text-xs font-medium text-gray-700 sm:text-sm dark:text-dark-200">{{ step }}</span>
          </li>
        </ol>

        <section class="card overflow-hidden">
          <div class="flex flex-col gap-3 border-b border-gray-200 px-5 py-4 sm:flex-row sm:items-center sm:justify-between dark:border-dark-700">
            <div class="flex items-start gap-3">
              <span class="mt-0.5 text-xs font-semibold text-primary-600 dark:text-primary-400">01</span>
              <div>
                <h2 class="font-semibold text-gray-900 dark:text-white">
                  {{ t('invoice.user.selectOrders') }} <span class="text-red-500" aria-hidden="true">*</span>
                </h2>
                <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('invoice.user.orderLimit', { count: config.max_orders_per_request }) }}</p>
              </div>
            </div>
            <div class="flex items-center gap-2">
              <span v-if="selectedOrders.size" class="hidden text-xs font-medium text-emerald-600 sm:inline dark:text-emerald-400">{{ t('invoice.user.selectedOrders', { count: selectedOrders.size }) }}</span>
              <input v-model="orderKeyword" class="input min-w-0 sm:w-64" :placeholder="t('invoice.user.searchOrder')" @keyup.enter="loadOrders" />
              <button type="button" class="btn btn-secondary px-3" :disabled="loadingOrders" :title="t('invoice.user.searchOrder')" @click="loadOrders">
                <Icon name="search" size="sm" />
              </button>
            </div>
          </div>
          <div class="overflow-x-auto">
            <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
              <thead class="bg-gray-50 dark:bg-dark-800/80">
                <tr>
                  <th class="w-12 px-4 py-3"><span class="sr-only">{{ t('common.select') }}</span></th>
                  <th class="table-th">{{ t('invoice.fields.orderNo') }}</th>
                  <th class="table-th">{{ t('invoice.fields.orderType') }}</th>
                  <th class="table-th">{{ t('invoice.fields.paidAt') }}</th>
                  <th class="table-th table-th-right">{{ t('invoice.fields.amount') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
                <tr v-if="loadingOrders"><td colspan="5" class="px-5 py-8 text-center text-sm text-gray-500">{{ t('common.loading') }}</td></tr>
                <tr v-else-if="eligibleOrders.length === 0">
                  <td colspan="5" class="px-5 py-8 text-center">
                    <Icon name="inbox" size="lg" class="mx-auto text-gray-300 dark:text-dark-500" />
                    <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">{{ t('invoice.user.noEligibleOrders') }}</p>
                  </td>
                </tr>
                <tr v-for="order in eligibleOrders" :key="order.payment_order_id" class="cursor-pointer hover:bg-gray-50 dark:hover:bg-dark-800/60" @click="toggleOrder(order)">
                  <td class="px-4 py-3">
                    <input :checked="selectedOrders.has(order.payment_order_id)" type="checkbox" class="rounded border-gray-300 text-primary-600 focus:ring-primary-500" :disabled="!selectedOrders.has(order.payment_order_id) && selectedOrders.size >= config.max_orders_per_request" @click.stop @change="toggleOrder(order)" />
                  </td>
                  <td class="table-td font-mono text-xs">{{ order.out_trade_no }}</td>
                  <td class="table-td">{{ orderTypeLabel(order.order_type) }}</td>
                  <td class="table-td">{{ formatDateTime(order.completed_at || order.paid_at) }}</td>
                  <td class="table-td table-td-right font-medium">¥{{ order.pay_amount }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <div v-if="eligibleTotal > orderPageSize" class="border-t border-gray-200 px-5 py-3 dark:border-dark-700">
            <Pagination :page="orderPage" :total="eligibleTotal" :page-size="orderPageSize" @update:page="changeOrderPage" @update:pageSize="changeOrderPageSize" />
          </div>
        </section>

        <section class="grid items-start gap-5 xl:grid-cols-[minmax(0,1fr)_320px]">
          <form id="invoice-request-form" class="card overflow-hidden" @submit.prevent="openSubmitConfirm">
            <div class="flex flex-col gap-4 border-b border-gray-200 px-5 py-4 sm:flex-row sm:items-center sm:justify-between dark:border-dark-700">
              <div class="flex items-start gap-3">
                <span class="mt-0.5 text-xs font-semibold text-primary-600 dark:text-primary-400">02</span>
                <div>
                  <h2 class="font-semibold text-gray-900 dark:text-white">{{ t('invoice.user.headerInfo') }}</h2>
                  <p class="mt-1 text-xs text-gray-500 dark:text-dark-400"><span class="text-red-500">*</span> {{ t('invoice.user.requiredHint') }}</p>
                </div>
              </div>
              <div class="inline-flex w-fit rounded-lg bg-gray-100 p-1 dark:bg-dark-900">
                <button type="button" class="px-3 py-1.5 text-sm" :class="form.title_type === 'PERSONAL' ? activeTabClass : inactiveTabClass" @click="form.title_type = 'PERSONAL'">{{ t('invoice.titleType.personal') }}</button>
                <button type="button" class="px-3 py-1.5 text-sm" :class="form.title_type === 'COMPANY' ? activeTabClass : inactiveTabClass" @click="form.title_type = 'COMPANY'">{{ t('invoice.titleType.company') }}</button>
              </div>
            </div>
            <div class="space-y-6 p-5">
              <div class="grid gap-4 md:grid-cols-2">
                <label class="block">
                  <span class="input-label">{{ t('invoice.fields.titleName') }} <span class="text-red-500" aria-hidden="true">*</span></span>
                  <input v-model="form.title_name" class="input" :class="form.title_name.length > 0 && !form.title_name.trim() ? 'input-error' : ''" maxlength="200" required aria-required="true" autocomplete="organization" />
                </label>
                <label v-if="form.title_type === 'COMPANY'" class="block">
                  <span class="input-label">{{ t('invoice.fields.taxpayerId') }} <span class="text-red-500" aria-hidden="true">*</span></span>
                  <input v-model="form.taxpayer_id" class="input uppercase" :class="form.taxpayer_id.length > 0 && !taxpayerIdValid(form.taxpayer_id) ? 'input-error' : ''" maxlength="20" minlength="15" pattern="[0-9A-Za-z]{15,20}" required aria-required="true" autocapitalize="characters" spellcheck="false" />
                  <p v-if="form.taxpayer_id.length > 0 && !taxpayerIdValid(form.taxpayer_id)" class="input-error-text">{{ t('invoice.user.invalidTaxpayerId') }}</p>
                </label>
                <label class="block">
                  <span class="input-label">{{ t('invoice.fields.recipientEmail') }} <span class="text-red-500" aria-hidden="true">*</span></span>
                  <input v-model="form.recipient_email" class="input" :class="form.recipient_email.length > 0 && !emailValid(form.recipient_email) ? 'input-error' : ''" type="email" maxlength="255" required aria-required="true" autocomplete="email" />
                  <p v-if="form.recipient_email.length > 0 && !emailValid(form.recipient_email)" class="input-error-text">{{ t('invoice.user.invalidEmail') }}</p>
                </label>
                <label class="block">
                  <span class="input-label">{{ t('invoice.fields.recipientPhone') }} <span class="ml-1 font-normal text-gray-400">{{ t('common.optional') }}</span></span>
                  <input v-model="form.recipient_phone" class="input" maxlength="32" autocomplete="tel" />
                </label>
              </div>

              <div v-if="form.title_type === 'COMPANY'" class="border-t border-gray-200 pt-5 dark:border-dark-700">
                <div class="mb-4">
                  <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('invoice.user.companyDetails') }}</h3>
                  <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('invoice.user.companyDetailsHint') }}</p>
                </div>
                <div class="grid gap-4 md:grid-cols-2">
                  <label class="block"><span class="input-label">{{ t('invoice.fields.companyAddress') }} <span class="ml-1 font-normal text-gray-400">{{ t('common.optional') }}</span></span><input v-model="form.company_address" class="input" maxlength="255" autocomplete="street-address" /></label>
                  <label class="block"><span class="input-label">{{ t('invoice.fields.companyPhone') }} <span class="ml-1 font-normal text-gray-400">{{ t('common.optional') }}</span></span><input v-model="form.company_phone" class="input" maxlength="32" autocomplete="tel" /></label>
                  <label class="block"><span class="input-label">{{ t('invoice.fields.bankName') }} <span class="ml-1 font-normal text-gray-400">{{ t('common.optional') }}</span></span><input v-model="form.bank_name" class="input" maxlength="100" /></label>
                  <label class="block"><span class="input-label">{{ t('invoice.fields.bankAccount') }} <span class="ml-1 font-normal text-gray-400">{{ t('common.optional') }}</span></span><input v-model="form.bank_account" class="input" maxlength="64" inputmode="numeric" /></label>
                </div>
              </div>

              <label class="block">
                <span class="input-label">{{ t('invoice.fields.userNote') }} <span class="ml-1 font-normal text-gray-400">{{ t('common.optional') }}</span></span>
                <textarea v-model="form.user_note" class="input min-h-20 resize-y" maxlength="500" />
              </label>
            </div>
          </form>

          <aside class="card h-fit overflow-hidden xl:sticky xl:top-5">
            <div class="border-b border-gray-200 px-5 py-4 dark:border-dark-700">
              <div class="flex items-center gap-3">
                <span class="text-xs font-semibold text-primary-600 dark:text-primary-400">03</span>
                <h2 class="font-semibold text-gray-900 dark:text-white">{{ t('invoice.user.summary') }}</h2>
              </div>
            </div>
            <div class="p-5">
            <dl class="mt-4 space-y-3 text-sm">
              <div class="flex justify-between"><dt class="text-gray-500 dark:text-dark-400">{{ t('invoice.fields.orderCount') }}</dt><dd class="font-medium">{{ selectedOrders.size }}</dd></div>
              <div class="flex justify-between"><dt class="text-gray-500 dark:text-dark-400">{{ t('invoice.fields.itemName') }}</dt><dd class="max-w-44 text-right font-medium">{{ config.item_name }}</dd></div>
              <div class="flex items-end justify-between border-t border-gray-200 pt-4 dark:border-dark-700"><dt class="text-gray-500 dark:text-dark-400">{{ t('invoice.fields.amount') }}</dt><dd class="text-2xl font-semibold text-gray-900 dark:text-white">¥{{ selectedAmount }}</dd></div>
            </dl>
              <p class="mt-3 text-xs" :class="amountValid && selectedOrders.size > 0 ? 'text-emerald-600 dark:text-emerald-400' : 'text-amber-700 dark:text-amber-300'">{{ amountConstraintText }}</p>

              <ul class="mt-5 space-y-2 border-t border-gray-200 pt-4 text-xs dark:border-dark-700">
                <li v-for="item in readinessItems" :key="item.label" class="flex items-center gap-2" :class="item.ready ? 'text-emerald-700 dark:text-emerald-300' : 'text-gray-500 dark:text-dark-400'">
                  <Icon :name="item.ready ? 'checkCircle' : 'xCircle'" size="sm" />
                  <span>{{ item.label }}</span>
                </li>
              </ul>

              <button type="submit" form="invoice-request-form" class="btn btn-primary mt-5 w-full" :disabled="!canSubmit || submitting" :aria-disabled="!canSubmit || submitting">
                {{ submitting ? t('common.processing') : t('invoice.user.submit') }}
              </button>
              <p v-if="!canSubmit" class="mt-2 text-center text-xs text-gray-500 dark:text-dark-400">{{ t('invoice.user.completeRequired') }}</p>
            </div>
          </aside>
        </section>
      </template>

      <section v-if="!loadingConfig && tab === 'history'" class="card overflow-hidden">
        <div class="flex items-center justify-between border-b border-gray-200 px-5 py-4 dark:border-dark-700">
          <h2 class="font-semibold text-gray-900 dark:text-white">{{ t('invoice.user.historyTab') }}</h2>
          <button class="btn btn-secondary" :disabled="loadingHistory" :title="t('common.refresh')" @click="loadHistory"><Icon name="refresh" size="sm" :class="loadingHistory ? 'animate-spin' : ''" /></button>
        </div>
        <div class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
            <thead class="bg-gray-50 dark:bg-dark-800/80"><tr><th class="table-th">{{ t('invoice.fields.requestNo') }}</th><th class="table-th">{{ t('invoice.fields.titleName') }}</th><th class="table-th">{{ t('invoice.fields.status') }}</th><th class="table-th table-th-right">{{ t('invoice.fields.amount') }}</th><th class="table-th">{{ t('invoice.fields.createdAt') }}</th><th class="table-th table-th-right">{{ t('common.actions') }}</th></tr></thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
              <tr v-if="loadingHistory"><td colspan="6" class="px-5 py-12 text-center text-sm text-gray-500">{{ t('common.loading') }}</td></tr>
              <tr v-else-if="history.length === 0"><td colspan="6" class="px-5 py-12 text-center text-sm text-gray-500">{{ t('invoice.user.noHistory') }}</td></tr>
              <tr v-for="request in history" :key="request.id" class="hover:bg-gray-50 dark:hover:bg-dark-800/60">
                <td class="table-td font-mono text-xs">{{ request.request_no }}</td><td class="table-td">{{ request.title_name }}</td>
                <td class="table-td"><span class="badge" :class="statusClass(request.status)">{{ statusLabel(request.status) }}</span></td>
                <td class="table-td table-td-right font-medium">¥{{ request.total_amount }}</td><td class="table-td">{{ formatDateTime(request.created_at) }}</td>
                <td class="table-td table-td-right"><button class="btn-icon" :title="t('common.view')" @click="openDetail(request.id)"><Icon name="eye" size="sm" /></button></td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-if="historyTotal > historyPageSize" class="border-t border-gray-200 px-5 py-3 dark:border-dark-700"><Pagination :page="historyPage" :total="historyTotal" :page-size="historyPageSize" @update:page="changeHistoryPage" @update:pageSize="changeHistoryPageSize" /></div>
      </section>
    </div>

    <ConfirmDialog :show="showSubmitConfirm" :title="t('invoice.user.confirmTitle')" :message="t('invoice.user.confirmMessage', { count: selectedOrders.size, amount: selectedAmount, title: form.title_name })" @confirm="submitRequest" @cancel="showSubmitConfirm = false" />
    <ConfirmDialog :show="!!cancelTarget" :title="t('invoice.user.cancelTitle')" :message="t('invoice.user.cancelMessage')" danger @confirm="cancelRequest" @cancel="cancelTarget = null" />

    <BaseDialog :show="!!detail" :title="detail?.request_no || t('invoice.user.detail')" width="wide" @close="closeDetail">
      <div v-if="detail" class="space-y-5">
        <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <div><p class="text-xs text-gray-500">{{ t('invoice.fields.status') }}</p><span class="badge mt-1" :class="statusClass(detail.status)">{{ statusLabel(detail.status) }}</span></div>
          <div><p class="text-xs text-gray-500">{{ t('invoice.fields.amount') }}</p><p class="mt-1 font-semibold">¥{{ detail.total_amount }}</p></div>
          <div><p class="text-xs text-gray-500">{{ t('invoice.fields.orderCount') }}</p><p class="mt-1 font-semibold">{{ detail.order_count }}</p></div>
          <div><p class="text-xs text-gray-500">{{ t('invoice.fields.invoiceDate') }}</p><p class="mt-1 font-semibold">{{ formatDate(detail.invoice_date) }}</p></div>
        </div>
        <div v-if="detail.reject_reason" class="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-200">{{ detail.reject_reason }}</div>
        <dl class="grid gap-x-6 gap-y-3 text-sm sm:grid-cols-2">
          <div><dt class="text-gray-500">{{ t('invoice.fields.titleName') }}</dt><dd class="mt-1 text-gray-900 dark:text-white">{{ detail.title_name }}</dd></div>
          <div><dt class="text-gray-500">{{ t('invoice.fields.recipientEmail') }}</dt><dd class="mt-1 text-gray-900 dark:text-white">{{ detail.recipient_email }}</dd></div>
          <div v-if="detail.taxpayer_id"><dt class="text-gray-500">{{ t('invoice.fields.taxpayerId') }}</dt><dd class="mt-1 font-mono text-gray-900 dark:text-white">{{ detail.taxpayer_id }}</dd></div>
          <div><dt class="text-gray-500">{{ t('invoice.fields.itemName') }}</dt><dd class="mt-1 text-gray-900 dark:text-white">{{ detail.invoice_item_name }}</dd></div>
        </dl>
        <div v-if="detail.orders?.length" class="overflow-x-auto"><table class="min-w-full"><thead><tr><th class="table-th">{{ t('invoice.fields.orderNo') }}</th><th class="table-th">{{ t('invoice.fields.orderType') }}</th><th class="table-th table-th-right">{{ t('invoice.fields.amount') }}</th></tr></thead><tbody><tr v-for="order in detail.orders" :key="order.id"><td class="table-td font-mono text-xs">{{ order.out_trade_no }}</td><td class="table-td">{{ orderTypeLabel(order.order_type) }}</td><td class="table-td table-td-right">¥{{ order.pay_amount }}</td></tr></tbody></table></div>
      </div>
      <template #footer><div class="flex flex-wrap justify-end gap-2"><button v-if="detail?.status === 'ISSUED' || detail?.status === 'VOIDED'" class="btn btn-secondary" @click="downloadInvoice(detail)"><Icon name="download" size="sm" />{{ t('invoice.actions.download') }}</button><button v-if="detail?.status === 'REJECTED' && config.enabled" class="btn btn-primary" @click="startResubmit(detail)">{{ t('invoice.actions.resubmit') }}</button><button v-if="config.enabled && (detail?.status === 'PENDING' || detail?.status === 'REJECTED')" class="btn btn-danger" @click="cancelTarget = detail">{{ t('invoice.actions.cancel') }}</button></div></template>
    </BaseDialog>

    <BaseDialog :show="!!resubmitTarget" :title="t('invoice.actions.resubmit')" width="wide" @close="resubmitTarget = null">
      <form id="invoice-resubmit-form" class="grid gap-4 sm:grid-cols-2" @submit.prevent="resubmitRequest">
        <p class="sm:col-span-2 text-xs text-gray-500 dark:text-dark-400"><span class="text-red-500">*</span> {{ t('invoice.user.requiredHint') }}</p>
        <div class="sm:col-span-2 inline-flex w-fit rounded-lg bg-gray-100 p-1 dark:bg-dark-800">
          <button type="button" class="px-3 py-1.5 text-sm" :class="resubmitForm.title_type === 'PERSONAL' ? activeTabClass : inactiveTabClass" @click="resubmitForm.title_type = 'PERSONAL'">{{ t('invoice.titleType.personal') }}</button>
          <button type="button" class="px-3 py-1.5 text-sm" :class="resubmitForm.title_type === 'COMPANY' ? activeTabClass : inactiveTabClass" @click="resubmitForm.title_type = 'COMPANY'">{{ t('invoice.titleType.company') }}</button>
        </div>
        <label class="block"><span class="input-label">{{ t('invoice.fields.titleName') }} <span class="text-red-500" aria-hidden="true">*</span></span><input v-model="resubmitForm.title_name" class="input" maxlength="200" required aria-required="true" /></label>
        <label v-if="resubmitForm.title_type === 'COMPANY'" class="block"><span class="input-label">{{ t('invoice.fields.taxpayerId') }} <span class="text-red-500" aria-hidden="true">*</span></span><input v-model="resubmitForm.taxpayer_id" class="input uppercase" maxlength="20" minlength="15" pattern="[0-9A-Za-z]{15,20}" required aria-required="true" autocapitalize="characters" spellcheck="false" /></label>
        <label class="block"><span class="input-label">{{ t('invoice.fields.recipientEmail') }} <span class="text-red-500" aria-hidden="true">*</span></span><input v-model="resubmitForm.recipient_email" class="input" type="email" maxlength="255" required aria-required="true" autocomplete="email" /></label>
        <label class="block"><span class="input-label">{{ t('invoice.fields.recipientPhone') }} <span class="ml-1 font-normal text-gray-400">{{ t('common.optional') }}</span></span><input v-model="resubmitForm.recipient_phone" class="input" maxlength="32" autocomplete="tel" /></label>
        <template v-if="resubmitForm.title_type === 'COMPANY'">
          <label class="block"><span class="input-label">{{ t('invoice.fields.companyAddress') }} <span class="ml-1 font-normal text-gray-400">{{ t('common.optional') }}</span></span><input v-model="resubmitForm.company_address" class="input" maxlength="255" /></label>
          <label class="block"><span class="input-label">{{ t('invoice.fields.companyPhone') }} <span class="ml-1 font-normal text-gray-400">{{ t('common.optional') }}</span></span><input v-model="resubmitForm.company_phone" class="input" maxlength="32" /></label>
          <label class="block"><span class="input-label">{{ t('invoice.fields.bankName') }} <span class="ml-1 font-normal text-gray-400">{{ t('common.optional') }}</span></span><input v-model="resubmitForm.bank_name" class="input" maxlength="100" /></label>
          <label class="block"><span class="input-label">{{ t('invoice.fields.bankAccount') }} <span class="ml-1 font-normal text-gray-400">{{ t('common.optional') }}</span></span><input v-model="resubmitForm.bank_account" class="input" maxlength="64" /></label>
        </template>
        <label class="block sm:col-span-2"><span class="input-label">{{ t('invoice.fields.userNote') }} <span class="ml-1 font-normal text-gray-400">{{ t('common.optional') }}</span></span><textarea v-model="resubmitForm.user_note" class="input min-h-20 resize-y" maxlength="500" /></label>
      </form>
      <template #footer><button type="submit" form="invoice-resubmit-form" class="btn btn-primary" :disabled="actionLoading || !headerValid(resubmitForm)" :aria-disabled="actionLoading || !headerValid(resubmitForm)">{{ t('invoice.actions.resubmit') }}</button></template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores'
import { createInvoiceIdempotencyKey, invoicesAPI } from '@/api/invoices'
import type { InvoiceHeaderInput, InvoiceOrderSnapshot, InvoicePublicConfig, InvoiceRequest, InvoiceStatus } from '@/types/invoice'

const { t } = useI18n()
const appStore = useAppStore()
const defaultConfig: InvoicePublicConfig = { enabled: false, has_history: false, min_amount: 0.01, max_amount: 0, application_days: 0, max_orders_per_request: 50, item_name: '', admin_notification_emails: [], max_file_size_mb: 0, allow_reapply_after_void: false, allowed_order_types: [] }
const emptyHeader = (): InvoiceHeaderInput => ({ title_type: 'PERSONAL', title_name: '', taxpayer_id: '', recipient_email: '', recipient_phone: '', company_address: '', company_phone: '', bank_name: '', bank_account: '', user_note: '' })
const config = reactive<InvoicePublicConfig>({ ...defaultConfig })
const form = reactive<InvoiceHeaderInput>(emptyHeader())
const resubmitForm = reactive<InvoiceHeaderInput>(emptyHeader())
const loadingConfig = ref(true), loadingOrders = ref(false), loadingHistory = ref(false), submitting = ref(false), actionLoading = ref(false)
const tab = ref<'apply' | 'history'>('apply')
const eligibleOrders = ref<InvoiceOrderSnapshot[]>([]), eligibleTotal = ref(0), orderPage = ref(1), orderPageSize = ref(20), orderKeyword = ref('')
const selectedOrders = reactive(new Map<number, InvoiceOrderSnapshot>())
const history = ref<InvoiceRequest[]>([]), historyTotal = ref(0), historyPage = ref(1), historyPageSize = ref(20)
const detail = ref<InvoiceRequest | null>(null), cancelTarget = ref<InvoiceRequest | null>(null), resubmitTarget = ref<InvoiceRequest | null>(null), showSubmitConfirm = ref(false)
let submitKey = '', actionKey = ''
const activeTabClass = 'rounded-md bg-primary-600 text-white shadow-sm'
const inactiveTabClass = 'text-gray-600 hover:text-gray-900 dark:text-dark-300 dark:hover:text-white'
const selectedAmountCents = computed(() => [...selectedOrders.values()].reduce((sum, order) => sum + moneyToCents(order.pay_amount), 0))
const selectedAmount = computed(() => centsToMoney(selectedAmountCents.value))
const amountValid = computed(() => selectedAmountCents.value >= moneyToCents(String(config.min_amount)) && (config.max_amount === 0 || selectedAmountCents.value <= moneyToCents(String(config.max_amount))))
const amountConstraintText = computed(() => config.max_amount > 0 ? t('invoice.user.amountRange', { min: config.min_amount.toFixed(2), max: config.max_amount.toFixed(2) }) : t('invoice.user.amountMinimum', { min: config.min_amount.toFixed(2) }))
const canSubmit = computed(() => config.enabled && selectedOrders.size > 0 && selectedOrders.size <= config.max_orders_per_request && amountValid.value && headerValid(form))
const applicationSteps = computed(() => [t('invoice.user.stepOrders'), t('invoice.user.stepDetails'), t('invoice.user.stepConfirm')])
const stepComplete = computed(() => [selectedOrders.size > 0 && amountValid.value, headerValid(form), canSubmit.value])
const readinessItems = computed(() => [
  { label: t('invoice.user.ordersReady'), ready: selectedOrders.size > 0 },
  { label: t('invoice.user.amountReady'), ready: selectedOrders.size > 0 && amountValid.value },
  { label: t('invoice.user.detailsReady'), ready: headerValid(form) }
])

async function loadConfig() { try { Object.assign(config, await invoicesAPI.getConfig()); if (!config.enabled) tab.value = 'history' } catch (error: any) { appStore.showError(error?.message || t('common.error')); tab.value = 'history' } finally { loadingConfig.value = false } }
async function loadOrders() { if (!config.enabled) return; loadingOrders.value = true; try { const data = await invoicesAPI.getEligibleOrders({ page: orderPage.value, page_size: orderPageSize.value, keyword: orderKeyword.value.trim() || undefined }); eligibleOrders.value = data.items || []; eligibleTotal.value = data.total || 0 } catch (error: any) { appStore.showError(error?.message || t('common.error')) } finally { loadingOrders.value = false } }
async function loadHistory() { loadingHistory.value = true; try { const data = await invoicesAPI.listMy({ page: historyPage.value, page_size: historyPageSize.value }); history.value = data.items || []; historyTotal.value = data.total || 0 } catch (error: any) { appStore.showError(error?.message || t('common.error')) } finally { loadingHistory.value = false } }
function toggleOrder(order: InvoiceOrderSnapshot) {
  if (selectedOrders.has(order.payment_order_id)) {
    selectedOrders.delete(order.payment_order_id)
    return
  }
  if (selectedOrders.size < config.max_orders_per_request) selectedOrders.set(order.payment_order_id, order)
}
function openSubmitConfirm() { if (config.enabled && canSubmit.value) showSubmitConfirm.value = true }
async function submitRequest() { if (!config.enabled || !canSubmit.value) { showSubmitConfirm.value = false; return } submitting.value = true; showSubmitConfirm.value = false; submitKey ||= createInvoiceIdempotencyKey(); try { await invoicesAPI.create([...selectedOrders.keys()], normalizedHeader(form), submitKey); submitKey = ''; selectedOrders.clear(); Object.assign(form, emptyHeader()); appStore.showSuccess(t('invoice.user.submitted')); tab.value = 'history'; await Promise.all([loadHistory(), loadOrders()]); window.dispatchEvent(new CustomEvent('invoice-config-changed')) } catch (error: any) { appStore.showError(error?.message || t('common.error')) } finally { submitting.value = false } }
async function openDetail(id: number) { try { detail.value = await invoicesAPI.getMy(id) } catch (error: any) { appStore.showError(error?.message || t('common.error')) } }
function closeDetail() { detail.value = null }
async function cancelRequest() { if (!config.enabled || !cancelTarget.value) { cancelTarget.value = null; return } actionLoading.value = true; actionKey ||= createInvoiceIdempotencyKey(); try { await invoicesAPI.cancel(cancelTarget.value.id, actionKey); actionKey = ''; cancelTarget.value = null; detail.value = null; appStore.showSuccess(t('invoice.user.cancelled')); await Promise.all([loadHistory(), config.enabled ? loadOrders() : Promise.resolve()]) } catch (error: any) { appStore.showError(error?.message || t('common.error')) } finally { actionLoading.value = false } }
function startResubmit(request: InvoiceRequest) { if (!config.enabled) return; resubmitTarget.value = request; Object.assign(resubmitForm, headerFromRequest(request)); detail.value = null }
async function resubmitRequest() { if (!config.enabled || !resubmitTarget.value || !headerValid(resubmitForm)) { if (!config.enabled) resubmitTarget.value = null; return } actionLoading.value = true; actionKey ||= createInvoiceIdempotencyKey(); try { await invoicesAPI.resubmit(resubmitTarget.value.id, normalizedHeader(resubmitForm), actionKey); actionKey = ''; resubmitTarget.value = null; appStore.showSuccess(t('invoice.user.resubmitted')); await loadHistory() } catch (error: any) { appStore.showError(error?.message || t('common.error')) } finally { actionLoading.value = false } }
async function downloadInvoice(request: InvoiceRequest) { try { const response = await invoicesAPI.download(request.id); saveBlob(response.data, request.current_file?.original_filename || `${request.request_no}.pdf`) } catch (error: any) { appStore.showError(error?.message || t('common.error')) } }
function changeOrderPage(page: number) { orderPage.value = page; void loadOrders() }
function changeOrderPageSize(size: number) { orderPageSize.value = size; orderPage.value = 1; void loadOrders() }
function changeHistoryPage(page: number) { historyPage.value = page; void loadHistory() }
function changeHistoryPageSize(size: number) { historyPageSize.value = size; historyPage.value = 1; void loadHistory() }
function emailValid(value: string) { return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value.trim()) }
function taxpayerIdValid(value: string) { return /^[0-9A-Z]{15,20}$/.test(value.trim().toUpperCase()) }
function headerValid(value: InvoiceHeaderInput) { const taxOK = value.title_type === 'PERSONAL' || taxpayerIdValid(value.taxpayer_id); return !!value.title_name.trim() && emailValid(value.recipient_email) && taxOK }
function normalizedHeader(value: InvoiceHeaderInput): InvoiceHeaderInput { const result = { ...value, title_name: value.title_name.trim(), taxpayer_id: value.taxpayer_id.trim().toUpperCase(), recipient_email: value.recipient_email.trim().toLowerCase() }; if (result.title_type === 'PERSONAL') Object.assign(result, { taxpayer_id: '', company_address: '', company_phone: '', bank_name: '', bank_account: '' }); return result }
function headerFromRequest(value: InvoiceRequest): InvoiceHeaderInput { return { title_type: value.title_type, title_name: value.title_name, taxpayer_id: value.taxpayer_id, recipient_email: value.recipient_email, recipient_phone: value.recipient_phone, company_address: value.company_address, company_phone: value.company_phone, bank_name: value.bank_name, bank_account: value.bank_account, user_note: value.user_note } }
function moneyToCents(value: string) { const normalized = String(value || '0').trim(); const match = normalized.match(/^(-?\d+)(?:\.(\d{0,2}))?$/); if (!match) return 0; return Number(match[1]) * 100 + Number((match[2] || '').padEnd(2, '0')) * (normalized.startsWith('-') ? -1 : 1) }
function centsToMoney(value: number) { return `${Math.floor(value / 100)}.${String(Math.abs(value % 100)).padStart(2, '0')}` }
function formatDateTime(value?: string) { return value ? new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) : '-' }
function formatDate(value?: string) { return value ? new Intl.DateTimeFormat(undefined, { dateStyle: 'medium' }).format(new Date(value)) : '-' }
function orderTypeLabel(value: string) { return t(`invoice.orderType.${value}`, value) }
function statusLabel(value: InvoiceStatus) { return t(`invoice.status.${value}`) }
function statusClass(value: InvoiceStatus) { return ({ PENDING: 'badge-warning', PROCESSING: 'badge-info', REJECTED: 'badge-danger', CANCELLED: 'badge-gray', ISSUED: 'badge-success', VOIDED: 'badge-gray' } as Record<InvoiceStatus, string>)[value] }
function saveBlob(blob: Blob, filename: string) { const url = URL.createObjectURL(blob); const link = document.createElement('a'); link.href = url; link.download = filename; link.click(); URL.revokeObjectURL(url) }
onMounted(async () => { await loadConfig(); await loadHistory(); if (config.enabled) await loadOrders() })
</script>

<style scoped>
.table-th {
  @apply whitespace-nowrap px-5 py-3 text-left text-xs font-semibold text-gray-500 dark:text-dark-400;
}

.table-th-right {
  @apply text-right;
}

.table-td {
  @apply whitespace-nowrap px-5 py-3 text-sm text-gray-600 dark:text-dark-300;
}

.table-td-right {
  @apply text-right tabular-nums;
}
</style>
