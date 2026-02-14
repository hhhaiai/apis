<template>
  <div>
    <div class="section-header">
      <h2>{{ tx("事件", "Events") }}</h2>
      <div class="btn-row">
        <button class="btn btn-outline" @click="load">{{ tx("刷新", "Refresh") }}</button>
        <button class="btn btn-outline" @click="startStream">{{ tx("启动 SSE", "Start SSE") }}</button>
        <button class="btn btn-danger" @click="stopStream">{{ tx("停止 SSE", "Stop SSE") }}</button>
      </div>
    </div>

    <div class="panel">
      <div class="panel-body">
        <div class="grid-4">
          <div class="form-group"><label>event_type</label><input v-model="filter.eventType" placeholder="team.task.completed" /></div>
          <div class="form-group"><label>session_id</label><input v-model="filter.sessionID" placeholder="sess_xxx" /></div>
          <div class="form-group"><label>run_id</label><input v-model="filter.runID" placeholder="run_xxx" /></div>
          <div class="form-group"><label>plan_id</label><input v-model="filter.planID" placeholder="plan_xxx" /></div>
          <div class="form-group"><label>todo_id</label><input v-model="filter.todoID" placeholder="todo_xxx" /></div>
          <div class="form-group"><label>team_id</label><input v-model="filter.teamID" placeholder="team_alpha" /></div>
          <div class="form-group"><label>subagent_id</label><input v-model="filter.subagentID" placeholder="agent_xxx" /></div>
          <div class="form-group"><label>limit</label><input v-model.number="filter.limit" type="number" min="1" max="500" /></div>
        </div>
      </div>
    </div>

    <div class="panel">
      <table>
        <thead>
          <tr>
            <th>{{ tx("时间", "Time") }}</th>
            <th>{{ tx("类型", "Type") }}</th>
            <th>{{ tx("团队", "Team") }}</th>
            <th>{{ tx("子代理", "Subagent") }}</th>
            <th>{{ tx("记录", "Record") }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="items.length === 0">
            <td colspan="5" class="small">{{ tx("暂无事件", "No events") }}</td>
          </tr>
          <tr v-for="item in items" :key="item.id">
            <td class="mono">{{ item.created_at || "—" }}</td>
            <td>{{ item.event_type || "—" }}</td>
            <td class="mono">{{ item.team_id || item?.data?.team_id || "—" }}</td>
            <td class="mono">{{ item.subagent_id || item?.data?.subagent_id || "—" }}</td>
            <td class="small">{{ item?.data?.record_text || "—" }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, reactive, ref } from "vue";
import { apiRequest } from "../../lib/api";
import { useAdminI18n } from "../../lib/i18n";
import { toast } from "../../lib/toast";

const { tx } = useAdminI18n();
const filter = reactive({
  eventType: "",
  sessionID: "",
  runID: "",
  planID: "",
  todoID: "",
  teamID: "",
  subagentID: "",
  limit: 100
});

const items = ref<any[]>([]);
let source: EventSource | null = null;

function buildQuery(includeLimit = true): URLSearchParams {
  const q = new URLSearchParams();
  if (filter.eventType.trim()) q.set("event_type", filter.eventType.trim());
  if (filter.sessionID.trim()) q.set("session_id", filter.sessionID.trim());
  if (filter.runID.trim()) q.set("run_id", filter.runID.trim());
  if (filter.planID.trim()) q.set("plan_id", filter.planID.trim());
  if (filter.todoID.trim()) q.set("todo_id", filter.todoID.trim());
  if (filter.teamID.trim()) q.set("team_id", filter.teamID.trim());
  if (filter.subagentID.trim()) q.set("subagent_id", filter.subagentID.trim());
  if (includeLimit && filter.limit > 0) q.set("limit", String(filter.limit));
  return q;
}

async function load() {
  try {
    const q = buildQuery(true);
    const data = await apiRequest<any>(`/v1/cc/events${q.toString() ? `?${q.toString()}` : ""}`);
    items.value = Array.isArray(data?.data) ? data.data : [];
  } catch (err: any) {
    toast(`${tx("加载事件失败", "Load events failed")}: ${err.message || err}`, "err");
  }
}

function upsert(ev: any) {
  items.value = [ev, ...items.value].slice(0, Math.max(1, filter.limit));
}

function startStream() {
  stopStream();
  const q = buildQuery(false);
  source = new EventSource(`/v1/cc/events/stream${q.toString() ? `?${q.toString()}` : ""}`);

  const onData = (msg: MessageEvent) => {
    try {
      const ev = JSON.parse(msg.data || "{}");
      upsert(ev);
    } catch {
      // ignore parse error
    }
  };

  const knownTypes = [
    "run.created",
    "run.completed",
    "run.failed",
    "todo.created",
    "todo.updated",
    "todo.completed",
    "plan.created",
    "plan.approved",
    "plan.executing",
    "plan.completed",
    "plan.failed",
    "plan.todos_created",
    "plan.todos_synced",
    "plan.step_advanced",
    "plan.auto_completed",
    "team.created",
    "team.agent.added",
    "team.task.created",
    "team.task.running",
    "team.task.completed",
    "team.task.failed",
    "team.message.sent",
    "team.orchestrated",
    "subagent.created",
    "subagent.running",
    "subagent.completed",
    "subagent.failed",
    "subagent.terminated",
    "subagent.deleted",
    "plugin.installed",
    "plugin.enabled",
    "plugin.disabled",
    "plugin.uninstalled"
  ];

  if (filter.eventType.trim()) {
    source.addEventListener(filter.eventType.trim(), onData as EventListener);
  } else {
    for (const t of knownTypes) {
      source.addEventListener(t, onData as EventListener);
    }
    source.onmessage = onData;
  }

  source.onerror = () => toast(tx("事件流已断开", "Event stream disconnected"), "err");
  toast(tx("事件流已启动", "Event stream started"));
}

function stopStream() {
  if (source) {
    source.close();
    source = null;
    toast(tx("事件流已停止", "Event stream stopped"));
  }
}

onBeforeUnmount(stopStream);
</script>
