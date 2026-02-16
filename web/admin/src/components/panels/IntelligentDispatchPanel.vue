<template>
  <div>
    <div class="section-header">
      <h2>{{ tx("智能调度设置", "Intelligent Dispatch Settings") }}</h2>
      <button class="btn" @click="save">{{ tx("保存", "Save") }}</button>
    </div>

    <div class="panel">
      <div class="panel-head">
        <strong>{{ tx("基本设置", "Basic Settings") }}</strong>
      </div>
      <div class="panel-body">
        <div class="grid-4">
          <div class="form-group">
            <label>{{ tx("启用智能调度", "Enable Intelligent Dispatch") }}</label>
            <select v-model="form.enabled">
              <option :value="true">{{ tx("启用", "Enabled") }}</option>
              <option :value="false">{{ tx("禁用", "Disabled") }}</option>
            </select>
          </div>
          <div class="form-group">
            <label>{{ tx("失败时回退到调度器", "Fallback to Scheduler on Failure") }}</label>
            <select v-model="form.fallbackToScheduler">
              <option :value="true">{{ tx("启用", "Enabled") }}</option>
              <option :value="false">{{ tx("禁用", "Disabled") }}</option>
            </select>
          </div>
        </div>

        <div class="grid-4">
          <div class="form-group">
            <label>{{ tx("最小分数差", "Min Score Difference") }}</label>
            <input v-model.number="form.minScoreDifference" type="number" min="0" max="100" step="0.1" />
          </div>
          <div class="form-group">
            <label>{{ tx("重新选举间隔 (ms)", "Re-elect Interval (ms)") }}</label>
            <input v-model.number="form.reElectIntervalMS" type="number" min="60000" max="3600000" step="60000" />
          </div>
        </div>
      </div>
    </div>

    <div class="panel">
      <div class="panel-head">
        <strong>{{ tx("复杂度阈值", "Complexity Thresholds") }}</strong>
      </div>
      <div class="panel-body">
        <div class="grid-4">
          <div class="form-group">
            <label>{{ tx("长上下文字符数", "Long Context Chars") }}</label>
            <input v-model.number="form.longContextChars" type="number" min="1000" max="100000" />
          </div>
          <div class="form-group">
            <label>{{ tx("工具数量阈值", "Tool Count Threshold") }}</label>
            <input v-model.number="form.toolCountThreshold" type="number" min="0" max="20" />
          </div>
        </div>
      </div>
    </div>

    <div class="panel">
      <div class="panel-head">
        <strong>{{ tx("模型策略 (JSON)", "Model Policies (JSON)") }}</strong>
      </div>
      <div class="panel-body">
        <div class="form-group">
          <textarea v-model="form.modelPoliciesJSON" rows="8" />
        </div>
      </div>
    </div>

    <div class="panel">
      <div class="panel-head">
        <strong>{{ tx("调度状态", "Dispatch Status") }}</strong>
        <div class="panel-actions">
          <button class="btn btn-outline" @click="rerun">{{ tx("重新选举", "Re-run Election") }}</button>
          <button class="btn btn-outline" @click="resetStats">{{ tx("重置统计", "Reset Stats") }}</button>
          <button class="btn btn-outline" @click="load">{{ tx("刷新", "Refresh") }}</button>
        </div>
      </div>
      <div class="panel-body">
        <div class="card-grid">
          <div class="card">
            <div class="label">{{ tx("复杂任务路由", "Complex Routed") }}</div>
            <div class="value">{{ status.stats?.complex_routed ?? 0 }}</div>
          </div>
          <div class="card">
            <div class="label">{{ tx("简单任务路由", "Simple Routed") }}</div>
            <div class="value">{{ status.stats?.simple_routed ?? 0 }}</div>
          </div>
          <div class="card">
            <div class="label">{{ tx("回退次数", "Fallback Count") }}</div>
            <div class="value">{{ status.stats?.fallback_count ?? 0 }}</div>
          </div>
        </div>

        <div v-if="status.election" class="mt-4">
          <h4>{{ tx("选举信息", "Election Info") }}</h4>
          <div class="grid-4">
            <div class="form-group">
              <label>{{ tx("调度器适配器", "Scheduler Adapter") }}</label>
              <input :value="status.election.scheduler_adapter || '—'" readonly />
            </div>
            <div class="form-group">
              <label>{{ tx("选举时间", "Elected At") }}</label>
              <input :value="formatTime(status.election.elected_at)" readonly />
            </div>
          </div>
        </div>

        <div v-if="status.recent_events && status.recent_events.length > 0" class="mt-4">
          <h4>{{ tx("最近事件", "Recent Events") }}</h4>
          <table>
            <thead>
              <tr>
                <th>{{ tx("时间", "Time") }}</th>
                <th>{{ tx("类型", "Type") }}</th>
                <th>{{ tx("复杂度", "Complexity") }}</th>
                <th>{{ tx("选择", "Selected") }}</th>
                <th>{{ tx("原因", "Reason") }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(event, idx) in status.recent_events" :key="idx">
                <td>{{ formatTime(event.timestamp) }}</td>
                <td>{{ event.event_type }}</td>
                <td>{{ event.complexity || "—" }}</td>
                <td>{{ event.selected || "—" }}</td>
                <td>{{ event.reason || "—" }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";
import { apiRequest } from "../../lib/api";
import { useAdminI18n } from "../../lib/i18n";
import { toast } from "../../lib/toast";

const { tx } = useAdminI18n();

const form = reactive({
  enabled: true,
  fallbackToScheduler: true,
  minScoreDifference: 5.0,
  reElectIntervalMS: 600000,
  longContextChars: 4000,
  toolCountThreshold: 1,
  modelPoliciesJSON: "{}"
});

const status = ref<any>({});

function formatTime(ts: string | undefined): string {
  if (!ts) return "—";
  try {
    return new Date(ts).toLocaleString();
  } catch {
    return ts;
  }
}

async function load() {
  try {
    const data = await apiRequest<any>("/admin/intelligent-dispatch");
    const settings = data.settings || {};
    const statusData = data.status || {};

    form.enabled = settings.enabled ?? true;
    form.fallbackToScheduler = settings.fallback_to_scheduler ?? true;
    form.minScoreDifference = settings.min_score_difference ?? 5.0;
    form.reElectIntervalMS = settings.re_elect_interval_ms ?? 600000;
    form.longContextChars = settings.complexity_thresholds?.long_context_chars ?? 4000;
    form.toolCountThreshold = settings.complexity_thresholds?.tool_count_threshold ?? 1;

    // Model policies
    const policies = settings.model_policies || {};
    form.modelPoliciesJSON = JSON.stringify(policies, null, 2);

    status.value = statusData;
  } catch (err: any) {
    toast(`${tx("加载失败", "Load failed")}: ${err.message || err}`, "err");
  }
}

async function save() {
  try {
    let modelPolicies = {};
    try {
      modelPolicies = JSON.parse(form.modelPoliciesJSON || "{}");
    } catch {
      toast(tx("模型策略 JSON 格式错误", "Model policies JSON format error"), "err");
      return;
    }

    await apiRequest("/admin/intelligent-dispatch", {
      method: "PUT",
      body: {
        enabled: form.enabled,
        fallback_to_scheduler: form.fallbackToScheduler,
        min_score_difference: form.minScoreDifference,
        re_elect_interval_ms: form.reElectIntervalMS,
        complexity_thresholds: {
          long_context_chars: form.longContextChars,
          tool_count_threshold: form.toolCountThreshold
        },
        model_policies: modelPolicies
      }
    });
    toast(tx("保存成功", "Saved successfully"), "success");
    await load();
  } catch (err: any) {
    toast(`${tx("保存失败", "Save failed")}: ${err.message || err}`, "err");
  }
}

async function rerun() {
  try {
    await apiRequest("/admin/intelligent-dispatch?action=rerun", { method: "POST" });
    toast(tx("重新选举已触发", "Re-election triggered"), "success");
    await load();
  } catch (err: any) {
    toast(`${tx("操作失败", "Operation failed")}: ${err.message || err}`, "err");
  }
}

async function resetStats() {
  try {
    await apiRequest("/admin/intelligent-dispatch?action=reset-stats", { method: "POST" });
    toast(tx("统计已重置", "Stats reset"), "success");
    await load();
  } catch (err: any) {
    toast(`${tx("操作失败", "Operation failed")}: ${err.message || err}`, "err");
  }
}

onMounted(load);
</script>

<style scoped>
.mt-4 {
  margin-top: 1rem;
}
.panel-actions {
  display: flex;
  gap: 0.5rem;
}
</style>
