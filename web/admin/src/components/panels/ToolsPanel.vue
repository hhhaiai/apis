<template>
  <div>
    <div class="section-header">
      <h2>{{ tx("工具目录", "Tool Catalog") }}</h2>
      <button class="btn btn-outline" @click="load">{{ tx("刷新", "Refresh") }}</button>
    </div>

    <div class="panel">
      <table>
        <thead>
          <tr>
            <th>{{ tx("名称", "Name") }}</th>
            <th>{{ tx("类别", "Category") }}</th>
            <th>{{ tx("状态", "Status") }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="tools.length === 0">
            <td colspan="3" class="small">{{ tx("暂无工具", "No tools") }}</td>
          </tr>
          <tr v-for="tool in tools" :key="tool.name">
            <td class="mono">{{ tool.name }}</td>
            <td>
              <span
                class="badge"
                :class="
                  tool.category === 'supported'
                    ? 'badge-green'
                    : tool.category === 'experimental'
                      ? 'badge-orange'
                      : 'badge-red'
                "
              >
                {{ tool.category || tx("未知", "unknown") }}
              </span>
            </td>
            <td>{{ tool.enabled !== false ? tx("启用", "Enabled") : tx("禁用", "Disabled") }}</td>
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
const tools = ref<any[]>([]);

async function load() {
  try {
    const data = await apiRequest<any>("/admin/tools");
    tools.value = Array.isArray(data?.tools) ? data.tools : [];
  } catch (err: any) {
    toast(`${tx("加载工具失败", "Load tools failed")}: ${err.message || err}`, "err");
  }
}

onMounted(load);
</script>
