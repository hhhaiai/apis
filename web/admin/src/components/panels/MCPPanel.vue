<template>
  <div>
    <div class="section-header">
      <h2>{{ tx("MCP 服务", "MCP Servers") }}</h2>
      <button class="btn btn-outline" @click="load">{{ tx("刷新", "Refresh") }}</button>
    </div>

    <div class="panel">
      <div class="panel-head">
        <strong>{{ tx("作用域状态", "Scope State") }}</strong>
      </div>
      <div class="panel-body">
        <p class="small">
          {{ tx("当前 MCP 作用域", "Current MCP scope") }}:
          <span class="mono">{{ scopeLabel }}</span>
          ·
          {{ tx("项目", "Project") }}:
          <span class="mono">{{ projectID }}</span>
        </p>
      </div>
    </div>

    <div class="panel">
      <div class="panel-head">
        <strong>{{ tx("注册 MCP", "Register MCP") }}</strong>
      </div>
      <div class="panel-body">
        <div class="grid-3">
          <div class="form-group">
            <label>{{ tx("服务 ID", "Server ID") }}</label>
            <input v-model="form.id" placeholder="mcp_local" />
          </div>
          <div class="form-group">
            <label>{{ tx("名称", "Name") }}</label>
            <input v-model="form.name" placeholder="local-mcp" />
          </div>
          <div class="form-group">
            <label>{{ tx("传输方式", "Transport") }}</label>
            <select v-model="form.transport">
              <option value="http">http</option>
              <option value="stdio">stdio</option>
            </select>
          </div>
        </div>
        <div class="grid-3">
          <div class="form-group">
            <label>{{ tx("URL (http)", "URL (http)") }}</label>
            <input v-model="form.url" placeholder="http://127.0.0.1:18080/mcp" />
          </div>
          <div class="form-group">
            <label>{{ tx("命令 (stdio)", "Command (stdio)") }}</label>
            <input v-model="form.command" placeholder="npx -y @modelcontextprotocol/server-filesystem" />
          </div>
          <div class="form-group">
            <label>{{ tx("参数 (JSON 数组)", "Args (JSON array)") }}</label>
            <input v-model="form.args" placeholder='["/tmp"]' />
          </div>
        </div>
        <div class="grid-3">
          <div class="form-group">
            <label>{{ tx("超时时间 (ms)", "Timeout (ms)") }}</label>
            <input v-model.number="form.timeoutMS" type="number" min="1000" />
          </div>
          <div class="form-group">
            <label>{{ tx("重试次数", "Retries") }}</label>
            <input v-model.number="form.retries" type="number" min="0" />
          </div>
        </div>
        <div class="btn-row">
          <button class="btn" @click="register">{{ tx("注册", "Register") }}</button>
        </div>
      </div>
    </div>

    <div class="panel">
      <table>
        <thead>
          <tr>
            <th>ID</th>
            <th>{{ tx("名称", "Name") }}</th>
            <th>{{ tx("传输", "Transport") }}</th>
            <th>{{ tx("端点", "Endpoint") }}</th>
            <th>{{ tx("状态", "Status") }}</th>
            <th>{{ tx("操作", "Actions") }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="servers.length === 0">
            <td colspan="6" class="small">{{ tx("暂无 MCP 服务", "No MCP servers") }}</td>
          </tr>
          <tr v-for="item in servers" :key="item.id">
            <td class="mono">{{ item.id }}</td>
            <td>{{ item.name || "—" }}</td>
            <td>{{ item.transport || "—" }}</td>
            <td class="mono">{{ item.url || item.command || "—" }}</td>
            <td>
              <span class="badge" :class="item?.status?.healthy === false ? 'badge-red' : 'badge-green'">
                {{ item?.status?.healthy === false ? tx("不可用", "down") : tx("可用", "up") }}
              </span>
            </td>
            <td>
              <div class="btn-row">
                <button class="btn btn-outline" @click="health(item.id)">{{ tx("健康检查", "Health") }}</button>
                <button class="btn btn-outline" @click="reconnect(item.id)">{{ tx("重连", "Reconnect") }}</button>
                <button class="btn btn-outline" @click="listTools(item.id)">{{ tx("工具", "Tools") }}</button>
                <button class="btn btn-danger" @click="remove(item.id)">{{ tx("删除", "Delete") }}</button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="panel">
      <div class="panel-head">
        <strong>{{ tx("MCP 工具预览", "MCP Tools Preview") }}</strong>
      </div>
      <div class="panel-body">
        <textarea v-model="previewText" readonly style="min-height: 170px" />
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
  id: "",
  name: "",
  transport: "http",
  url: "",
  command: "",
  args: "[]",
  timeoutMS: 15000,
  retries: 1
});

const servers = ref<any[]>([]);
const previewText = ref("");
const scopeLabel = ref("project");
const projectID = ref("default");

function parseArgs(raw: string): any[] {
  const parsed = JSON.parse(raw || "[]");
  if (!Array.isArray(parsed)) {
    throw new Error(tx("args 必须是 JSON 数组", "args must be JSON array"));
  }
  return parsed;
}

async function load() {
  try {
    const data = await apiRequest<any>("/v1/cc/mcp/servers?limit=200");
    scopeLabel.value = String(data?.scope || "project");
    projectID.value = String(data?.project_id || "default");
    servers.value = Array.isArray(data?.data) ? data.data : [];
  } catch (err: any) {
    toast(`${tx("加载 MCP 失败", "Load MCP failed")}: ${err.message || err}`, "err");
  }
}

async function register() {
  try {
    const body: any = {
      id: form.id.trim() || undefined,
      name: form.name.trim(),
      transport: form.transport,
      timeout_ms: form.timeoutMS,
      retries: form.retries
    };
    if (form.transport === "http") {
      body.url = form.url.trim();
    } else {
      body.command = form.command.trim();
      body.args = parseArgs(form.args);
    }
    await apiRequest("/v1/cc/mcp/servers", { method: "POST", body: JSON.stringify(body) });
    toast(tx("MCP 服务已注册", "MCP server registered"));
    await load();
  } catch (err: any) {
    toast(`${tx("注册 MCP 失败", "Register MCP failed")}: ${err.message || err}`, "err");
  }
}

async function health(id: string) {
  try {
    await apiRequest(`/v1/cc/mcp/servers/${encodeURIComponent(id)}/health`, { method: "POST", body: "{}" });
    toast(tx("健康检查完成", "Health check done"));
    await load();
  } catch (err: any) {
    toast(`${tx("健康检查失败", "Health failed")}: ${err.message || err}`, "err");
  }
}

async function reconnect(id: string) {
  try {
    await apiRequest(`/v1/cc/mcp/servers/${encodeURIComponent(id)}/reconnect`, { method: "POST", body: "{}" });
    toast(tx("重连完成", "Reconnect done"));
    await load();
  } catch (err: any) {
    toast(`${tx("重连失败", "Reconnect failed")}: ${err.message || err}`, "err");
  }
}

async function listTools(id: string) {
  try {
    const data = await apiRequest<any>(`/v1/cc/mcp/servers/${encodeURIComponent(id)}/tools/list`, {
      method: "POST",
      body: "{}"
    });
    previewText.value = JSON.stringify(data, null, 2);
  } catch (err: any) {
    toast(`${tx("获取 MCP 工具失败", "List MCP tools failed")}: ${err.message || err}`, "err");
  }
}

async function remove(id: string) {
  if (!window.confirm(tx(`确认删除 MCP 服务 ${id} ?`, `Delete MCP server ${id} ?`))) {
    return;
  }
  try {
    await apiRequest(`/v1/cc/mcp/servers/${encodeURIComponent(id)}`, { method: "DELETE" });
    previewText.value = "";
    toast(tx("MCP 服务已删除", "MCP server deleted"));
    await load();
  } catch (err: any) {
    toast(`${tx("删除 MCP 失败", "Delete MCP failed")}: ${err.message || err}`, "err");
  }
}

onMounted(load);
</script>
