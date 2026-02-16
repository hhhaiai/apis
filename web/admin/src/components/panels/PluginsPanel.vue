<template>
  <div>
    <div class="section-header">
      <h2>{{ tx("插件中心", "Plugin Center") }}</h2>
      <button class="btn btn-outline" @click="loadAll">{{ tx("刷新", "Refresh") }}</button>
    </div>

    <div class="panel">
      <div class="panel-head">
        <strong>{{ tx("作用域状态", "Scope State") }}</strong>
      </div>
      <div class="panel-body">
        <p class="small">
          {{ tx("当前插件作用域", "Current plugin scope") }}:
          <span class="mono">{{ scopeLabel }}</span>
          ·
          {{ tx("项目", "Project") }}:
          <span class="mono">{{ projectID }}</span>
        </p>
      </div>
    </div>

    <div class="panel">
      <div class="panel-head">
        <strong>{{ tx("手动安装", "Manual Install") }}</strong>
      </div>
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
          <button class="btn" @click="installManual">{{ tx("安装插件", "Install Plugin") }}</button>
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
        <strong>{{ tx("云市场安装", "Cloud Marketplace Install") }}</strong>
        <div class="btn-row">
          <button class="btn btn-outline" @click="loadMarketplace">{{ tx("拉取列表", "Load List") }}</button>
          <button class="btn" @click="installChecked">{{ tx("安装勾选项", "Install Checked") }}</button>
        </div>
      </div>
      <div class="panel-body">
        <div class="grid-2">
          <div class="form-group">
            <label>{{ tx("云清单 URL (可选)", "Cloud Manifest URL (optional)") }}</label>
            <input v-model="cloudSourceURL" placeholder="https://example.com/plugins.json" />
          </div>
          <div class="form-group">
            <label>{{ tx("筛选", "Filter") }}</label>
            <input v-model="marketFilter" :placeholder="tx('按名称过滤', 'Filter by name')" />
          </div>
        </div>

        <table>
          <thead>
            <tr>
              <th style="width: 70px">{{ tx("选择", "Select") }}</th>
              <th>{{ tx("名称", "Name") }}</th>
              <th>{{ tx("版本", "Version") }}</th>
              <th>{{ tx("来源", "Source") }}</th>
              <th>{{ tx("可信", "Verified") }}</th>
              <th>{{ tx("描述", "Description") }}</th>
              <th>{{ tx("状态", "State") }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="filteredMarketplace.length === 0">
              <td colspan="7" class="small">{{ tx("暂无可安装插件", "No marketplace plugins") }}</td>
            </tr>
            <tr v-for="item in filteredMarketplace" :key="item.name">
              <td>
                <input type="checkbox" :checked="checkedNames.has(item.name)" @change="toggleChecked(item.name)" />
              </td>
              <td class="mono">{{ item.name }}</td>
              <td class="mono">{{ item.version || "—" }}</td>
              <td class="mono">{{ item.source || "—" }}</td>
              <td>
                <span class="badge" :class="item.verified ? 'badge-green' : 'badge-orange'">
                  {{ item.verified ? tx("已验证", "verified") : tx("未验证", "unverified") }}
                </span>
              </td>
              <td class="small">{{ item.description || "—" }}</td>
              <td>
                <span class="badge" :class="installedNames.has(item.name) ? 'badge-green' : 'badge-orange'">
                  {{ installedNames.has(item.name) ? tx("已安装", "installed") : tx("未安装", "not installed") }}
                </span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <div class="panel">
      <div class="panel-head">
        <strong>{{ tx("能力向导安装", "Capability Wizard Install") }}</strong>
        <div class="btn-row">
          <button class="btn btn-outline" @click="autoSelectRecommended">
            {{ tx("自动勾选推荐", "Auto Select") }}
          </button>
          <button class="btn" @click="installWizardRecommended">
            {{ tx("一键安装推荐", "Install Recommended") }}
          </button>
        </div>
      </div>
      <div class="panel-body">
        <p class="small">
          {{ tx("根据能力目标自动推荐插件，适合不熟悉插件细节的用户。", "Automatically recommend plugins based on capability goals.") }}
        </p>
        <div class="toolbar" style="margin-bottom: 10px">
          <div class="toolbar-item grow">
            <label>{{ tx("预设模板", "Preset Template") }}</label>
            <select v-model="presetID">
              <option value="">{{ tx("请选择模板", "Select a template") }}</option>
              <option v-for="preset in wizardPresets" :key="preset.id" :value="preset.id">
                {{ preset.label }}
              </option>
            </select>
          </div>
          <div class="toolbar-item">
            <label>&nbsp;</label>
            <button class="btn btn-outline" @click="applyPreset">{{ tx("应用模板", "Apply Preset") }}</button>
          </div>
        </div>
        <div class="card-grid">
          <label v-for="cap in capabilityDefs" :key="cap.id" class="card wizard-card">
            <div class="wizard-head">
              <input
                type="checkbox"
                :checked="selectedCapabilities.has(cap.id)"
                @change="toggleCapability(cap.id)"
              />
              <strong>{{ cap.label }}</strong>
            </div>
            <p class="small">{{ cap.desc }}</p>
            <p class="small">
              {{ tx("匹配插件", "Matched") }}: {{ capabilityMatches[cap.id]?.length || 0 }}
            </p>
            <div class="chip-list">
              <span
                v-for="name in (capabilityMatches[cap.id] || []).slice(0, 4)"
                :key="`${cap.id}-${name}`"
                class="chip"
              >
                {{ name }}
              </span>
            </div>
          </label>
        </div>
      </div>
    </div>

    <div class="panel">
      <div class="panel-head">
        <strong>{{ tx("插件详情", "Plugin Detail") }}</strong>
      </div>
      <div class="panel-body">
        <textarea v-model="previewText" readonly style="min-height: 160px" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
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
const scopeLabel = ref("project");
const projectID = ref("default");

const cloudSourceURL = ref("");
const marketplace = ref<any[]>([]);
const installedNames = ref<Set<string>>(new Set());
const checkedNames = ref<Set<string>>(new Set());
const marketFilter = ref("");
const selectedCapabilities = ref<Set<string>>(new Set());
const presetID = ref("");

const filteredMarketplace = computed(() => {
  const keyword = marketFilter.value.trim().toLowerCase();
  if (!keyword) return marketplace.value;
  return marketplace.value.filter((item) => String(item?.name || "").toLowerCase().includes(keyword));
});

const capabilityDefs = computed(() => [
  {
    id: "search",
    label: tx("网络搜索", "Web Search"),
    desc: tx("让模型具备实时检索能力。", "Enable real-time information retrieval."),
    tags: ["search", "web", "information"],
    keywords: ["search", "web", "检索", "信息"]
  },
  {
    id: "file_mcp",
    label: tx("文件与 MCP", "File & MCP"),
    desc: tx("让模型可操作本地文件或接入 MCP 服务。", "Enable file operations and MCP integration."),
    tags: ["file", "filesystem", "mcp", "tools"],
    keywords: ["file", "filesystem", "mcp", "文件"]
  },
  {
    id: "openai_proxy",
    label: tx("OpenAI 代理", "OpenAI Proxy"),
    desc: tx("快速接入 OpenAI 兼容模型。", "Quickly connect OpenAI-compatible models."),
    tags: ["openai", "gpt", "proxy"],
    keywords: ["openai", "gpt", "proxy"]
  },
  {
    id: "anthropic_proxy",
    label: tx("Claude 代理", "Claude Proxy"),
    desc: tx("快速接入 Anthropic Claude 模型。", "Quickly connect Anthropic Claude models."),
    tags: ["anthropic", "claude", "proxy"],
    keywords: ["anthropic", "claude", "proxy"]
  },
  {
    id: "vision",
    label: tx("图像理解", "Image Understanding"),
    desc: tx("支持图片识别、OCR、视觉理解。", "Support OCR and image understanding."),
    tags: ["image", "vision", "ocr"],
    keywords: ["image", "vision", "ocr", "识图", "图像"]
  },
  {
    id: "drawing",
    label: tx("绘图生成", "Image Generation"),
    desc: tx("支持画图、生成图片类能力。", "Support drawing and image generation."),
    tags: ["draw", "image-generation", "midjourney", "dalle"],
    keywords: ["draw", "image generation", "midjourney", "dalle", "绘图", "画图"]
  }
]);

const wizardPresets = computed(() => [
  {
    id: "assistant_basic",
    label: tx("通用助手（搜索 + 代理）", "General Assistant (Search + Proxies)"),
    capabilities: ["search", "openai_proxy", "anthropic_proxy"]
  },
  {
    id: "dev_agent",
    label: tx("开发助手（文件 + 搜索 + MCP）", "Dev Agent (Files + Search + MCP)"),
    capabilities: ["file_mcp", "search"]
  },
  {
    id: "multimodal",
    label: tx("多模态助手（视觉 + 绘图 + 搜索）", "Multimodal (Vision + Drawing + Search)"),
    capabilities: ["vision", "drawing", "search"]
  }
]);

const capabilityMatches = computed<Record<string, string[]>>(() => {
  const map: Record<string, string[]> = {};
  for (const cap of capabilityDefs.value) {
    const rows = marketplace.value.filter((item) => pluginMatchesCapability(item, cap));
    map[cap.id] = rows.map((item) => String(item?.name || "")).filter(Boolean);
  }
  return map;
});

function parseJSONArray(raw: string, label: string): any[] {
  const parsed = JSON.parse(raw || "[]");
  if (!Array.isArray(parsed)) {
    throw new Error(tx(`${label} 必须是 JSON 数组`, `${label} must be JSON array`));
  }
  return parsed;
}

function pluginMatchesCapability(plugin: any, cap: any): boolean {
  const tags = Array.isArray(plugin?.tags) ? plugin.tags.map((v: any) => String(v || "").toLowerCase()) : [];
  const text = `${String(plugin?.name || "")} ${String(plugin?.description || "")}`.toLowerCase();
  return (
    cap.tags.some((tag: string) => tags.includes(tag)) ||
    cap.keywords.some((keyword: string) => text.includes(keyword.toLowerCase()))
  );
}

async function loadPlugins() {
  const data = await apiRequest<any>("/v1/cc/plugins?limit=500");
  scopeLabel.value = String(data?.scope || "project");
  projectID.value = String(data?.project_id || "default");
  const rows = Array.isArray(data?.data) ? data.data : [];
  plugins.value = rows;
  installedNames.value = new Set(rows.map((item: any) => String(item?.name || "")).filter(Boolean));
}

async function loadMarketplace() {
  try {
    const rawURL = cloudSourceURL.value.trim();
    if (rawURL) {
      const cloud = await apiRequest<any>("/admin/marketplace/cloud/list", {
        method: "POST",
        body: JSON.stringify({ url: rawURL })
      });
      const rows = Array.isArray(cloud?.data) ? cloud.data : [];
      marketplace.value = rows.map((item: any) => ({
        ...item,
        source: String(item?.source || "cloud.marketplace"),
        verified: item?.verified === true
      }));
    } else {
      const local = await apiRequest<any>("/v1/cc/marketplace/plugins");
      const rows = Array.isArray(local?.data) ? local.data : [];
      marketplace.value = rows.map((item: any) => ({
        ...item,
        source: String(item?.source || "local.marketplace"),
        verified: item?.verified === true
      }));
    }
  } catch (err: any) {
    marketplace.value = [];
    toast(`${tx("拉取云市场失败", "Load marketplace failed")}: ${err.message || err}`, "err");
  }
}

function toggleChecked(name: string) {
  const next = new Set(checkedNames.value);
  if (next.has(name)) {
    next.delete(name);
  } else {
    next.add(name);
  }
  checkedNames.value = next;
}

async function installChecked() {
  const names = Array.from(checkedNames.value.values()).filter(Boolean);
  if (names.length === 0) {
    toast(tx("请先勾选插件", "Please select plugins first"), "err");
    return;
  }
  await installByNames(names);
}

async function installByNames(names: string[]) {
  const uniqNames = Array.from(new Set(names.filter(Boolean)));
  if (uniqNames.length === 0) {
    toast(tx("没有可安装插件", "No plugins to install"), "err");
    return;
  }
  try {
    const rawURL = cloudSourceURL.value.trim();
    if (rawURL) {
      const out = await apiRequest<any>("/admin/marketplace/cloud/install", {
        method: "POST",
        body: JSON.stringify({
          url: rawURL,
          names: uniqNames
        })
      });
      previewText.value = JSON.stringify(out, null, 2);
    } else {
      for (const name of uniqNames) {
        await apiRequest(`/v1/cc/marketplace/plugins/${encodeURIComponent(name)}/install`, {
          method: "POST",
          body: "{}"
        });
      }
    }
    checkedNames.value = new Set();
    toast(tx("插件安装已执行", "Plugin installation submitted"));
    await loadAll();
  } catch (err: any) {
    toast(`${tx("安装插件失败", "Install plugins failed")}: ${err.message || err}`, "err");
  }
}

function toggleCapability(id: string) {
  const next = new Set(selectedCapabilities.value);
  if (next.has(id)) {
    next.delete(id);
  } else {
    next.add(id);
  }
  selectedCapabilities.value = next;
}

function applyPreset() {
  const preset = wizardPresets.value.find((item) => item.id === presetID.value);
  if (!preset) {
    toast(tx("请先选择模板", "Please select a preset first"), "err");
    return;
  }
  selectedCapabilities.value = new Set(preset.capabilities);
  toast(tx("模板已应用", "Preset applied"));
}

function collectRecommendedNames(): string[] {
  const selected = Array.from(selectedCapabilities.value.values());
  const names: string[] = [];
  for (const id of selected) {
    names.push(...(capabilityMatches.value[id] || []));
  }
  return Array.from(new Set(names.filter((name) => !installedNames.value.has(name))));
}

function autoSelectRecommended() {
  const names = collectRecommendedNames();
  checkedNames.value = new Set([...checkedNames.value, ...names]);
  toast(tx(`已自动勾选 ${names.length} 个推荐插件`, `Auto-selected ${names.length} recommended plugins`));
}

async function installWizardRecommended() {
  const names = collectRecommendedNames();
  if (names.length === 0) {
    toast(tx("当前能力选择没有可安装插件", "No installable plugins for selected capabilities"), "err");
    return;
  }
  await installByNames(names);
}

async function loadAll() {
  try {
    await loadPlugins();
    await loadMarketplace();
  } catch (err: any) {
    toast(`${tx("插件面板刷新失败", "Plugin panel refresh failed")}: ${err.message || err}`, "err");
  }
}

async function installManual() {
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
    await loadPlugins();
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
    const data = await apiRequest<any>(`/v1/cc/plugins/${encodeURIComponent(name)}/${enabled ? "enable" : "disable"}`, {
      method: "POST",
      body: "{}"
    });
    previewText.value = JSON.stringify(data, null, 2);
    toast(enabled ? tx("插件已启用", "Plugin enabled") : tx("插件已禁用", "Plugin disabled"));
    await loadPlugins();
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
    await loadPlugins();
  } catch (err: any) {
    toast(`${tx("删除插件失败", "Delete plugin failed")}: ${err.message || err}`, "err");
  }
}

onMounted(loadAll);
</script>
