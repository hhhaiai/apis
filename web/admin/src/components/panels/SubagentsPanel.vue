<template>
  <div>
    <div class="section-header">
      <h2>{{ tx("子代理", "Subagents") }}</h2>
      <button class="btn btn-outline" @click="load">{{ tx("刷新", "Refresh") }}</button>
    </div>

    <div class="panel">
      <div class="panel-body">
        <div class="grid-4">
          <div class="form-group">
            <label>{{ tx("父级/团队 ID", "Parent/Team ID") }}</label>
            <input v-model="filter.parentID" placeholder="team_alpha" />
          </div>
          <div class="form-group">
            <label>{{ tx("状态", "Status") }}</label>
            <input v-model="filter.status" placeholder="running/completed" />
          </div>
          <div class="form-group">
            <label>{{ tx("模型", "Model") }}</label>
            <input v-model="filter.model" placeholder="gpt-4o" />
          </div>
          <div class="form-group">
            <label>{{ tx("包含已删除", "Include Deleted") }}</label>
            <select v-model="filter.includeDeleted">
              <option :value="false">{{ tx("否", "false") }}</option>
              <option :value="true">{{ tx("是", "true") }}</option>
            </select>
          </div>
        </div>
      </div>
    </div>

    <div class="panel">
      <table>
        <thead>
          <tr>
            <th>ID</th>
            <th>{{ tx("父级", "Parent") }}</th>
            <th>{{ tx("状态", "Status") }}</th>
            <th>{{ tx("模型", "Model") }}</th>
            <th>{{ tx("操作", "Actions") }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="items.length === 0">
            <td colspan="5" class="small">{{ tx("暂无子代理", "No subagents") }}</td>
          </tr>
          <tr v-for="item in items" :key="item.id">
            <td class="mono">{{ item.id }}</td>
            <td class="mono">{{ item.parent_id || "—" }}</td>
            <td>{{ item.status || "—" }}</td>
            <td class="mono">{{ item.model || "—" }}</td>
            <td>
              <div class="btn-row">
                <button class="btn btn-outline" @click="loadTimeline(item.id)">{{ tx("时间线", "Timeline") }}</button>
                <button class="btn btn-outline" @click="terminate(item.id)">{{ tx("终止", "Terminate") }}</button>
                <button class="btn btn-danger" @click="remove(item.id)">{{ tx("删除", "Delete") }}</button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="panel">
      <div class="panel-head">
        <strong>{{ tx("时间线", "Timeline") }}</strong>
      </div>
      <div class="panel-body">
        <textarea v-model="timelineText" readonly style="min-height: 160px" />
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
const filter = reactive({
  parentID: "",
  status: "",
  model: "",
  includeDeleted: false
});

const items = ref<any[]>([]);
const timelineText = ref("");

function query(): string {
  const q = new URLSearchParams();
  if (filter.parentID.trim()) {
    q.set("team_id", filter.parentID.trim());
  }
  if (filter.status.trim()) {
    q.set("status", filter.status.trim());
  }
  if (filter.model.trim()) {
    q.set("model", filter.model.trim());
  }
  if (filter.includeDeleted) {
    q.set("include_deleted", "true");
  }
  return q.toString();
}

async function load() {
  try {
    const q = query();
    const data = await apiRequest<any>(`/v1/cc/subagents${q ? `?${q}` : ""}`);
    items.value = Array.isArray(data?.data) ? data.data : [];
  } catch (err: any) {
    toast(`${tx("加载子代理失败", "Load subagents failed")}: ${err.message || err}`, "err");
  }
}

async function loadTimeline(id: string) {
  try {
    const data = await apiRequest<any>(`/v1/cc/subagents/${encodeURIComponent(id)}/timeline?limit=200`);
    timelineText.value = JSON.stringify(data, null, 2);
  } catch (err: any) {
    toast(`${tx("加载时间线失败", "Load timeline failed")}: ${err.message || err}`, "err");
  }
}

async function terminate(id: string) {
  const by = window.prompt(tx("终止人", "terminated by"), "admin") || "";
  const reason = window.prompt(tx("原因", "reason"), tx("手动停止", "manual stop")) || "";
  try {
    await apiRequest(`/v1/cc/subagents/${encodeURIComponent(id)}/terminate`, {
      method: "POST",
      body: JSON.stringify({ by, reason })
    });
    toast(tx("子代理已终止", "Subagent terminated"));
    await load();
    await loadTimeline(id);
  } catch (err: any) {
    toast(`${tx("终止失败", "Terminate failed")}: ${err.message || err}`, "err");
  }
}

async function remove(id: string) {
  if (!window.confirm(tx(`确认删除子代理 ${id} ?`, `Delete subagent ${id} ?`))) {
    return;
  }
  const by = window.prompt(tx("删除人", "deleted by"), "admin") || "";
  const reason = window.prompt(tx("原因", "reason"), tx("清理", "cleanup")) || "";
  try {
    await apiRequest(`/v1/cc/subagents/${encodeURIComponent(id)}`, {
      method: "DELETE",
      body: JSON.stringify({ by, reason })
    });
    toast(tx("子代理已删除", "Subagent deleted"));
    await load();
    await loadTimeline(id);
  } catch (err: any) {
    toast(`${tx("删除子代理失败", "Delete subagent failed")}: ${err.message || err}`, "err");
  }
}

watch(filter, load, { deep: true });
onMounted(load);
</script>
