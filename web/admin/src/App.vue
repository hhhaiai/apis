<template>
  <div class="app">
    <template v-if="checkingAuth">
      <div class="auth-shell">
        <section class="auth-card">
          <h1>{{ locale.loadingTitle }}</h1>
          <p class="small">{{ locale.loadingDesc }}</p>
          <div class="lang-switch">
            <button class="btn btn-outline" :class="{ active: language === 'zh' }" @click="language = 'zh'">
              中文
            </button>
            <button class="btn btn-outline" :class="{ active: language === 'en' }" @click="language = 'en'">
              EN
            </button>
          </div>
        </section>
      </div>
    </template>

    <template v-else-if="!authenticated">
      <div class="auth-shell">
        <section class="auth-card">
          <h1>{{ locale.loginTitle }}</h1>
          <p class="small">{{ locale.loginDesc }}</p>

          <div class="lang-switch">
            <button class="btn btn-outline" :class="{ active: language === 'zh' }" @click="language = 'zh'">
              中文
            </button>
            <button class="btn btn-outline" :class="{ active: language === 'en' }" @click="language = 'en'">
              EN
            </button>
          </div>

          <div v-if="defaultTokenEnabled" class="security-warning">
            {{ locale.defaultTokenWarning }}
          </div>

          <div class="form-group">
            <label>{{ locale.passwordLabel }}</label>
            <input
              v-model="passwordInput"
              type="password"
              :placeholder="locale.passwordPlaceholder"
              @keyup.enter="login"
            />
          </div>
          <div class="btn-row">
            <button class="btn" @click="login">{{ locale.loginButton }}</button>
            <button class="btn btn-outline" @click="useDefaultPassword">
              {{ locale.useDefaultButton }}
            </button>
          </div>
          <p class="small">{{ locale.loginHint }}</p>
        </section>
      </div>
    </template>

    <template v-else>
      <button class="mobile-nav-toggle btn btn-outline" @click="mobileSidebarOpen = !mobileSidebarOpen">
        {{ locale.mobileMenu }}
      </button>
      <div v-if="mobileSidebarOpen" class="sidebar-mask" @click="mobileSidebarOpen = false"></div>

      <aside class="sidebar" :class="{ 'sidebar-open': mobileSidebarOpen }">
        <div class="brand">
          <h1>{{ locale.brandTitle }}</h1>
          <p>{{ locale.brandSubtitle }}</p>
        </div>

        <div class="lang-switch side-lang">
          <button class="btn btn-outline" :class="{ active: language === 'zh' }" @click="language = 'zh'">
            中文
          </button>
          <button class="btn btn-outline" :class="{ active: language === 'en' }" @click="language = 'en'">
            EN
          </button>
        </div>

        <div class="nav-groups">
          <section v-for="group in localizedNavGroups" :key="group.key" class="nav-group">
            <p class="nav-group-title">{{ group.label }}</p>
            <button
              v-for="tab in group.items"
              :key="tab.key"
              class="nav-item"
              :class="{ active: activeTab === tab.key }"
              @click="selectTab(tab.key)"
            >
              <span class="nav-item-icon">{{ tab.icon }}</span>
              <span>{{ tab.label }}</span>
            </button>
          </section>
        </div>

        <div class="sidebar-footer">
          <button v-if="authRequired" class="btn btn-outline full-width" @click="logout">
            {{ locale.logoutButton }}
          </button>
        </div>
      </aside>

      <main class="main">
        <header class="panel topbar">
          <div class="panel-body topbar-body">
            <div>
              <h2 class="topbar-title">{{ activeTabLabel }}</h2>
              <p class="small">{{ scopeSummary }}</p>
            </div>
            <div class="topbar-controls">
              <div class="form-group compact">
                <label>{{ locale.scopeLabel }}</label>
                <select v-model="scopeDraft">
                  <option value="project">{{ locale.scopeProject }}</option>
                  <option value="global">{{ locale.scopeGlobal }}</option>
                </select>
              </div>
              <div class="form-group compact project-field">
                <label>{{ locale.projectLabel }}</label>
                <input v-model="projectDraft" :placeholder="locale.projectPlaceholder" />
              </div>
              <button class="btn btn-outline" @click="applyScope">{{ locale.applyScopeButton }}</button>
            </div>
          </div>
        </header>

        <div v-if="defaultTokenEnabled" class="security-warning">
          {{ locale.defaultTokenWarning }}
        </div>

        <component :is="activeComponent" />
      </main>

      <div v-if="toastItem" class="toast" :class="toastItem.type">
        {{ toastItem.text }}
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, provide, ref, watch } from "vue";
import {
  apiRequest,
  getStoredProjectID,
  getStoredScope,
  normalizeProjectID,
  saveStoredScope,
  type ScopeMode
} from "./lib/api";
import { ADMIN_I18N_KEY, type AdminLang } from "./lib/i18n";
import { subscribeToast, toast, type ToastItem } from "./lib/toast";

import OverviewPanel from "./components/panels/OverviewPanel.vue";
import SettingsPanel from "./components/panels/SettingsPanel.vue";
import ModelsPanel from "./components/panels/ModelsPanel.vue";
import ToolsPanel from "./components/panels/ToolsPanel.vue";
import PluginsPanel from "./components/panels/PluginsPanel.vue";
import MCPPanel from "./components/panels/MCPPanel.vue";
import BootstrapPanel from "./components/panels/BootstrapPanel.vue";
import ChannelsPanel from "./components/panels/ChannelsPanel.vue";
import AuthPanel from "./components/panels/AuthPanel.vue";
import TeamsPanel from "./components/panels/TeamsPanel.vue";
import SubagentsPanel from "./components/panels/SubagentsPanel.vue";
import EventsPanel from "./components/panels/EventsPanel.vue";
import TodosPanel from "./components/panels/TodosPanel.vue";
import PlansPanel from "./components/panels/PlansPanel.vue";
import SkillsPanel from "./components/panels/SkillsPanel.vue";
import RulesPanel from "./components/panels/RulesPanel.vue";
import CostPanel from "./components/panels/CostPanel.vue";
import EvalPanel from "./components/panels/EvalPanel.vue";
import IntelligentDispatchPanel from "./components/panels/IntelligentDispatchPanel.vue";
import SchedulerPanel from "./components/panels/SchedulerPanel.vue";

const DEFAULT_ADMIN_TOKEN = "admin123456";
const LANG_STORAGE_KEY = "cc_admin_lang";
const TOKEN_STORAGE_KEY = "cc_admin_token";
const TAB_STORAGE_KEY = "cc_admin_tab";

const tabDefs = [
  { key: "overview", icon: "OV", group: "core" },
  { key: "settings", icon: "ST", group: "core" },
  { key: "dispatch", icon: "DS", group: "core" },
  { key: "scheduler", icon: "SC", group: "core" },
  { key: "models", icon: "MD", group: "core" },
  { key: "tools", icon: "TL", group: "integration" },
  { key: "plugins", icon: "PL", group: "integration" },
  { key: "mcp", icon: "MC", group: "integration" },
  { key: "bootstrap", icon: "BT", group: "integration" },
  { key: "channels", icon: "CH", group: "governance" },
  { key: "auth", icon: "AU", group: "governance" },
  { key: "teams", icon: "TM", group: "orchestration" },
  { key: "subagents", icon: "SA", group: "orchestration" },
  { key: "events", icon: "EV", group: "orchestration" },
  { key: "todos", icon: "TD", group: "orchestration" },
  { key: "plans", icon: "PN", group: "orchestration" },
  { key: "skills", icon: "SK", group: "governance" },
  { key: "rules", icon: "RL", group: "governance" },
  { key: "cost", icon: "CT", group: "governance" },
  { key: "eval", icon: "EL", group: "governance" }
] as const;

const groupOrder = ["core", "integration", "orchestration", "governance"] as const;

type TabKey = (typeof tabDefs)[number]["key"];
type GroupKey = (typeof groupOrder)[number];
type LocalePack = {
  loadingTitle: string;
  loadingDesc: string;
  loginTitle: string;
  loginDesc: string;
  passwordLabel: string;
  passwordPlaceholder: string;
  loginButton: string;
  useDefaultButton: string;
  loginHint: string;
  logoutButton: string;
  brandTitle: string;
  brandSubtitle: string;
  defaultTokenWarning: string;
  loginSuccessToast: string;
  loginFailedToast: string;
  loginEmptyToast: string;
  logoutToast: string;
  mobileMenu: string;
  scopeLabel: string;
  projectLabel: string;
  projectPlaceholder: string;
  applyScopeButton: string;
  scopeProject: string;
  scopeGlobal: string;
  currentScope: string;
  currentProject: string;
  scopeAppliedToast: string;
  groups: Record<GroupKey, string>;
  tabs: Record<TabKey, string>;
};

const locales: Record<AdminLang, LocalePack> = {
  zh: {
    loadingTitle: "正在检查后台权限",
    loadingDesc: "系统会自动检测当前管理端是否需要密码。",
    loginTitle: "登录管理后台",
    loginDesc: "请输入后台密码（Admin Token）以继续。",
    passwordLabel: "后台密码",
    passwordPlaceholder: "输入 Admin Token",
    loginButton: "登录",
    useDefaultButton: "填入默认密码",
    loginHint: "支持命令行设置 ADMIN_TOKEN，自定义后建议立即替换默认密码。",
    logoutButton: "退出登录",
    brandTitle: "CC Gateway 管理后台",
    brandSubtitle: "统一控制平面",
    defaultTokenWarning: "检测到仍在使用默认密码 admin123456，请尽快修改 ADMIN_TOKEN。",
    loginSuccessToast: "登录成功",
    loginFailedToast: "登录失败，请检查密码",
    loginEmptyToast: "请先输入后台密码",
    logoutToast: "已退出登录",
    mobileMenu: "菜单",
    scopeLabel: "作用域",
    projectLabel: "项目",
    projectPlaceholder: "default",
    applyScopeButton: "应用范围",
    scopeProject: "项目级",
    scopeGlobal: "全局级",
    currentScope: "当前范围",
    currentProject: "当前项目",
    scopeAppliedToast: "作用域设置已生效",
    groups: {
      core: "核心配置",
      integration: "工具与插件",
      orchestration: "编排运行",
      governance: "治理与运维"
    },
    tabs: {
      overview: "总览",
      settings: "设置",
      dispatch: "智能调度",
      scheduler: "调度器",
      models: "模型",
      tools: "工具",
      plugins: "插件",
      mcp: "MCP",
      bootstrap: "配置导入",
      channels: "渠道",
      auth: "用户与令牌",
      teams: "团队",
      subagents: "子代理",
      events: "事件",
      todos: "待办",
      plans: "计划",
      skills: "技能",
      rules: "规则",
      cost: "成本",
      eval: "评估"
    }
  },
  en: {
    loadingTitle: "Checking admin access",
    loadingDesc: "The control plane is verifying whether an admin password is required.",
    loginTitle: "Admin Sign In",
    loginDesc: "Enter your admin token to access the control plane.",
    passwordLabel: "Admin password",
    passwordPlaceholder: "Enter admin token",
    loginButton: "Sign in",
    useDefaultButton: "Use default token",
    loginHint: "Set ADMIN_TOKEN from CLI/env. Replace the default token for production.",
    logoutButton: "Sign out",
    brandTitle: "CC Gateway Admin",
    brandSubtitle: "Unified control plane",
    defaultTokenWarning: "Default password admin123456 is still enabled. Change ADMIN_TOKEN as soon as possible.",
    loginSuccessToast: "Login success",
    loginFailedToast: "Login failed, please check the password",
    loginEmptyToast: "Please enter the admin password",
    logoutToast: "Signed out",
    mobileMenu: "Menu",
    scopeLabel: "Scope",
    projectLabel: "Project",
    projectPlaceholder: "default",
    applyScopeButton: "Apply Scope",
    scopeProject: "Project",
    scopeGlobal: "Global",
    currentScope: "Current scope",
    currentProject: "Current project",
    scopeAppliedToast: "Scope setting applied",
    groups: {
      core: "Core",
      integration: "Tools & Plugins",
      orchestration: "Orchestration",
      governance: "Governance"
    },
    tabs: {
      overview: "Overview",
      settings: "Settings",
      dispatch: "Dispatch",
      scheduler: "Scheduler",
      models: "Models",
      tools: "Tools",
      plugins: "Plugins",
      mcp: "MCP",
      bootstrap: "Bootstrap",
      channels: "Channels",
      auth: "Users & Tokens",
      teams: "Teams",
      subagents: "Subagents",
      events: "Events",
      todos: "Todos",
      plans: "Plans",
      skills: "Skills",
      rules: "Rules",
      cost: "Cost",
      eval: "Eval"
    }
  }
};

type AdminAuthStatus = {
  auth_required?: boolean;
  default_token_enabled?: boolean;
};

function detectLanguage(): AdminLang {
  const stored = (localStorage.getItem(LANG_STORAGE_KEY) || "").toLowerCase();
  if (stored === "zh" || stored === "en") {
    return stored;
  }
  const browserLang = (navigator.language || "").toLowerCase();
  if (browserLang.startsWith("zh")) {
    return "zh";
  }
  return "en";
}

function detectTab(): TabKey {
  const stored = (localStorage.getItem(TAB_STORAGE_KEY) || "").trim() as TabKey;
  if (tabDefs.some((item) => item.key === stored)) {
    return stored;
  }
  return "overview";
}

const language = ref<AdminLang>(detectLanguage());
watch(language, (next) => {
  localStorage.setItem(LANG_STORAGE_KEY, next);
});
const tx = (zh: string, en: string): string => (language.value === "zh" ? zh : en);
provide(ADMIN_I18N_KEY, { language, tx });

const locale = computed(() => locales[language.value]);
const activeTab = ref<TabKey>(detectTab());
watch(activeTab, (next) => {
  localStorage.setItem(TAB_STORAGE_KEY, next);
});

const componentMap: Record<TabKey, any> = {
  overview: OverviewPanel,
  settings: SettingsPanel,
  dispatch: IntelligentDispatchPanel,
  scheduler: SchedulerPanel,
  models: ModelsPanel,
  tools: ToolsPanel,
  plugins: PluginsPanel,
  mcp: MCPPanel,
  bootstrap: BootstrapPanel,
  channels: ChannelsPanel,
  auth: AuthPanel,
  teams: TeamsPanel,
  subagents: SubagentsPanel,
  events: EventsPanel,
  todos: TodosPanel,
  plans: PlansPanel,
  skills: SkillsPanel,
  rules: RulesPanel,
  cost: CostPanel,
  eval: EvalPanel
};

const localizedNavGroups = computed(() =>
  groupOrder.map((group) => ({
    key: group,
    label: locale.value.groups[group],
    items: tabDefs
      .filter((tab) => tab.group === group)
      .map((tab) => ({
        key: tab.key,
        icon: tab.icon,
        label: locale.value.tabs[tab.key]
      }))
  }))
);

const activeComponent = computed(() => componentMap[activeTab.value] || OverviewPanel);
const activeTabLabel = computed(() => locale.value.tabs[activeTab.value]);

const toastItem = ref<ToastItem | null>(null);
const unsubscribe = subscribeToast((item) => {
  toastItem.value = item;
});

const checkingAuth = ref(true);
const authenticated = ref(false);
const passwordInput = ref("");
const authStatus = ref<AdminAuthStatus>({});
const mobileSidebarOpen = ref(false);

const scopeDraft = ref<ScopeMode>(getStoredScope());
const projectDraft = ref<string>(getStoredProjectID());

const authRequired = computed(() => Boolean(authStatus.value.auth_required));
const defaultTokenEnabled = computed(() => Boolean(authStatus.value.default_token_enabled));

const scopeSummary = computed(() => {
  const scopeLabel = scopeDraft.value === "global" ? locale.value.scopeGlobal : locale.value.scopeProject;
  const projectValue = normalizeProjectID(projectDraft.value);
  return `${locale.value.currentScope}: ${scopeLabel} · ${locale.value.currentProject}: ${projectValue}`;
});

function applyScope(): void {
  const saved = saveStoredScope(scopeDraft.value, projectDraft.value);
  scopeDraft.value = saved.scope;
  projectDraft.value = saved.projectID;
  toast(locale.value.scopeAppliedToast, "ok");
}

function selectTab(tab: TabKey): void {
  activeTab.value = tab;
  mobileSidebarOpen.value = false;
}

async function verifyToken(token: string): Promise<boolean> {
  const value = token.trim();
  if (!value) {
    return false;
  }
  try {
    await apiRequest("/admin/status", {
      method: "GET",
      headers: {
        "x-admin-token": value,
        Authorization: `Bearer ${value}`
      }
    });
    return true;
  } catch {
    return false;
  }
}

async function initAuth(): Promise<void> {
  checkingAuth.value = true;
  try {
    authStatus.value = await apiRequest<AdminAuthStatus>("/admin/auth/status", {
      method: "GET"
    });
  } catch {
    authStatus.value = { auth_required: true };
  }

  if (!authRequired.value) {
    authenticated.value = true;
    checkingAuth.value = false;
    return;
  }

  const cached = (localStorage.getItem(TOKEN_STORAGE_KEY) || "").trim();
  if (!cached) {
    authenticated.value = false;
    checkingAuth.value = false;
    return;
  }

  const ok = await verifyToken(cached);
  if (!ok) {
    localStorage.removeItem(TOKEN_STORAGE_KEY);
    authenticated.value = false;
    checkingAuth.value = false;
    return;
  }

  authenticated.value = true;
  checkingAuth.value = false;
}

async function login(): Promise<void> {
  const token = passwordInput.value.trim();
  if (!token) {
    toast(locale.value.loginEmptyToast, "err");
    return;
  }
  const ok = await verifyToken(token);
  if (!ok) {
    toast(locale.value.loginFailedToast, "err");
    return;
  }
  localStorage.setItem(TOKEN_STORAGE_KEY, token);
  passwordInput.value = "";
  authenticated.value = true;
  toast(locale.value.loginSuccessToast, "ok");
}

function logout(): void {
  localStorage.removeItem(TOKEN_STORAGE_KEY);
  passwordInput.value = "";
  authenticated.value = !authRequired.value;
  toast(locale.value.logoutToast, "ok");
}

function useDefaultPassword(): void {
  passwordInput.value = DEFAULT_ADMIN_TOKEN;
}

onMounted(() => {
  void initAuth();
});

onBeforeUnmount(() => {
  unsubscribe();
});
</script>
