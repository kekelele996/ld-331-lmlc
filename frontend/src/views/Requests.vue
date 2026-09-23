<template>
  <div>
    <div class="page-title">
      <div>
        <h1>调班与替班申请</h1>
        <p>申请先由替班人确认，双方同意后再交主管审批；主管通过后系统原子性交换双方班次。</p>
      </div>
    </div>
    <el-card>
      <el-table :data="rows" v-loading="loading">
        <el-table-column prop="id" label="#" width="70" />
        <el-table-column label="申请人">
          <template #default="scope">{{ scope.row.applicant?.name }}</template>
        </el-table-column>
        <el-table-column label="替班人">
          <template #default="scope">{{ scope.row.substitute?.name }}</template>
        </el-table-column>
        <el-table-column prop="reason" label="调班原因" />
        <el-table-column label="当前状态" width="190">
          <template #default="scope">
            <el-tag :type="statusMeta(scope.row.status).type">{{ statusMeta(scope.row.status).label }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="当前等待" width="170">
          <template #default="scope">
            <span v-if="waitingFor(scope.row)">{{ waitingFor(scope.row) }}</span>
            <span v-else class="closed">流程已结束</span>
          </template>
        </el-table-column>
        <el-table-column v-if="auth.role !== 'staff' || canRespond" label="操作" width="200">
          <template #default="scope">
            <template v-if="scope.row.status === 'substitute_pending' && scope.row.substitute_id === auth.staffId">
              <el-button size="small" type="success" :loading="busy === scope.row.id" @click="respond(scope.row, true)">同意换班</el-button>
              <el-button size="small" type="danger" :loading="busy === scope.row.id" @click="respond(scope.row, false)">拒绝</el-button>
            </template>
            <template v-else-if="auth.role !== 'staff' && canSupervisorReview(scope.row.status)">
              <el-button size="small" type="success" :loading="busy === scope.row.id" @click="review(scope.row, true)">同意</el-button>
              <el-button size="small" type="danger" :loading="busy === scope.row.id" @click="review(scope.row, false)">驳回</el-button>
            </template>
            <span v-else class="no-action">无待办操作</span>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import api from '../api/http'
import { auth } from '../stores/auth'

const rows = ref<any[]>([])
const loading = ref(false)
const busy = ref(0)
const canRespond = ref(false)

const STATUS_META: Record<string, { label: string; type: '' | 'success' | 'warning' | 'danger' }> = {
  pending: { label: '待主管审批（历史申请）', type: 'warning' },
  substitute_pending: { label: '待替班人确认', type: 'warning' },
  supervisor_pending: { label: '替班人已同意，待主管审批', type: 'warning' },
  approved: { label: '已通过，班次已交换', type: 'success' },
  rejected: { label: '主管已驳回', type: 'danger' },
  substitute_rejected: { label: '替班人已拒绝', type: 'danger' }
}
const statusMeta = (s: string) => STATUS_META[s] || { label: s, type: '' as const }

function waitingFor(row: any): string {
  if (row.status === 'substitute_pending') return `替班人：${row.substitute?.name || ''}`
  if (row.status === 'supervisor_pending' || row.status === 'pending') return '主管审批'
  return ''
}
function canSupervisorReview(s: string) {
  return s === 'pending' || s === 'supervisor_pending'
}

async function load() {
  loading.value = true
  try {
    const data: any = await api.get('/shift-requests')
    rows.value = data
    canRespond.value = data.some((r: any) => r.status === 'substitute_pending' && r.substitute_id === auth.staffId)
  } finally {
    loading.value = false
  }
}
async function respond(row: any, approved: boolean) {
  busy.value = row.id
  try {
    await api.put(`/shift-requests/${row.id}/respond`, { approved })
    ElMessage.success(approved ? '已同意，等待主管审批' : '已拒绝，申请结束')
    await load()
  } catch (error: any) {
    ElMessage.error(error.message)
  } finally {
    busy.value = 0
  }
}
async function review(row: any, approved: boolean) {
  busy.value = row.id
  try {
    await api.put(`/shift-requests/${row.id}/review`, { approved })
    ElMessage.success('审批已完成')
    await load()
  } catch (error: any) {
    ElMessage.error(error.message)
  } finally {
    busy.value = 0
  }
}

onMounted(load)
</script>
<style scoped>
.closed,
.no-action {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}
</style>
