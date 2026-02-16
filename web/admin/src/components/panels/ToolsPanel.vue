<template>
  <div>
    <div class="section-header">
      <h2>{{ tx("工具目录", "Tool Catalog") }}</h2>
      <div class="btn-row">
        <button class="btn btn-outline" @click="load">{{ tx("刷新", "Refresh") }}</button>
        <button class="btn" @click="save">{{ tx("保存", "Save") }}</button>
      </div>
    </div>

    <div class="panel">
      <div class="panel-head">
        <strong>{{ tx("作用域状态", "Scope State") }}</strong>
      </div>
      <div class="panel-body">
        <p class="small">
          {{ tx("当前工具目录作用域", "Current tool scope") }}:
          <span class="mono">{{ scopeLabel }}</span>
          ·
          {{ tx("项目", "Project") }}:
          <span class="mono">{{ projectID }}</span>
        </p>
      </div>
    </div>

    <div class="panel">
      <table>
        <thead>
          <tr>
            <th>{{ tx("名称", "Name") }}</th>
            <th>{{ tx("类别", "Category") }}</th>
            <th>{{ tx("状态", "Status") }}</th>
            <th>{{ tx("启用", "Enabled") }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="tools.length === 0">
            <td colspan="4" class="small">{{ tx("暂无工具", "No tools") }}</td>
          </tr>
          <tr v-for="tool in tools" :key="tool.name">
            <td class="mono">{{ tool.name }}</td>
            <td>
              <select v-model="tool.category">
                <option value="supported">supported</option>
                <option value="experimental">experimental</option>
                <option value="unsupported">unsupported</option>
              </select>
            </td>
            <td>
              <select v-model="tool.status">
                <option value="supported">supported</option>
                <option value="experimental">experimental</option>
                <option value="unsupported">unsupported</option>
              </select>
            </td>
            <td>
              <select v-model="tool.enabled">
                <option :value="true">true</option>
                <option :value="false">false</option>
              </select>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="panel">
      <div class="panel-head">
        <strong>{{ tx("未支持工具缺口", "Unsupported Tool Gaps") }}</strong>
        <button class="btn btn-outline" @click="loadGaps">{{ tx("刷新缺口", "Refresh Gaps") }}</button>
      </div>
      <table>
        <thead>
          <tr>
            <th>{{ tx("工具名", "Tool") }}</th>
            <th>{{ tx("原因", "Reason") }}</th>
            <th>{{ tx("次数", "Count") }}</th>
            <th>{{ tx("最近", "Last Seen") }}</th>
            <th>{{ tx("路径", "Path") }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="gaps.length === 0">
            <td colspan="5" class="small">{{ tx("暂无缺口记录", "No gap records") }}</td>
          </tr>
          <tr v-for="row in gaps" :key="`${row.name}-${row.reason}`">
            <td class="mono">{{ row.name }}</td>
            <td>{{ row.reason }}</td>
            <td>{{ row.count }}</td>
            <td class="mono">{{ row.last_seen || "—" }}</td>
            <td class="small">{{ (row.paths || []).join(", ") || "—" }}</td>
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

type ToolSpec = {
  name: string;
  category?: string;
  status?: string;
  enabled?: boolean;
};

const { tx } = useAdminI18n();
const tools = ref<ToolSpec[]>([]);
const gaps = ref<any[]>([]);
const scopeLabel = ref("project");
const projectID = ref("default");

function normalizeTool(item: any): ToolSpec {
  const status = String(item?.status || item?.category || "unsupported");
  const category = String(item?.category || status);
  return {
    name: String(item?.name || ""),
    category,
    status,
    enabled: item?.enabled !== false
  };
}

async function load() {
  try {
    const data = await apiRequest<any>("/admin/tools");
    scopeLabel.value = String(data?.scope || "project");
    projectID.value = String(data?.project_id || "default");
    const list = Array.isArray(data?.tools) ? data.tools : [];
    tools.value = list.map(normalizeTool).filter((item) => item.name);
  } catch (err: any) {
    toast(`${tx("加载工具失败", "Load tools failed")}: ${err.message || err}`, "err");
  }
}

async function save() {
  try {
    const payload = tools.value.map((item) => ({
      name: item.name,
      category: item.category || item.status || "unsupported",
      status: item.status || item.category || "unsupported",
      enabled: item.enabled !== false
    }));
    const data = await apiRequest<any>("/admin/tools", {
      method: "PUT",
      body: JSON.stringify({ tools: payload })
    });
    scopeLabel.value = String(data?.scope || scopeLabel.value);
    projectID.value = String(data?.project_id || projectID.value);
    toast(tx("工具配置已保存", "Tool catalog saved"));
    await load();
  } catch (err: any) {
    toast(`${tx("保存工具失败", "Save tools failed")}: ${err.message || err}`, "err");
  }
}

async function loadGaps() {
  try {
    const data = await apiRequest<any>("/admin/tools/gaps?limit=200");
    if (Array.isArray(data?.gap_summaries)) {
      gaps.value = data.gap_summaries;
      return;
    }
    gaps.value = Array.isArray(data?.summary) ? data.summary : [];
  } catch (err: any) {
    gaps.value = [];
    toast(`${tx("加载工具缺口失败", "Load tool gaps failed")}: ${err.message || err}`, "err");
  }
}

onMounted(async () => {
  await load();
  await loadGaps();
});
</script>
