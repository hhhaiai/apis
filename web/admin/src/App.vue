<template>
  <div class="app">
    <template v-if="checkingAuth">
      <div class="auth-shell">
        <section class="auth-card">
          <h1>{{ locale.loadingTitle }}</h1>
          <p class="small">{{ locale.loadingDesc }}</p>
          <div class="lang-switch">
            <button
              class="btn btn-outline"
              :class="{ active: language === 'zh' }"
              @click="language = 'zh'"
            >
              中文
            </button>
            <button
              class="btn btn-outline"
              :class="{ active: language === 'en' }"
              @click="language = 'en'"
            >
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
            <button
              class="btn btn-outline"
              :class="{ active: language === 'zh' }"
              @click="language = 'zh'"
            >
              中文
            </button>
            <button
              class="btn btn-outline"
              :class="{ active: language === 'en' }"
              @click="language = 'en'"
            >
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
      <aside class="sidebar">
        <div class="brand">
          <h1>{{ locale.brandTitle }}</h1>
          <p>{{ locale.brandSubtitle }}</p>
        </div>
        <div class="lang-switch side-lang">
          <button
            class="btn btn-outline"
            :class="{ active: language === 'zh' }"
            @click="language = 'zh'"
          >
            中文
          </button>
          <button
            class="btn btn-outline"
            :class="{ active: language === 'en' }"
            @click="language = 'en'"
          >
            EN
          </button>
        </div>
        <button
          v-for="tab in localizedTabs"
          :key="tab.key"
          class="nav-item"
          :class="{ active: activeTab === tab.key }"
          @click="activeTab = tab.key"
        >
          <span>{{ tab.icon }}</span>
          <span>{{ tab.label }}</span>
        </button>
        <div class="sidebar-footer">
          <button v-if="authRequired" class="btn btn-outline full-width" @click="logout">
            {{ locale.logoutButton }}
          </button>
        </div>
      </aside>

      <main class="main">
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
import { apiRequest } from "./lib/api";
import { ADMIN_I18N_KEY, type AdminLang } from "./lib/i18n";
import { subscribeToast, toast, type ToastItem } from "./lib/toast";

import OverviewPanel from "./components/panels/OverviewPanel.vue";
import SettingsPanel from "./components/panels/SettingsPanel.vue";
import ModelsPanel from "./components/panels/ModelsPanel.vue";
import ToolsPanel from "./components/panels/ToolsPanel.vue";
import PluginsPanel from "./components/panels/PluginsPanel.vue";
import MCPPanel from "./components/panels/MCPPanel.vue";
import TeamsPanel from "./components/panels/TeamsPanel.vue";
import SubagentsPanel from "./components/panels/SubagentsPanel.vue";
import EventsPanel from "./components/panels/EventsPanel.vue";
import TodosPanel from "./components/panels/TodosPanel.vue";
import PlansPanel from "./components/panels/PlansPanel.vue";
import SkillsPanel from "./components/panels/SkillsPanel.vue";
import RulesPanel from "./components/panels/RulesPanel.vue";
import CostPanel from "./components/panels/CostPanel.vue";
import EvalPanel from "./components/panels/EvalPanel.vue";

const DEFAULT_ADMIN_TOKEN = "admin123456";
const LANG_STORAGE_KEY = "cc_admin_lang";
const TOKEN_STORAGE_KEY = "cc_admin_token";

const tabDefs = [
  { key: "overview", icon: "📊" },
  { key: "settings", icon: "⚙️" },
  { key: "models", icon: "🤖" },
  { key: "tools", icon: "🧰" },
  { key: "plugins", icon: "🧩" },
  { key: "mcp", icon: "🔌" },
  { key: "teams", icon: "👥" },
  { key: "subagents", icon: "🤝" },
  { key: "events", icon: "📡" },
  { key: "todos", icon: "✅" },
  { key: "plans", icon: "🗺️" },
  { key: "skills", icon: "✨" },
  { key: "rules", icon: "📜" },
  { key: "cost", icon: "💰" },
  { key: "eval", icon: "🧠" }
] as const;

type TabKey = (typeof tabDefs)[number]["key"];
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
    brandSubtitle: "Vue + Vite 控制平面",
    defaultTokenWarning: "检测到仍在使用默认密码 admin123456，请尽快修改 ADMIN_TOKEN。",
    loginSuccessToast: "登录成功",
    loginFailedToast: "登录失败，请检查密码",
    loginEmptyToast: "请先输入后台密码",
    logoutToast: "已退出登录",
    tabs: {
      overview: "总览",
      settings: "设置",
      models: "模型",
      tools: "工具",
      plugins: "插件",
      mcp: "MCP",
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
    brandSubtitle: "Vue + Vite Control Plane",
    defaultTokenWarning: "Default password admin123456 is still enabled. Change ADMIN_TOKEN as soon as possible.",
    loginSuccessToast: "Login success",
    loginFailedToast: "Login failed, please check the password",
    loginEmptyToast: "Please enter the admin password",
    logoutToast: "Signed out",
    tabs: {
      overview: "Overview",
      settings: "Settings",
      models: "Models",
      tools: "Tools",
      plugins: "Plugins",
      mcp: "MCP",
      teams: "Agent Teams",
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

const language = ref<AdminLang>(detectLanguage());
watch(language, (next) => {
  localStorage.setItem(LANG_STORAGE_KEY, next);
});
const tx = (zh: string, en: string): string => (language.value === "zh" ? zh : en);
provide(ADMIN_I18N_KEY, { language, tx });

const locale = computed(() => locales[language.value]);
const activeTab = ref<TabKey>("overview");

const componentMap: Record<TabKey, any> = {
  overview: OverviewPanel,
  settings: SettingsPanel,
  models: ModelsPanel,
  tools: ToolsPanel,
  plugins: PluginsPanel,
  mcp: MCPPanel,
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

const localizedTabs = computed(() =>
  tabDefs.map((tab) => ({
    key: tab.key,
    icon: tab.icon,
    label: locale.value.tabs[tab.key]
  }))
);

const activeComponent = computed(() => componentMap[activeTab.value] || OverviewPanel);

const toastItem = ref<ToastItem | null>(null);
const unsubscribe = subscribeToast((item) => {
  toastItem.value = item;
});

const checkingAuth = ref(true);
const authenticated = ref(false);
const passwordInput = ref("");
const authStatus = ref<AdminAuthStatus>({});

const authRequired = computed(() => Boolean(authStatus.value.auth_required));
const defaultTokenEnabled = computed(() => Boolean(authStatus.value.default_token_enabled));

async function verifyToken(token: string): Promise<boolean> {
  const value = token.trim();
  if (!value) {
    return false;
  }
  try {
    await apiRequest("/admin/status", {
      method: "GET",
      headers: {
        "x-admin-token": value
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
