<template>
  <div>
    <div class="section-header">
      <h2>{{ tx("成本追踪", "Cost Tracking") }}</h2>
      <button class="btn btn-outline" @click="load">{{ tx("刷新", "Refresh") }}</button>
    </div>

    <div class="card-grid">
      <div class="card">
        <div class="label">{{ tx("总成本", "Total Cost") }}</div>
        <div class="value">${{ totalCost.toFixed(4) }}</div>
      </div>
      <div class="card">
        <div class="label">{{ tx("预算", "Budget") }}</div>
        <div class="value">{{ budget > 0 ? `$${budget.toFixed(2)}` : tx("不限额", "No Limit") }}</div>
        <div class="small">{{ budget > 0 ? tx(`已使用 ${((totalCost / budget) * 100).toFixed(1)}%`, `${((totalCost / budget) * 100).toFixed(1)}% used`) : tx("无限制", "unlimited") }}</div>
      </div>
      <div class="card">
        <div class="label">{{ tx("模型数", "Models") }}</div>
        <div class="value">{{ rows.length }}</div>
      </div>
    </div>

    <div class="panel">
      <table>
        <thead>
          <tr>
            <th>{{ tx("模型", "Model") }}</th>
            <th>{{ tx("请求数", "Requests") }}</th>
            <th>{{ tx("输入 Tokens", "Input Tokens") }}</th>
            <th>{{ tx("输出 Tokens", "Output Tokens") }}</th>
            <th>{{ tx("成本 (USD)", "Cost (USD)") }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="rows.length === 0">
            <td colspan="5" class="small">{{ tx("暂无成本数据", "No cost data") }}</td>
          </tr>
          <tr v-for="row in rows" :key="row.model">
            <td class="mono">{{ row.model }}</td>
            <td>{{ row.requests }}</td>
            <td>{{ row.input_tokens }}</td>
            <td>{{ row.output_tokens }}</td>
            <td>${{ row.cost.toFixed(4) }}</td>
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

const { tx } = useAdminI18n();
const totalCost = ref(0);
const budget = ref(0);
const rows = ref<Array<any>>([]);

async function load() {
  try {
    const data = await apiRequest<any>("/admin/cost");
    totalCost.value = Number(data?.total_cost_usd || 0);
    budget.value = Number(data?.budget_limit_usd || 0);
    const perModel = data?.per_model || {};
    rows.value = Object.entries(perModel).map(([model, payload]: [string, any]) => ({
      model,
      requests: Number(payload?.requests || 0),
      input_tokens: Number(payload?.input_tokens || 0),
      output_tokens: Number(payload?.output_tokens || 0),
      cost: Number(payload?.cost || 0)
    }));
  } catch (err: any) {
    rows.value = [];
    totalCost.value = 0;
    budget.value = 0;
    toast(`${tx("成本接口不可用", "Cost endpoint unavailable")}: ${err.message || err}`, "err");
  }
}

onMounted(load);
</script>
