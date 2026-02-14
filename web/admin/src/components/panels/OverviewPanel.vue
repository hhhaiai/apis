<template>
  <div>
    <div class="section-header">
      <h2>{{ tx("系统总览", "System Overview") }}</h2>
      <span class="badge" :class="healthOk ? 'badge-green' : 'badge-red'">
        {{ healthOk ? tx("健康", "Healthy") : tx("异常", "Unhealthy") }}
      </span>
    </div>

    <div class="card-grid">
      <div v-for="card in cards" :key="card.label" class="card">
        <div class="label">{{ card.label }}</div>
        <div class="value">{{ card.value }}</div>
        <div class="small">{{ card.sub }}</div>
      </div>
    </div>

    <div class="panel">
      <div class="panel-head">
        <strong>{{ tx("适配器状态", "Adapter Status") }}</strong>
        <button class="btn btn-outline" @click="load">{{ tx("刷新", "Refresh") }}</button>
      </div>
      <table>
        <thead>
          <tr>
            <th>{{ tx("适配器", "Adapter") }}</th>
            <th>{{ tx("状态", "Status") }}</th>
            <th>{{ tx("模型", "Model") }}</th>
            <th>{{ tx("请求数", "Requests") }}</th>
            <th>{{ tx("失败数", "Failures") }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="adapters.length === 0">
            <td colspan="5" class="small">{{ tx("暂无调度数据", "No scheduler data") }}</td>
          </tr>
          <tr v-for="item in adapters" :key="item.name || item.id">
            <td class="mono">{{ item.name || item.id || "—" }}</td>
            <td>{{ item.cooldown_until ? tx("冷却中", "Cooldown") : item.healthy !== false ? tx("活跃", "Active") : tx("失败", "Failed") }}</td>
            <td class="mono">{{ item.model || "—" }}</td>
            <td>{{ item.request_count || 0 }}</td>
            <td>{{ item.failure_count || 0 }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue";
import { apiRequest } from "../../lib/api";
import { useAdminI18n } from "../../lib/i18n";
import { toast } from "../../lib/toast";

type Card = { label: string; value: string; sub: string };

const { tx } = useAdminI18n();
const healthOk = ref(true);
const cards = ref<Card[]>([]);
const adapters = ref<any[]>([]);

async function load() {
  try {
    const [health, settings, scheduler] = await Promise.all([
      apiRequest<any>("/healthz").catch(() => ({ ok: false })),
      apiRequest<any>("/admin/settings").catch(() => ({})),
      apiRequest<any>("/admin/scheduler").catch(() => ({ scheduler: {} }))
    ]);

    healthOk.value = !!health?.ok;
    cards.value = [
      {
        label: tx("反思轮次", "Reflection Passes"),
        value: String(settings?.routing?.reflection_passes ?? 1),
        sub: tx("每次请求", "per request")
      },
      {
        label: tx("并行候选", "Parallel Candidates"),
        value: String(settings?.routing?.parallel_candidates ?? 1),
        sub: tx("调度链路", "scheduler path")
      },
      {
        label: tx("工具循环", "Tool Loop"),
        value: String(settings?.tool_loop?.mode ?? "client_loop"),
        sub: tx(`最多 ${settings?.tool_loop?.max_steps ?? 4} 步`, `max ${settings?.tool_loop?.max_steps ?? 4} steps`)
      },
      {
        label: tx("超时时间", "Timeout"),
        value: `${Math.floor((settings?.routing?.timeout_ms ?? 30000) / 1000)}s`,
        sub: tx("上游请求", "upstream request")
      }
    ];

    const sched = scheduler?.scheduler || scheduler || {};
    adapters.value = Array.isArray(sched.adapters) ? sched.adapters : [];
  } catch (err: any) {
    toast(`${tx("总览加载失败", "Overview load failed")}: ${err.message || err}`, "err");
  }
}

onMounted(load);
</script>
