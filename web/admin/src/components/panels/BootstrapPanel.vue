<template>
  <div>
    <div class="section-header">
      <h2>{{ tx("配置导入", "Bootstrap Config") }}</h2>
      <div class="btn-row">
        <button class="btn btn-outline" @click="loadTemplate">{{ tx("加载模板", "Load Template") }}</button>
        <button class="btn" @click="applyConfig">{{ tx("应用配置", "Apply Config") }}</button>
      </div>
    </div>

    <div class="panel">
      <div class="panel-head">
        <strong>{{ tx("JSON 配置", "JSON Config") }}</strong>
      </div>
      <div class="panel-body">
        <p class="small">
          {{ tx("支持 tools/plugins/mcp_servers/upstream；插件与 MCP 会按当前作用域和项目生效。", "Supports tools/plugins/mcp_servers/upstream; plugins and MCP will follow current scope and project.") }}
        </p>
        <textarea v-model="configText" style="min-height: 280px" />
      </div>
    </div>

    <div class="panel">
      <div class="panel-head">
        <strong>{{ tx("执行结果", "Apply Result") }}</strong>
      </div>
      <div class="panel-body">
        <textarea v-model="resultText" readonly style="min-height: 180px" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue";
import { apiRequest } from "../../lib/api";
import { useAdminI18n } from "../../lib/i18n";
import { toast } from "../../lib/toast";

const { tx } = useAdminI18n();
const configText = ref("{}");
const resultText = ref("");

async function loadTemplate() {
  try {
    const data = await apiRequest<any>("/admin/bootstrap/apply");
    configText.value = JSON.stringify(data, null, 2);
  } catch (err: any) {
    toast(`${tx("加载模板失败", "Load template failed")}: ${err.message || err}`, "err");
  }
}

async function applyConfig() {
  let body: any = {};
  try {
    body = JSON.parse(configText.value || "{}");
  } catch (err: any) {
    toast(`${tx("配置 JSON 格式错误", "Invalid config JSON")}: ${err.message || err}`, "err");
    return;
  }

  try {
    const data = await apiRequest<any>("/admin/bootstrap/apply", {
      method: "POST",
      body: JSON.stringify(body)
    });
    resultText.value = JSON.stringify(data, null, 2);
    toast(tx("配置已应用", "Config applied"));
  } catch (err: any) {
    toast(`${tx("应用配置失败", "Apply config failed")}: ${err.message || err}`, "err");
  }
}

onMounted(loadTemplate);
</script>
