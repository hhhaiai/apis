<template>
  <div>
    <div class="section-header">
      <h2>{{ tx("插件中心", "Plugin Center") }}</h2>
      <button class="btn btn-outline" @click="load">{{ tx("刷新", "Refresh") }}</button>
    </div>

    <div class="panel">
      <div class="panel-body">
        <div class="grid-3">
          <div class="form-group">
            <label>{{ tx("名称", "Name") }}</label>
            <input v-model="form.name" placeholder="planner_pack" />
          </div>
          <div class="form-group">
            <label>{{ tx("版本", "Version") }}</label>
            <input v-model="form.version" placeholder="1.0.0" />
          </div>
          <div class="form-group">
            <label>{{ tx("描述", "Description") }}</label>
            <input v-model="form.description" :placeholder="tx('插件描述', 'Plugin description')" />
          </div>
        </div>
        <div class="grid-3">
          <div class="form-group">
            <label>{{ tx("技能 (JSON 数组)", "Skills (JSON array)") }}</label>
            <textarea v-model="form.skills" />
          </div>
          <div class="form-group">
            <label>{{ tx("Hooks (JSON 数组)", "Hooks (JSON array)") }}</label>
            <textarea v-model="form.hooks" />
          </div>
          <div class="form-group">
            <label>{{ tx("MCP 服务 (JSON 数组)", "MCP Servers (JSON array)") }}</label>
            <textarea v-model="form.mcpServers" />
          </div>
        </div>
        <div class="btn-row">
          <button class="btn" @click="install">{{ tx("安装", "Install") }}</button>
          <button class="btn btn-outline" @click="load">{{ tx("重新加载", "Reload") }}</button>
        </div>
      </div>
    </div>

    <div class="panel">
      <table>
        <thead>
          <tr>
            <th>{{ tx("名称", "Name") }}</th>
            <th>{{ tx("版本", "Version") }}</th>
            <th>{{ tx("状态", "Status") }}</th>
            <th>{{ tx("技能数", "Skills") }}</th>
            <th>{{ tx("Hooks 数", "Hooks") }}</th>
            <th>{{ tx("MCP 数", "MCP") }}</th>
            <th>{{ tx("操作", "Actions") }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="plugins.length === 0">
            <td colspan="7" class="small">{{ tx("暂无插件", "No plugins") }}</td>
          </tr>
          <tr v-for="item in plugins" :key="item.name">
            <td class="mono">{{ item.name }}</td>
            <td class="mono">{{ item.version || "—" }}</td>
            <td>
              <span class="badge" :class="item.enabled ? 'badge-green' : 'badge-red'">
                {{ item.enabled ? tx("启用", "enabled") : tx("禁用", "disabled") }}
              </span>
            </td>
            <td>{{ item.skills?.length || 0 }}</td>
            <td>{{ item.hooks?.length || 0 }}</td>
            <td>{{ item.mcp_servers?.length || 0 }}</td>
            <td>
              <div class="btn-row">
                <button class="btn btn-outline" @click="view(item.name)">{{ tx("查看", "View") }}</button>
                <button class="btn btn-outline" @click="toggle(item.name, true)">{{ tx("启用", "Enable") }}</button>
                <button class="btn btn-outline" @click="toggle(item.name, false)">{{ tx("禁用", "Disable") }}</button>
                <button class="btn btn-danger" @click="remove(item.name)">{{ tx("删除", "Delete") }}</button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="panel">
      <div class="panel-head">
        <strong>{{ tx("插件详情", "Plugin Detail") }}</strong>
      </div>
      <div class="panel-body">
        <textarea v-model="previewText" readonly style="min-height: 150px" />
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
  name: "",
  version: "",
  description: "",
  skills: "[]",
  hooks: "[]",
  mcpServers: "[]"
});

const plugins = ref<any[]>([]);
const previewText = ref("");

function parseJSONArray(raw: string, label: string): any[] {
  const parsed = JSON.parse(raw || "[]");
  if (!Array.isArray(parsed)) {
    throw new Error(tx(`${label} 必须是 JSON 数组`, `${label} must be JSON array`));
  }
  return parsed;
}

async function load() {
  try {
    const data = await apiRequest<any>("/v1/cc/plugins?limit=200");
    plugins.value = Array.isArray(data?.data) ? data.data : [];
  } catch (err: any) {
    toast(`${tx("加载插件失败", "Load plugins failed")}: ${err.message || err}`, "err");
  }
}

async function install() {
  if (!form.name.trim()) {
    toast(tx("插件名称不能为空", "plugin name is required"), "err");
    return;
  }
  try {
    const body = {
      name: form.name.trim(),
      version: form.version.trim(),
      description: form.description.trim(),
      skills: parseJSONArray(form.skills, "skills"),
      hooks: parseJSONArray(form.hooks, "hooks"),
      mcp_servers: parseJSONArray(form.mcpServers, "mcp_servers")
    };
    const data = await apiRequest<any>("/v1/cc/plugins", {
      method: "POST",
      body: JSON.stringify(body)
    });
    previewText.value = JSON.stringify(data, null, 2);
    toast(tx("插件已安装", "Plugin installed"));
    await load();
  } catch (err: any) {
    toast(`${tx("安装插件失败", "Install plugin failed")}: ${err.message || err}`, "err");
  }
}

async function view(name: string) {
  try {
    const data = await apiRequest<any>(`/v1/cc/plugins/${encodeURIComponent(name)}`);
    previewText.value = JSON.stringify(data, null, 2);
  } catch (err: any) {
    toast(`${tx("查看插件失败", "View plugin failed")}: ${err.message || err}`, "err");
  }
}

async function toggle(name: string, enabled: boolean) {
  try {
    const data = await apiRequest<any>(
      `/v1/cc/plugins/${encodeURIComponent(name)}/${enabled ? "enable" : "disable"}`,
      { method: "POST", body: "{}" }
    );
    previewText.value = JSON.stringify(data, null, 2);
    toast(enabled ? tx("插件已启用", "Plugin enabled") : tx("插件已禁用", "Plugin disabled"));
    await load();
  } catch (err: any) {
    toast(`${tx("切换插件状态失败", "Toggle plugin failed")}: ${err.message || err}`, "err");
  }
}

async function remove(name: string) {
  if (!window.confirm(tx(`确认删除插件 ${name} ?`, `Delete plugin ${name} ?`))) {
    return;
  }
  try {
    await apiRequest(`/v1/cc/plugins/${encodeURIComponent(name)}`, { method: "DELETE" });
    previewText.value = "";
    toast(tx("插件已删除", "Plugin deleted"));
    await load();
  } catch (err: any) {
    toast(`${tx("删除插件失败", "Delete plugin failed")}: ${err.message || err}`, "err");
  }
}

onMounted(load);
</script>
