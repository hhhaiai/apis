<template>
  <div>
    <div class="section-header">
      <h2>{{ tx("计划编排", "Plan Orchestration") }}</h2>
      <button class="btn btn-outline" @click="load">{{ tx("刷新", "Refresh") }}</button>
    </div>

    <div class="panel">
      <div class="panel-body">
        <div class="grid-4">
          <div class="form-group"><label>ID</label><input v-model="createForm.id" placeholder="plan_custom_1" /></div>
          <div class="form-group"><label>{{ tx("标题", "Title") }}</label><input v-model="createForm.title" :placeholder="tx('高并发执行计划', 'High-concurrency execution plan')" /></div>
          <div class="form-group"><label>{{ tx("会话 ID", "Session ID") }}</label><input v-model="createForm.sessionID" placeholder="sess_xxx" /></div>
          <div class="form-group"><label>{{ tx("运行 ID", "Run ID") }}</label><input v-model="createForm.runID" placeholder="run_xxx" /></div>
        </div>
        <div class="form-group">
          <label>{{ tx("摘要", "Summary") }}</label>
          <input v-model="createForm.summary" :placeholder="tx('计划摘要', 'plan summary')" />
        </div>
        <div class="form-group">
          <label>{{ tx("步骤（每行格式：`title | description`）", "Steps (one line each: `title | description`)") }}</label>
          <textarea
            v-model="createForm.stepsRaw"
            placeholder="Analyze request | classify complexity&#10;Dispatch workers | parallel execute&#10;Reflect and calibrate | finalize response"
            style="min-height: 110px"
          />
        </div>
        <div class="btn-row">
          <button class="btn" @click="create">{{ tx("创建计划", "Create Plan") }}</button>
        </div>
      </div>
    </div>

    <div class="panel">
      <div class="panel-body">
        <div class="grid-4">
          <div class="form-group"><label>{{ tx("状态筛选", "Filter status") }}</label><input v-model="filter.status" placeholder="approved/executing" /></div>
          <div class="form-group"><label>{{ tx("会话筛选", "Filter session_id") }}</label><input v-model="filter.sessionID" placeholder="sess_xxx" /></div>
          <div class="form-group"><label>{{ tx("运行筛选", "Filter run_id") }}</label><input v-model="filter.runID" placeholder="run_xxx" /></div>
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
            <th>{{ tx("步骤数", "Steps") }}</th>
            <th>{{ tx("操作", "Actions") }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="items.length === 0">
            <td colspan="5" class="small">{{ tx("暂无计划", "No plans") }}</td>
          </tr>
          <tr v-for="item in items" :key="item.id">
            <td class="mono">{{ item.id }}</td>
            <td>{{ item.title || "—" }}</td>
            <td>{{ item.status || "—" }}</td>
            <td>{{ item.steps?.length || 0 }}</td>
            <td>
              <div class="btn-row">
                <button class="btn btn-outline" @click="view(item.id)">{{ tx("查看", "View") }}</button>
                <button class="btn btn-outline" @click="approve(item.id)">{{ tx("批准", "Approve") }}</button>
                <button class="btn btn-outline" @click="execute(item.id, 'step')">{{ tx("单步", "Step") }}</button>
                <button class="btn btn-outline" @click="execute(item.id, 'complete')">{{ tx("完成", "Complete") }}</button>
                <button class="btn btn-outline" @click="execute(item.id, 'failed')">{{ tx("失败", "Fail") }}</button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="panel">
      <div class="panel-head">
        <strong>{{ tx("计划详情", "Plan Detail") }}</strong>
      </div>
      <div class="panel-body">
        <textarea v-model="previewText" readonly style="min-height: 160px" />
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
  sessionID: "",
  runID: "",
  summary: "",
  stepsRaw: ""
});

const filter = reactive({
  status: "",
  sessionID: "",
  runID: "",
  limit: 100
});

const items = ref<any[]>([]);
const previewText = ref("");

function parseSteps(raw: string): Array<{ title: string; description?: string }> {
  return (raw || "")
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => {
      const idx = line.indexOf("|");
      if (idx < 0) {
        return { title: line };
      }
      return {
        title: line.slice(0, idx).trim(),
        description: line.slice(idx + 1).trim()
      };
    })
    .filter((step) => !!step.title);
}

function buildQuery(): string {
  const q = new URLSearchParams();
  if (filter.status.trim()) q.set("status", filter.status.trim());
  if (filter.sessionID.trim()) q.set("session_id", filter.sessionID.trim());
  if (filter.runID.trim()) q.set("run_id", filter.runID.trim());
  q.set("limit", String(filter.limit));
  return q.toString();
}

async function load() {
  try {
    const data = await apiRequest<any>(`/v1/cc/plans?${buildQuery()}`);
    items.value = Array.isArray(data?.data) ? data.data : [];
  } catch (err: any) {
    toast(`${tx("加载计划失败", "Load plans failed")}: ${err.message || err}`, "err");
  }
}

async function create() {
  if (!createForm.title.trim()) {
    toast(tx("计划标题不能为空", "plan title is required"), "err");
    return;
  }
  try {
    const data = await apiRequest<any>("/v1/cc/plans", {
      method: "POST",
      body: JSON.stringify({
        id: createForm.id.trim() || undefined,
        title: createForm.title.trim(),
        session_id: createForm.sessionID.trim(),
        run_id: createForm.runID.trim(),
        summary: createForm.summary.trim(),
        steps: parseSteps(createForm.stepsRaw)
      })
    });
    previewText.value = JSON.stringify(data, null, 2);
    toast(tx("计划已创建", "Plan created"));
    await load();
  } catch (err: any) {
    toast(`${tx("创建计划失败", "Create plan failed")}: ${err.message || err}`, "err");
  }
}

async function approve(id: string) {
  try {
    const data = await apiRequest<any>(`/v1/cc/plans/${encodeURIComponent(id)}/approve`, {
      method: "POST",
      body: "{}"
    });
    previewText.value = JSON.stringify(data, null, 2);
    toast(tx("计划已批准", "Plan approved"));
    await load();
  } catch (err: any) {
    toast(`${tx("批准计划失败", "Approve plan failed")}: ${err.message || err}`, "err");
  }
}

async function execute(id: string, mode: "step" | "complete" | "failed") {
  const body: Record<string, any> = {};
  if (mode === "complete") body.complete = true;
  if (mode === "failed") body.failed = true;
  try {
    const data = await apiRequest<any>(`/v1/cc/plans/${encodeURIComponent(id)}/execute`, {
      method: "POST",
      body: JSON.stringify(body)
    });
    previewText.value = JSON.stringify(data, null, 2);
    toast(tx("计划执行状态已更新", "Plan execution updated"));
    await load();
  } catch (err: any) {
    toast(`${tx("执行计划失败", "Execute plan failed")}: ${err.message || err}`, "err");
  }
}

async function view(id: string) {
  try {
    const [plan, todos] = await Promise.all([
      apiRequest<any>(`/v1/cc/plans/${encodeURIComponent(id)}`),
      apiRequest<any>(`/v1/cc/todos?plan_id=${encodeURIComponent(id)}&limit=200`).catch(() => ({ data: [] }))
    ]);
    previewText.value = JSON.stringify({ plan, linked_todos: todos?.data || [] }, null, 2);
  } catch (err: any) {
    toast(`${tx("查看计划失败", "View plan failed")}: ${err.message || err}`, "err");
  }
}

watch(filter, load, { deep: true });
onMounted(load);
</script>
