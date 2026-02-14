<template>
  <div>
    <div class="section-header">
      <h2>{{ tx("运行时设置", "Runtime Settings") }}</h2>
      <button class="btn" @click="save">{{ tx("保存", "Save") }}</button>
    </div>

    <div class="panel">
      <div class="panel-body">
        <div class="grid-4">
          <div class="form-group">
            <label>{{ tx("反思轮次", "Reflection Passes") }}</label>
            <input v-model.number="form.reflectionPasses" type="number" min="0" max="8" />
          </div>
          <div class="form-group">
            <label>{{ tx("超时时间 (ms)", "Timeout (ms)") }}</label>
            <input v-model.number="form.timeoutMS" type="number" min="1000" max="180000" />
          </div>
          <div class="form-group">
            <label>{{ tx("并行候选数", "Parallel Candidates") }}</label>
            <input v-model.number="form.parallelCandidates" type="number" min="1" max="20" />
          </div>
          <div class="form-group">
            <label>{{ tx("重试次数", "Retries") }}</label>
            <input v-model.number="form.retries" type="number" min="0" max="8" />
          </div>
        </div>

        <div class="grid-4">
          <div class="form-group">
            <label>{{ tx("工具循环模式", "Tool Loop Mode") }}</label>
            <select v-model="form.toolMode">
              <option value="client_loop">client_loop</option>
              <option value="server_loop">server_loop</option>
            </select>
          </div>
          <div class="form-group">
            <label>{{ tx("工具循环最大步数", "Tool Loop Max Steps") }}</label>
            <input v-model.number="form.toolMaxSteps" type="number" min="1" max="64" />
          </div>
          <div class="form-group">
            <label>{{ tx("启用响应裁判", "Enable Response Judge") }}</label>
            <select v-model="form.enableJudge">
              <option :value="true">true</option>
              <option :value="false">false</option>
            </select>
          </div>
          <div class="form-group">
            <label>{{ tx("允许实验性工具", "Allow Experimental Tools") }}</label>
            <select v-model="form.allowExperimentalTools">
              <option :value="true">true</option>
              <option :value="false">false</option>
            </select>
          </div>
        </div>

        <div class="form-group">
          <label>{{ tx("模式模型 (JSON)", "Mode Models (JSON)") }}</label>
          <textarea v-model="form.modeModelsJSON" />
        </div>
        <div class="form-group">
          <label>{{ tx("提示词前缀 (JSON)", "Prompt Prefixes (JSON)") }}</label>
          <textarea v-model="form.promptPrefixesJSON" />
        </div>
        <div class="form-group">
          <label>{{ tx("模式路由 (JSON)", "Mode Routes (JSON)") }}</label>
          <textarea v-model="form.modeRoutesJSON" />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive } from "vue";
import { apiRequest } from "../../lib/api";
import { useAdminI18n } from "../../lib/i18n";
import { toast } from "../../lib/toast";

const { tx } = useAdminI18n();
const form = reactive({
  reflectionPasses: 1,
  timeoutMS: 30000,
  parallelCandidates: 1,
  retries: 1,
  toolMode: "client_loop",
  toolMaxSteps: 4,
  enableJudge: false,
  allowExperimentalTools: false,
  modeModelsJSON: "{}",
  promptPrefixesJSON: "{}",
  modeRoutesJSON: "{}"
});

function parseObjectJSON(raw: string, label: string): Record<string, any> {
  const out = JSON.parse(raw || "{}");
  if (typeof out !== "object" || out === null || Array.isArray(out)) {
    throw new Error(tx(`${label} 必须是 JSON 对象`, `${label} must be a JSON object`));
  }
  return out;
}

async function load() {
  try {
    const settings = await apiRequest<any>("/admin/settings");
    form.reflectionPasses = settings?.routing?.reflection_passes ?? 1;
    form.timeoutMS = settings?.routing?.timeout_ms ?? 30000;
    form.parallelCandidates = settings?.routing?.parallel_candidates ?? 1;
    form.retries = settings?.routing?.retries ?? 1;
    form.toolMode = settings?.tool_loop?.mode ?? "client_loop";
    form.toolMaxSteps = settings?.tool_loop?.max_steps ?? 4;
    form.enableJudge = !!settings?.routing?.enable_response_judge;
    form.allowExperimentalTools = !!settings?.allow_experimental_tools;
    form.modeModelsJSON = JSON.stringify(settings?.mode_models || {}, null, 2);
    form.promptPrefixesJSON = JSON.stringify(settings?.prompt_prefixes || {}, null, 2);
    form.modeRoutesJSON = JSON.stringify(settings?.routing?.mode_routes || {}, null, 2);
  } catch (err: any) {
    toast(`${tx("加载设置失败", "Load settings failed")}: ${err.message || err}`, "err");
  }
}

async function save() {
  try {
    const modeModels = parseObjectJSON(form.modeModelsJSON, "mode_models");
    const promptPrefixes = parseObjectJSON(form.promptPrefixesJSON, "prompt_prefixes");
    const modeRoutes = parseObjectJSON(form.modeRoutesJSON, "mode_routes");
    const body = {
      use_mode_model_override: Object.keys(modeModels).length > 0,
      mode_models: modeModels,
      prompt_prefixes: promptPrefixes,
      allow_experimental_tools: form.allowExperimentalTools,
      allow_unknown_tools: true,
      routing: {
        retries: form.retries,
        reflection_passes: form.reflectionPasses,
        timeout_ms: form.timeoutMS,
        parallel_candidates: form.parallelCandidates,
        enable_response_judge: form.enableJudge,
        mode_routes: modeRoutes
      },
      tool_loop: {
        mode: form.toolMode,
        max_steps: form.toolMaxSteps
      }
    };
    await apiRequest("/admin/settings", { method: "PUT", body: JSON.stringify(body) });
    toast(tx("设置已保存", "Settings saved"));
  } catch (err: any) {
    toast(`${tx("保存设置失败", "Save settings failed")}: ${err.message || err}`, "err");
  }
}

onMounted(load);
</script>
