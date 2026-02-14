<template>
  <div>
    <div class="section-header">
      <h2>{{ tx("Todo 看板", "Todo Board") }}</h2>
      <button class="btn btn-outline" @click="load">{{ tx("刷新", "Refresh") }}</button>
    </div>

    <div class="panel">
      <div class="panel-body">
        <div class="grid-4">
          <div class="form-group"><label>ID</label><input v-model="createForm.id" placeholder="todo_custom_1" /></div>
          <div class="form-group"><label>{{ tx("标题", "Title") }}</label><input v-model="createForm.title" :placeholder="tx('执行反思轮次', 'Run reflection pass')" /></div>
          <div class="form-group">
            <label>{{ tx("状态", "Status") }}</label>
            <select v-model="createForm.status">
              <option value="pending">pending</option>
              <option value="in_progress">in_progress</option>
              <option value="completed">completed</option>
              <option value="blocked">blocked</option>
              <option value="canceled">canceled</option>
            </select>
          </div>
          <div class="form-group"><label>{{ tx("计划 ID", "Plan ID") }}</label><input v-model="createForm.planID" placeholder="plan_xxx" /></div>
          <div class="form-group"><label>{{ tx("会话 ID", "Session ID") }}</label><input v-model="createForm.sessionID" placeholder="sess_xxx" /></div>
          <div class="form-group"><label>{{ tx("运行 ID", "Run ID") }}</label><input v-model="createForm.runID" placeholder="run_xxx" /></div>
          <div class="form-group" style="grid-column: span 2"><label>{{ tx("描述", "Description") }}</label><input v-model="createForm.description" :placeholder="tx('todo 描述', 'todo description')" /></div>
        </div>
        <div class="btn-row">
          <button class="btn" @click="create">{{ tx("创建 Todo", "Create Todo") }}</button>
        </div>
      </div>
    </div>

    <div class="panel">
      <div class="panel-body">
        <div class="grid-4">
          <div class="form-group"><label>{{ tx("状态筛选", "Filter status") }}</label><input v-model="filter.status" placeholder="completed" /></div>
          <div class="form-group"><label>{{ tx("计划筛选", "Filter plan_id") }}</label><input v-model="filter.planID" placeholder="plan_xxx" /></div>
          <div class="form-group"><label>{{ tx("会话筛选", "Filter session_id") }}</label><input v-model="filter.sessionID" placeholder="sess_xxx" /></div>
          <div class="form-group"><label>{{ tx("数量上限", "Limit") }}</label><input v-model.number="filter.limit" type="number" min="1" max="500" /></div>
        </div>
      </div>
    </div>

    <div class="panel">
      <table>
        <thead>
          <tr>
            <th>ID</th>
            <th>{{ tx("标题", "Title") }}</th>
            <th>{{ tx("状态", "Status") }}</th>
            <th>{{ tx("计划", "Plan") }}</th>
            <th>{{ tx("会话", "Session") }}</th>
            <th>{{ tx("操作", "Actions") }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="items.length === 0">
            <td colspan="6" class="small">{{ tx("暂无 Todo", "No todos") }}</td>
          </tr>
          <tr v-for="item in items" :key="item.id">
            <td class="mono">{{ item.id }}</td>
            <td>{{ item.title || "—" }}</td>
            <td>{{ item.status || "—" }}</td>
            <td class="mono">{{ item.plan_id || "—" }}</td>
            <td class="mono">{{ item.session_id || "—" }}</td>
            <td>
              <div class="btn-row">
                <button class="btn btn-outline" @click="view(item.id)">{{ tx("查看", "View") }}</button>
                <button class="btn btn-outline" @click="updateStatus(item.id, 'in_progress')">{{ tx("开始", "Start") }}</button>
                <button class="btn btn-outline" @click="updateStatus(item.id, 'completed')">{{ tx("完成", "Done") }}</button>
                <button class="btn btn-outline" @click="updateStatus(item.id, 'blocked')">{{ tx("阻塞", "Block") }}</button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="panel">
      <div class="panel-head">
        <strong>{{ tx("Todo 详情", "Todo Detail") }}</strong>
      </div>
      <div class="panel-body">
        <textarea v-model="previewText" readonly style="min-height: 150px" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref, watch } from "vue";
import { apiRequest } from "../../lib/api";
import { useAdminI18n } from "../../lib/i18n";
import { toast } from "../../lib/toast";

const { tx } = useAdminI18n();
const createForm = reactive({
  id: "",
  title: "",
  status: "pending",
  planID: "",
  sessionID: "",
  runID: "",
  description: ""
});

const filter = reactive({
  status: "",
  planID: "",
  sessionID: "",
  limit: 100
});

const items = ref<any[]>([]);
const previewText = ref("");

function buildQuery(): string {
  const q = new URLSearchParams();
  if (filter.status.trim()) q.set("status", filter.status.trim());
  if (filter.planID.trim()) q.set("plan_id", filter.planID.trim());
  if (filter.sessionID.trim()) q.set("session_id", filter.sessionID.trim());
  q.set("limit", String(filter.limit));
  return q.toString();
}

async function load() {
  try {
    const data = await apiRequest<any>(`/v1/cc/todos?${buildQuery()}`);
    items.value = Array.isArray(data?.data) ? data.data : [];
  } catch (err: any) {
    toast(`${tx("加载 Todo 失败", "Load todos failed")}: ${err.message || err}`, "err");
  }
}

async function create() {
  if (!createForm.title.trim()) {
    toast(tx("Todo 标题不能为空", "todo title is required"), "err");
    return;
  }
  try {
    const data = await apiRequest<any>("/v1/cc/todos", {
      method: "POST",
      body: JSON.stringify({
        id: createForm.id.trim() || undefined,
        title: createForm.title.trim(),
        status: createForm.status,
        plan_id: createForm.planID.trim(),
        session_id: createForm.sessionID.trim(),
        run_id: createForm.runID.trim(),
        description: createForm.description.trim()
      })
    });
    previewText.value = JSON.stringify(data, null, 2);
    toast(tx("Todo 已创建", "Todo created"));
    await load();
  } catch (err: any) {
    toast(`${tx("创建 Todo 失败", "Create todo failed")}: ${err.message || err}`, "err");
  }
}

async function updateStatus(id: string, status: string) {
  try {
    const data = await apiRequest<any>(`/v1/cc/todos/${encodeURIComponent(id)}`, {
      method: "PUT",
      body: JSON.stringify({ status })
    });
    previewText.value = JSON.stringify(data, null, 2);
    toast(tx("Todo 已更新", "Todo updated"));
    await load();
  } catch (err: any) {
    toast(`${tx("更新 Todo 失败", "Update todo failed")}: ${err.message || err}`, "err");
  }
}

async function view(id: string) {
  try {
    const data = await apiRequest<any>(`/v1/cc/todos/${encodeURIComponent(id)}`);
    previewText.value = JSON.stringify(data, null, 2);
  } catch (err: any) {
    toast(`${tx("查看 Todo 失败", "View todo failed")}: ${err.message || err}`, "err");
  }
}

watch(filter, load, { deep: true });
onMounted(load);
</script>
