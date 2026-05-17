<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.cpaImportTitle')"
    width="normal"
    close-on-click-outside
    @close="handleClose"
  >
    <form id="cpa-import-form" class="space-y-4" @submit.prevent="handleImport">
      <div class="text-sm text-gray-600 dark:text-dark-300">
        {{ t('admin.accounts.cpaImportHint') }}
      </div>
      <div
        class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs text-amber-700 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-300"
      >
        {{ t('admin.accounts.cpaImportWarning') }}
      </div>

      <div class="space-y-3">
        <label class="input-label">{{ t('admin.accounts.cpaImportFiles') }}</label>
        <div
          class="rounded-lg border border-dashed border-gray-300 bg-gray-50 px-4 py-3 dark:border-dark-600 dark:bg-dark-800"
        >
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div class="min-w-0">
              <div class="truncate text-sm text-gray-700 dark:text-dark-200">
                {{ fileSummary }}
              </div>
              <div class="text-xs text-gray-500 dark:text-dark-400">
                {{ t('admin.accounts.cpaImportSupportedFiles') }}
              </div>
            </div>
            <div class="flex flex-wrap gap-2">
              <button type="button" class="btn btn-secondary" @click="openFolderPicker">
                {{ t('admin.accounts.cpaImportChooseFolder') }}
              </button>
              <button type="button" class="btn btn-secondary" @click="openFilePicker">
                {{ t('admin.accounts.cpaImportChooseFiles') }}
              </button>
            </div>
          </div>
        </div>

        <input
          ref="folderInput"
          type="file"
          class="hidden"
          multiple
          webkitdirectory
          @change="handleFileChange"
        />
        <input
          ref="fileInput"
          type="file"
          class="hidden"
          multiple
          accept="application/json,.json,.yaml,.yml"
          @change="handleFileChange"
        />
      </div>

      <label class="flex items-start gap-3 rounded-lg border border-gray-200 px-3 py-3 text-sm dark:border-dark-700">
        <input v-model="skipDefaultGroupBind" type="checkbox" class="mt-0.5 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
        <span class="text-gray-700 dark:text-dark-200">
          {{ t('admin.accounts.cpaImportSkipDefaultGroupBind') }}
        </span>
      </label>

      <div
        v-if="result"
        class="space-y-3 rounded-xl border border-gray-200 p-4 dark:border-dark-700"
      >
        <div class="text-sm font-medium text-gray-900 dark:text-white">
          {{ t('admin.accounts.cpaImportResult') }}
        </div>

        <div class="grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
          <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-800">
            <div class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.accounts.cpaImportSeen') }}</div>
            <div class="mt-1 font-semibold text-gray-900 dark:text-white">{{ result.conversion.accounts_seen }}</div>
          </div>
          <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-800">
            <div class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.accounts.cpaImportConverted') }}</div>
            <div class="mt-1 font-semibold text-gray-900 dark:text-white">{{ result.import.account_created }}</div>
          </div>
          <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-800">
            <div class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.accounts.cpaImportSkipped') }}</div>
            <div class="mt-1 font-semibold text-gray-900 dark:text-white">{{ result.skipped_accounts.length }}</div>
          </div>
          <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-800">
            <div class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.accounts.cpaImportKeysPreserved') }}</div>
            <div class="mt-1 font-semibold text-gray-900 dark:text-white">{{ result.preserved_api_keys.length }}</div>
          </div>
        </div>

        <div class="text-sm text-gray-700 dark:text-dark-300">
          {{ t('admin.accounts.cpaImportResultSummary', summaryParams) }}
        </div>

        <div v-if="result.warnings.length" class="space-y-2">
          <div class="text-sm font-medium text-amber-600 dark:text-amber-400">
            {{ t('admin.accounts.cpaImportWarnings') }}
          </div>
          <div class="max-h-40 overflow-auto rounded-lg bg-gray-50 p-3 font-mono text-xs dark:bg-dark-800">
            <div v-for="(item, idx) in result.warnings" :key="`warning-${idx}`" class="whitespace-pre-wrap">
              {{ item }}
            </div>
          </div>
        </div>

        <div v-if="result.skipped_accounts.length" class="space-y-2">
          <div class="text-sm font-medium text-red-600 dark:text-red-400">
            {{ t('admin.accounts.cpaImportSkippedDetails') }}
          </div>
          <div class="max-h-48 overflow-auto rounded-lg bg-gray-50 p-3 font-mono text-xs dark:bg-dark-800">
            <div v-for="(item, idx) in result.skipped_accounts" :key="`skipped-${idx}`" class="whitespace-pre-wrap">
              {{ item.file_name }} - {{ item.reason }}
            </div>
          </div>
        </div>

        <div v-if="result.preserved_api_keys.length" class="space-y-2">
          <div class="text-sm font-medium text-blue-600 dark:text-blue-400">
            {{ t('admin.accounts.cpaImportPreservedKeys') }}
          </div>
          <div class="max-h-40 overflow-auto rounded-lg bg-gray-50 p-3 font-mono text-xs dark:bg-dark-800">
            <div v-for="(item, idx) in result.preserved_api_keys" :key="`key-${idx}`" class="whitespace-pre-wrap">
              {{ item.name }} - {{ item.source }}
            </div>
          </div>
        </div>
      </div>
    </form>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button class="btn btn-secondary" type="button" :disabled="importing" @click="handleClose">
          {{ t('common.cancel') }}
        </button>
        <button class="btn btn-primary" type="submit" form="cpa-import-form" :disabled="importing">
          {{ importing ? t('admin.accounts.cpaImporting') : t('admin.accounts.cpaImportButton') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import type { CpaImportResponse } from '@/types'

interface Props {
  show: boolean
}

interface Emits {
  (e: 'close'): void
  (e: 'imported'): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const { t } = useI18n()
const appStore = useAppStore()

const importing = ref(false)
const files = ref<File[]>([])
const result = ref<CpaImportResponse | null>(null)
const skipDefaultGroupBind = ref(true)

const folderInput = ref<HTMLInputElement | null>(null)
const fileInput = ref<HTMLInputElement | null>(null)

const fileSummary = computed(() => {
  if (files.value.length === 0) return t('admin.accounts.cpaImportSelectFiles')
  return t('admin.accounts.cpaImportSelectedFiles', { count: files.value.length })
})

const summaryParams = computed(() => ({
  account_created: result.value?.import.account_created ?? 0,
  account_failed: result.value?.import.account_failed ?? 0,
  proxy_created: result.value?.import.proxy_created ?? 0,
  proxy_reused: result.value?.import.proxy_reused ?? 0,
  proxy_failed: result.value?.import.proxy_failed ?? 0,
  accounts_skipped: result.value?.skipped_accounts.length ?? 0,
  api_keys_preserved: result.value?.preserved_api_keys.length ?? 0
}))

watch(
  () => props.show,
  (open) => {
    if (!open) return
    files.value = []
    result.value = null
    skipDefaultGroupBind.value = true
    if (folderInput.value) folderInput.value.value = ''
    if (fileInput.value) fileInput.value.value = ''
  }
)

const openFolderPicker = () => {
  folderInput.value?.click()
}

const openFilePicker = () => {
  fileInput.value?.click()
}

const handleFileChange = (event: Event) => {
  const target = event.target as HTMLInputElement
  files.value = Array.from(target.files || [])
}

const handleClose = () => {
  if (importing.value) return
  emit('close')
}

const handleImport = async () => {
  if (files.value.length === 0) {
    appStore.showError(t('admin.accounts.cpaImportSelectFiles'))
    return
  }

  importing.value = true
  try {
    const res = await adminAPI.accounts.importCpaData({
      files: files.value,
      skip_default_group_bind: skipDefaultGroupBind.value
    })
    result.value = res

    const hasErrors =
      res.import.account_failed > 0 ||
      res.import.proxy_failed > 0 ||
      res.skipped_accounts.length > 0

    if (hasErrors) {
      appStore.showError(t('admin.accounts.cpaImportCompletedWithErrors', summaryParams.value))
    } else {
      appStore.showSuccess(t('admin.accounts.cpaImportSuccess', summaryParams.value))
    }
    emit('imported')
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.accounts.cpaImportFailed'))
  } finally {
    importing.value = false
  }
}
</script>
