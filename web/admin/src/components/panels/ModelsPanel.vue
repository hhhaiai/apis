<template>
  <div>
    <div class="section-header">
      <h2>{{ tx("模型与路由", "Models & Routing") }}</h2>
      <div class="btn-row">
        <button class="btn btn-outline" @click="loadAll">{{ tx("刷新", "Refresh") }}</button>
      </div>
    </div>

    <div class="card-grid">
      <div v-for="item in modeItems" :key="item.mode" class="card">
        <div class="label">{{ item.mode }}</div>
        <div class="value mono" style="font-size: 14px">{{ item.model }}</div>
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

    <div class="panel">
      <div class="panel-head">
        <strong>{{ tx("模型映射", "Model Mapping") }}</strong>
      </div>
      <div class="panel-body">
        <div class="grid-4">
          <div class="form-group">
            <label>{{ tx("严格模式", "Strict Mode") }}</label>
            <select v-model="modelMapStrict">
              <option :value="true">true</option>
              <option :value="false">false</option>
            </select>
          </div>
          <div class="form-group">
            <label>{{ tx("兜底模型", "Fallback Model") }}</label>
            <input v-model="modelMapFallback" placeholder="gpt-4o-mini" />
          </div>
        </div>
        <div class="form-group">
          <label>{{ tx("映射表 (JSON)", "Mappings (JSON)") }}</label>
          <textarea v-model="modelMappingsJSON" style="min-height: 160px" />
        </div>
        <div class="btn-row">
          <button class="btn" @click="saveModelMappings">{{ tx("保存映射", "Save Mapping") }}</button>
        </div>
      </div>
    </div>

    <div class="panel">
      <div class="panel-head">
        <strong>{{ tx("上游适配器配置", "Upstream Adapters") }}</strong>
        <div class="btn-row">
          <button class="btn btn-outline" @click="loadUpstream">{{ tx("刷新", "Refresh") }}</button>
          <button class="btn" @click="saveUpstream">{{ tx("应用", "Apply") }}</button>
        </div>
      </div>
      <div class="panel-body">
        <textarea v-model="upstreamJSON" style="min-height: 250px" />
      </div>
    </div>

    <div class="panel">
      <div class="panel-head">
        <strong>{{ tx("快速添加适配器", "Quick Add Adapter") }}</strong>
      </div>
      <div class="panel-body">
        <div class="grid-4">
          <div class="form-group">
            <label>{{ tx("名称", "Name") }}</label>
            <input v-model="quick.name" placeholder="openai-main" />
          </div>
          <div class="form-group">
            <label>{{ tx("类型", "Kind") }}</label>
            <select v-model="quick.kind">
              <option value="openai">openai</option>
              <option value="anthropic">anthropic</option>
              <option value="gemini">gemini</option>
              <option value="canonical">canonical</option>
              <option value="script">script</option>
            </select>
          </div>
          <div class="form-group">
            <label>{{ tx("模型", "Model") }}</label>
            <input v-model="quick.model" placeholder="gpt-4o" />
          </div>
          <div class="form-group">
            <label>Base URL</label>
            <input v-model="quick.baseURL" placeholder="https://api.openai.com/v1" />
          </div>
          <div class="form-group">
            <label>API Key Env</label>
            <input v-model="quick.apiKeyEnv" placeholder="OPENAI_API_KEY" />
          </div>
          <div class="form-group">
            <label>{{ tx("支持工具", "Supports Tools") }}</label>
            <select v-model="quick.supportsTools">
              <option value="">unknown</option>
              <option value="true">true</option>
              <option value="false">false</option>
            </select>
          </div>
          <div class="form-group">
            <label>{{ tx("支持视觉", "Supports Vision") }}</label>
            <select v-model="quick.supportsVision">
              <option value="">unknown</option>
              <option value="true">true</option>
              <option value="false">false</option>
            </select>
          </div>
          <div class="form-group">
            <label>{{ tx("脚本命令", "Script Command") }}</label>
            <input v-model="quick.command" placeholder="python3" />
          </div>
          <div class="form-group" style="grid-column: span 4">
            <label>{{ tx("脚本参数 (JSON 数组)", "Script Args (JSON array)") }}</label>
            <input v-model="quick.args" placeholder='["adapter.py"]' />
          </div>
        </div>
        <div class="btn-row">
          <button class="btn btn-outline" @click="addQuickAdapter">{{ tx("添加到 JSON", "Add To JSON") }}</button>
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
const modeItems = ref<Array<{ mode: string; model: string }>>([]);
const routeItems = ref<Array<{ mode: string; chain: string }>>([]);

const modelMappingsJSON = ref("{}");
const modelMapStrict = ref(false);
const modelMapFallback = ref("");

const upstreamJSON = ref("{}");

const quick = reactive({
  name: "",
  kind: "openai",
  model: "",
  baseURL: "",
  apiKeyEnv: "",
  supportsTools: "",
  supportsVision: "",
  command: "",
  args: "[]"
});

function parseObjectJSON(raw: string, label: string): Record<string, any> {
  const parsed = JSON.parse(raw || "{}");
  if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
    throw new Error(tx(`${label} 必须是 JSON 对象`, `${label} must be a JSON object`));
  }
  return parsed;
}

function parseOptionalBool(raw: string): boolean | undefined {
  if (raw === "true") return true;
  if (raw === "false") return false;
  return undefined;
}

function parseJSONArray(raw: string): any[] {
  const parsed = JSON.parse(raw || "[]");
  if (!Array.isArray(parsed)) {
    throw new Error(tx("args 必须是 JSON 数组", "args must be JSON array"));
  }
  return parsed;
}

async function loadSettingsView() {
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
}

async function loadModelMappings() {
  const data = await apiRequest<any>("/admin/model-mapping");
  modelMappingsJSON.value = JSON.stringify(data?.model_mappings || {}, null, 2);
  modelMapStrict.value = Boolean(data?.model_map_strict);
  modelMapFallback.value = String(data?.model_map_fallback || "");
}

async function saveModelMappings() {
  try {
    const payload = {
      model_mappings: parseObjectJSON(modelMappingsJSON.value, "model_mappings"),
      model_map_strict: Boolean(modelMapStrict.value),
      model_map_fallback: modelMapFallback.value.trim()
    };
    await apiRequest("/admin/model-mapping", {
      method: "PUT",
      body: JSON.stringify(payload)
    });
    toast(tx("模型映射已保存", "Model mappings saved"));
  } catch (err: any) {
    toast(`${tx("保存模型映射失败", "Save model mappings failed")}: ${err.message || err}`, "err");
  }
}

async function loadUpstream() {
  try {
    const data = await apiRequest<any>("/admin/upstream");
    upstreamJSON.value = JSON.stringify(data, null, 2);
  } catch (err: any) {
    toast(`${tx("加载上游配置失败", "Load upstream config failed")}: ${err.message || err}`, "err");
  }
}

async function saveUpstream() {
  try {
    const payload = parseObjectJSON(upstreamJSON.value, "upstream");
    const data = await apiRequest<any>("/admin/upstream", {
      method: "PUT",
      body: JSON.stringify(payload)
    });
    upstreamJSON.value = JSON.stringify(data, null, 2);
    toast(tx("上游配置已应用", "Upstream config applied"));
  } catch (err: any) {
    toast(`${tx("应用上游配置失败", "Apply upstream config failed")}: ${err.message || err}`, "err");
  }
}

function addQuickAdapter() {
  try {
    const raw = parseObjectJSON(upstreamJSON.value, "upstream");
    if (!Array.isArray(raw.adapters)) {
      raw.adapters = [];
    }

    if (!quick.name.trim()) {
      throw new Error(tx("适配器名称不能为空", "adapter name is required"));
    }

    const spec: Record<string, any> = {
      name: quick.name.trim(),
      kind: quick.kind,
      model: quick.model.trim(),
      base_url: quick.baseURL.trim(),
      api_key_env: quick.apiKeyEnv.trim()
    };

    const supportsTools = parseOptionalBool(quick.supportsTools);
    const supportsVision = parseOptionalBool(quick.supportsVision);
    if (supportsTools !== undefined) spec.supports_tools = supportsTools;
    if (supportsVision !== undefined) spec.supports_vision = supportsVision;

    if (quick.kind === "script") {
      spec.command = quick.command.trim();
      spec.args = parseJSONArray(quick.args);
    }

    raw.adapters.push(spec);
    upstreamJSON.value = JSON.stringify(raw, null, 2);
    toast(tx("已添加到上游 JSON", "Added to upstream JSON"));
  } catch (err: any) {
    toast(`${tx("添加适配器失败", "Add adapter failed")}: ${err.message || err}`, "err");
  }
}

async function loadAll() {
  try {
    await Promise.all([loadSettingsView(), loadModelMappings(), loadUpstream()]);
  } catch (err: any) {
    toast(`${tx("加载模型面板失败", "Load model panel failed")}: ${err.message || err}`, "err");
  }
}

onMounted(loadAll);
</script>
