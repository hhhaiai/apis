<template>
  <div>
    <div class="section-header">
      <h2>{{ tx("调度器设置", "Scheduler Settings") }}</h2>
      <button class="btn" @click="save">{{ tx("保存", "Save") }}</button>
    </div>

    <div class="panel">
      <div class="panel-head">
        <strong>{{ tx("调度器状态", "Scheduler Status") }}</strong>
        <button class="btn btn-outline" @click="load">{{ tx("刷新", "Refresh") }}</button>
      </div>
      <div class="panel-body">
        <div v-if="status.enabled !== undefined" class="card-grid">
          <div class="card">
            <div class="label">{{ tx("启用状态", "Enabled") }}</div>
            <div class="value">{{ status.enabled ? tx("启用", "Enabled") : tx("禁用", "Disabled") }}</div>
          </div>
          <div class="card">
            <div class="label">{{ tx("适配器数量", "Adapters") }}</div>
            <div class="value">{{ status.adapter_count ?? 0 }}</div>
          </div>
          <div class="card">
            <div class="label">{{ tx("调度器适配器", "Scheduler Adapter") }}</div>
            <div class="value mono">{{ status.scheduler_adapter || "—" }}</div>
          </div>
          <div class="card">
            <div class="label">{{ tx("选举时间", "Elected At") }}</div>
            <div class="value">{{ formatTime(status.elected_at) }}</div>
          </div>
        </div>

        <div v-if="status.adapters && status.adapters.length > 0" class="mt-4">
          <h4>{{ tx("适配器列表", "Adapter List") }}</h4>
          <table>
            <thead>
              <tr>
                <th>{{ tx("适配器", "Adapter") }}</th>
                <th>{{ tx("模型", "Model") }}</th>
                <th>{{ tx("状态", "Status") }}</th>
                <th>{{ tx("分数", "Score") }}</th>
                <th>{{ tx("请求数", "Requests") }}</th>
                <th>{{ tx("失败数", "Failures") }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="adapter in status.adapters" :key="adapter.name">
                <td class="mono">{{ adapter.name }}</td>
                <td class="mono">{{ adapter.model || "—" }}</td>
                <td>{{ getAdapterStatus(adapter) }}</td>
                <td>{{ adapter.score ?? "—" }}</td>
                <td>{{ adapter.request_count ?? 0 }}</td>
                <td>{{ adapter.failure_count ?? 0 }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <div class="panel">
      <div class="panel-head">
        <strong>{{ tx("探测配置", "Probe Config") }}</strong>
      </div>
      <div class="panel-body">
        <div class="grid-4">
          <div class="form-group">
            <label>{{ tx("探测间隔 (ms)", "Probe Interval (ms)") }}</label>
            <input v-model.number="form.probeIntervalMS" type="number" min="1000" max="600000" />
          </div>
          <div class="form-group">
            <label>{{ tx("超时时间 (ms)", "Timeout (ms)") }}</label>
            <input v-model.number="form.probeTimeoutMS" type="number" min="100" max="60000" />
          </div>
          <div class="form-group">
            <label>{{ tx("重试次数", "Retries") }}</label>
            <input v-model.number="form.probeRetries" type="number" min="0" max="10" />
          </div>
          <div class="form-group">
            <label>{{ tx("启用探测", "Enable Probe") }}</label>
            <select v-model="form.probeEnabled">
              <option :value="true">{{ tx("启用", "Enabled") }}</option>
              <option :value="false">{{ tx("禁用", "Disabled") }}</option>
            </select>
          </div>
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
  probeIntervalMS: 30000,
  probeTimeoutMS: 5000,
  probeRetries: 3,
  probeEnabled: true
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

function getAdapterStatus(adapter: any): string {
  if (adapter.cooldown_until) return tx("冷却中", "Cooldown");
  if (adapter.healthy === false) return tx("失败", "Failed");
  return tx("活跃", "Active");
}

async function load() {
  try {
    const [scheduler, probe] = await Promise.all([
      apiRequest<any>("/admin/scheduler").catch(() => ({})),
      apiRequest<any>("/admin/probe").catch(() => ({}))
    ]);

    status.value = scheduler?.scheduler || {};

    const probeCfg = probe?.probe || {};
    form.probeIntervalMS = probeCfg.interval_ms ?? 30000;
    form.probeTimeoutMS = probeCfg.timeout_ms ?? 5000;
    form.probeRetries = probeCfg.retries ?? 3;
    form.probeEnabled = probeCfg.enabled ?? true;
  } catch (err: any) {
    toast(`${tx("加载失败", "Load failed")}: ${err.message || err}`, "err");
  }
}

async function save() {
  try {
    await apiRequest("/admin/probe", {
      method: "PUT",
      body: {
        interval_ms: form.probeIntervalMS,
        timeout_ms: form.probeTimeoutMS,
        retries: form.probeRetries,
        enabled: form.probeEnabled
      }
    });
    toast(tx("保存成功", "Saved successfully"), "success");
  } catch (err: any) {
    toast(`${tx("保存失败", "Save failed")}: ${err.message || err}`, "err");
  }
}

onMounted(load);
</script>

<style scoped>
.mt-4 {
  margin-top: 1rem;
}
</style>
