<template>
  <div>
    <div class="section-header">
      <h2>{{ tx("模型路由", "Model Routing") }}</h2>
      <button class="btn btn-outline" @click="load">{{ tx("刷新", "Refresh") }}</button>
    </div>

    <div class="card-grid">
      <div v-for="item in modeItems" :key="item.mode" class="card">
        <div class="label">{{ item.mode }}</div>
        <div class="value mono" style="font-size: 15px">{{ item.model }}</div>
      </div>
      <div v-if="modeItems.length === 0" class="card">
        <div class="label">{{ tx("默认", "Default") }}</div>
        <div class="value" style="font-size: 14px">{{ tx("未配置模式覆写", "No mode override configured") }}</div>
      </div>
    </div>

    <div class="panel">
      <div class="panel-head">
        <strong>{{ tx("模式路由链", "Mode Route Chains") }}</strong>
      </div>
      <table>
        <thead>
          <tr>
            <th>{{ tx("模式", "Mode") }}</th>
            <th>{{ tx("路由", "Route") }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="routeItems.length === 0">
            <td colspan="2" class="small">{{ tx("未配置路由链", "No route chain configured") }}</td>
          </tr>
          <tr v-for="item in routeItems" :key="item.mode">
            <td>{{ item.mode }}</td>
            <td class="mono">{{ item.chain }}</td>
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
const modeItems = ref<Array<{ mode: string; model: string }>>([]);
const routeItems = ref<Array<{ mode: string; chain: string }>>([]);

async function load() {
  try {
    const settings = await apiRequest<any>("/admin/settings");
    const modeModels = settings?.mode_models || {};
    modeItems.value = Object.entries(modeModels).map(([mode, model]) => ({
      mode,
      model: String(model)
    }));
    const modeRoutes = settings?.routing?.mode_routes || {};
    routeItems.value = Object.entries(modeRoutes).map(([mode, chain]) => ({
      mode,
      chain: Array.isArray(chain) ? chain.join(" -> ") : String(chain)
    }));
  } catch (err: any) {
    toast(`${tx("加载模型路由失败", "Load models failed")}: ${err.message || err}`, "err");
  }
}

onMounted(load);
</script>
