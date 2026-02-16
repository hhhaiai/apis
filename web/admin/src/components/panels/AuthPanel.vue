<template>
  <div>
    <div class="section-header">
      <h2>{{ tx("用户与令牌", "Users & Tokens") }}</h2>
      <div class="btn-row">
        <button class="btn btn-outline" @click="loadUsers">{{ tx("刷新", "Refresh") }}</button>
      </div>
    </div>

    <div class="panel">
      <div class="panel-head">
        <strong>{{ tx("用户检索", "User Search") }}</strong>
      </div>
      <div class="panel-body">
        <div class="toolbar">
          <div class="toolbar-item grow">
            <label>{{ tx("关键词", "Keyword") }}</label>
            <input
              v-model="query.search"
              :placeholder="tx('用户名/邮箱/显示名', 'username/email/display name')"
              @keyup.enter="searchUsers"
            />
          </div>
          <div class="toolbar-item">
            <label>{{ tx("每页", "Page Size") }}</label>
            <select v-model.number="query.limit" @change="searchUsers">
              <option :value="10">10</option>
              <option :value="20">20</option>
              <option :value="50">50</option>
            </select>
          </div>
          <div class="toolbar-item">
            <label>&nbsp;</label>
            <button class="btn btn-outline" @click="searchUsers">{{ tx("搜索", "Search") }}</button>
          </div>
        </div>
        <p class="small">
          {{ tx("共", "Total") }} {{ totalUsers }} {{ tx("个用户", "users") }}
          · {{ tx("当前页", "Page") }} {{ query.page }}
        </p>
      </div>
    </div>

    <div class="panel">
      <div class="panel-head">
        <strong>{{ tx("创建用户", "Create User") }}</strong>
      </div>
      <div class="panel-body">
        <div class="grid-4">
          <div class="form-group">
            <label>{{ tx("用户名", "Username") }}</label>
            <input v-model="createForm.username" placeholder="user01" />
          </div>
          <div class="form-group">
            <label>{{ tx("密码", "Password") }}</label>
            <input v-model="createForm.password" type="password" placeholder="******" />
          </div>
          <div class="form-group">
            <label>Email</label>
            <input v-model="createForm.email" placeholder="user01@example.com" />
          </div>
          <div class="form-group">
            <label>{{ tx("角色", "Role") }}</label>
            <select v-model="createForm.role">
              <option value="user">user</option>
              <option value="admin">admin</option>
            </select>
          </div>
          <div class="form-group">
            <label>{{ tx("分组", "Group") }}</label>
            <input v-model="createForm.group" placeholder="default" />
          </div>
          <div class="form-group">
            <label>{{ tx("初始额度", "Initial Quota") }}</label>
            <input v-model.number="createForm.quota" type="number" min="0" />
          </div>
        </div>
        <div class="btn-row">
          <button class="btn" @click="createUser">{{ tx("创建", "Create") }}</button>
        </div>
      </div>
    </div>

    <div class="panel">
      <div class="panel-head">
        <strong>{{ tx("用户列表", "User List") }}</strong>
        <div class="btn-row">
          <button class="btn btn-outline" @click="batchSetUserStatus(1)">
            {{ tx("批量启用", "Batch Enable") }}
          </button>
          <button class="btn btn-outline" @click="batchSetUserStatus(2)">
            {{ tx("批量禁用", "Batch Disable") }}
          </button>
          <button class="btn btn-danger" @click="batchDeleteUsers">
            {{ tx("批量删除", "Batch Delete") }}
          </button>
        </div>
      </div>
      <table>
        <thead>
          <tr>
            <th style="width: 44px">
              <input type="checkbox" :checked="allUsersCheckedOnPage" @change="toggleAllUsersOnPage" />
            </th>
            <th>ID</th>
            <th>{{ tx("用户名", "Username") }}</th>
            <th>{{ tx("角色", "Role") }}</th>
            <th>{{ tx("分组", "Group") }}</th>
            <th>{{ tx("额度", "Quota") }}</th>
            <th>{{ tx("状态", "Status") }}</th>
            <th>{{ tx("操作", "Actions") }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="users.length === 0">
            <td colspan="8" class="small">{{ tx("暂无用户", "No users") }}</td>
          </tr>
          <tr v-for="user in users" :key="user.id">
            <td>
              <input
                type="checkbox"
                :checked="selectedUserIDs.has(String(user.id))"
                @change="toggleUserCheck(String(user.id))"
              />
            </td>
            <td class="mono">{{ user.id }}</td>
            <td>{{ user.username || "—" }}</td>
            <td>{{ user.role || "user" }}</td>
            <td class="mono">{{ user.group || "default" }}</td>
            <td>{{ Number(user.quota || 0).toLocaleString() }}</td>
            <td>
              <span class="badge" :class="Number(user.status) === 1 ? 'badge-green' : 'badge-red'">
                {{ Number(user.status) === 1 ? tx("启用", "enabled") : tx("禁用", "disabled") }}
              </span>
            </td>
            <td>
              <div class="btn-row">
                <button class="btn btn-outline" @click="selectUser(user.id)">{{ tx("选择", "Select") }}</button>
                <button class="btn btn-outline" @click="openUserEdit(user)">{{ tx("编辑", "Edit") }}</button>
                <button class="btn btn-outline" @click="showUser(user.id)">{{ tx("详情", "Detail") }}</button>
                <button class="btn btn-danger" @click="removeUser(user.id)">{{ tx("删除", "Delete") }}</button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
      <div class="panel-body">
        <div class="pagination">
          <button class="btn btn-outline" :disabled="query.page <= 1" @click="changeUserPage(query.page - 1)">
            {{ tx("上一页", "Prev") }}
          </button>
          <span class="small">{{ query.page }} / {{ totalPages }}</span>
          <button class="btn btn-outline" :disabled="query.page >= totalPages" @click="changeUserPage(query.page + 1)">
            {{ tx("下一页", "Next") }}
          </button>
        </div>
      </div>
    </div>

    <div class="panel">
      <div class="panel-head">
        <strong>{{ tx("令牌管理", "Token Management") }}</strong>
      </div>
      <div class="panel-body">
        <p class="small">
          {{ tx("当前用户", "Current user") }}:
          <span class="mono">{{ selectedUserID || tx("未选择", "none") }}</span>
          <template v-if="quotaSnapshot">
            · {{ tx("剩余额度", "Remaining") }}:
            <span class="mono">{{ Number(quotaSnapshot.remaining || 0).toLocaleString() }}</span>
          </template>
        </p>
        <div class="grid-4">
          <div class="form-group">
            <label>{{ tx("额度增减", "Quota Delta") }}</label>
            <input v-model.number="quotaDelta" type="number" />
          </div>
          <div class="form-group">
            <label>{{ tx("令牌名称", "Token Name") }}</label>
            <input v-model="tokenForm.name" placeholder="sdk-token" />
          </div>
          <div class="form-group">
            <label>{{ tx("令牌额度", "Token Quota") }}</label>
            <input v-model.number="tokenForm.quota" type="number" min="0" />
          </div>
          <div class="form-group">
            <label>{{ tx("过期时间", "Expire At (Unix)") }}</label>
            <input v-model.number="tokenForm.expiredAt" type="number" />
          </div>
          <div class="form-group">
            <label>{{ tx("模型限制", "Models") }}</label>
            <input v-model="tokenForm.models" placeholder="gpt-4o,claude-3.5-sonnet" />
          </div>
          <div class="form-group">
            <label>{{ tx("子网限制", "Subnet") }}</label>
            <input v-model="tokenForm.subnet" placeholder="192.168.1.0/24" />
          </div>
        </div>
        <div class="btn-row">
          <button class="btn btn-outline" @click="loadTokens">{{ tx("加载令牌", "Load Tokens") }}</button>
          <button class="btn btn-outline" @click="addQuota">{{ tx("调整额度", "Adjust Quota") }}</button>
          <button class="btn" @click="createToken">{{ tx("创建令牌", "Create Token") }}</button>
        </div>
      </div>
    </div>

    <div class="panel">
      <table>
        <thead>
          <tr>
            <th>ID</th>
            <th>{{ tx("名称", "Name") }}</th>
            <th>{{ tx("状态", "Status") }}</th>
            <th>{{ tx("额度", "Quota") }}</th>
            <th>{{ tx("已用", "Used") }}</th>
            <th>{{ tx("过期", "Expire") }}</th>
            <th>{{ tx("操作", "Actions") }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="tokens.length === 0">
            <td colspan="7" class="small">{{ tx("暂无令牌", "No tokens") }}</td>
          </tr>
          <tr v-for="item in tokens" :key="item.id">
            <td class="mono">{{ item.id }}</td>
            <td>{{ item.name || "—" }}</td>
            <td>{{ Number(item.status) === 1 ? tx("启用", "enabled") : tx("禁用", "disabled") }}</td>
            <td>{{ Number(item.quota || 0).toLocaleString() }}</td>
            <td>{{ Number(item.used_quota || 0).toLocaleString() }}</td>
            <td class="mono">{{ item.expired_at || "—" }}</td>
            <td>
              <div class="btn-row">
                <button class="btn btn-outline" @click="showToken(item.id)">{{ tx("详情", "Detail") }}</button>
                <button class="btn btn-outline" @click="openTokenEdit(item)">{{ tx("编辑", "Edit") }}</button>
                <button class="btn btn-danger" @click="deleteToken(item.id)">{{ tx("删除", "Delete") }}</button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="panel">
      <div class="panel-head">
        <strong>{{ tx("详情预览", "Detail Preview") }}</strong>
      </div>
      <div class="panel-body">
        <textarea v-model="previewText" readonly style="min-height: 160px" />
      </div>
    </div>

    <div v-if="editUserOpen" class="modal-mask" @click.self="editUserOpen = false">
      <div class="modal">
        <div class="modal-head">
          <strong>{{ tx("编辑用户", "Edit User") }}</strong>
          <button class="btn btn-outline" @click="editUserOpen = false">{{ tx("关闭", "Close") }}</button>
        </div>
        <div class="modal-body">
          <div class="grid-2">
            <div class="form-group">
              <label>{{ tx("用户名", "Username") }}</label>
              <input v-model="editUser.username" />
            </div>
            <div class="form-group">
              <label>{{ tx("显示名", "Display Name") }}</label>
              <input v-model="editUser.display_name" />
            </div>
            <div class="form-group">
              <label>Email</label>
              <input v-model="editUser.email" />
            </div>
            <div class="form-group">
              <label>{{ tx("角色", "Role") }}</label>
              <select v-model="editUser.role">
                <option value="user">user</option>
                <option value="admin">admin</option>
              </select>
            </div>
            <div class="form-group">
              <label>{{ tx("分组", "Group") }}</label>
              <input v-model="editUser.group" />
            </div>
            <div class="form-group">
              <label>{{ tx("状态", "Status") }}</label>
              <select v-model.number="editUser.status">
                <option :value="1">{{ tx("启用", "enabled") }}</option>
                <option :value="2">{{ tx("禁用", "disabled") }}</option>
              </select>
            </div>
          </div>
        </div>
        <div class="modal-foot">
          <button class="btn" @click="saveUserEdit">{{ tx("保存", "Save") }}</button>
        </div>
      </div>
    </div>

    <div v-if="editTokenOpen" class="modal-mask" @click.self="editTokenOpen = false">
      <div class="modal">
        <div class="modal-head">
          <strong>{{ tx("编辑令牌", "Edit Token") }}</strong>
          <button class="btn btn-outline" @click="editTokenOpen = false">{{ tx("关闭", "Close") }}</button>
        </div>
        <div class="modal-body">
          <div class="grid-2">
            <div class="form-group">
              <label>{{ tx("名称", "Name") }}</label>
              <input v-model="editToken.name" />
            </div>
            <div class="form-group">
              <label>{{ tx("状态", "Status") }}</label>
              <select v-model.number="editToken.status">
                <option :value="1">{{ tx("启用", "enabled") }}</option>
                <option :value="2">{{ tx("禁用", "disabled") }}</option>
              </select>
            </div>
            <div class="form-group">
              <label>{{ tx("额度", "Quota") }}</label>
              <input v-model.number="editToken.quota" type="number" min="0" />
            </div>
            <div class="form-group">
              <label>{{ tx("过期时间", "Expire At") }}</label>
              <input v-model.number="editToken.expired_at" type="number" />
            </div>
            <div class="form-group">
              <label>{{ tx("模型限制", "Models") }}</label>
              <input v-model="editToken.models" />
            </div>
            <div class="form-group">
              <label>{{ tx("子网限制", "Subnet") }}</label>
              <input v-model="editToken.subnet" />
            </div>
          </div>
        </div>
        <div class="modal-foot">
          <button class="btn" @click="saveTokenEdit">{{ tx("保存", "Save") }}</button>
        </div>
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
const users = ref<any[]>([]);
const tokens = ref<any[]>([]);
const previewText = ref("");
const selectedUserID = ref("");
const quotaSnapshot = ref<any | null>(null);

const query = reactive({
  search: "",
  page: 1,
  limit: 20
});

const totalUsers = ref(0);
const totalPages = computed(() => Math.max(1, Math.ceil(totalUsers.value / query.limit)));
const selectedUserIDs = ref<Set<string>>(new Set());
const allUsersCheckedOnPage = computed(() => {
  if (!users.value.length) return false;
  return users.value.every((u) => selectedUserIDs.value.has(String(u?.id || "")));
});

const createForm = reactive({
  username: "",
  password: "",
  email: "",
  role: "user",
  group: "default",
  quota: 0
});

const tokenForm = reactive({
  name: "",
  quota: 0,
  models: "",
  subnet: "",
  expiredAt: 0
});

const quotaDelta = ref(0);

const editUserOpen = ref(false);
const editUser = reactive<any>({
  id: "",
  username: "",
  display_name: "",
  email: "",
  role: "user",
  group: "default",
  status: 1
});

const editTokenOpen = ref(false);
const editToken = reactive<any>({
  id: 0,
  name: "",
  quota: 0,
  status: 1,
  models: "",
  subnet: "",
  expired_at: 0
});

function mustUserID(): string {
  const id = String(selectedUserID.value || "").trim();
  if (!id) {
    throw new Error(tx("请先选择用户", "Please select a user first"));
  }
  return id;
}

function buildUserQueryURL(): string {
  const offset = (query.page - 1) * query.limit;
  const q = new URLSearchParams();
  q.set("limit", String(query.limit));
  q.set("offset", String(Math.max(0, offset)));
  if (query.search.trim()) {
    q.set("search", query.search.trim());
  }
  return `/admin/auth/users?${q.toString()}`;
}

async function loadUsers() {
  try {
    const data = await apiRequest<any>(buildUserQueryURL());
    users.value = Array.isArray(data?.data) ? data.data : [];
    totalUsers.value = Number(data?.total || users.value.length || 0);
    if (query.page > totalPages.value) {
      query.page = totalPages.value;
      await loadUsers();
    }
  } catch (err: any) {
    toast(`${tx("加载用户失败", "Load users failed")}: ${err.message || err}`, "err");
  }
}

function toggleUserCheck(userID: string) {
  const next = new Set(selectedUserIDs.value);
  if (next.has(userID)) {
    next.delete(userID);
  } else {
    next.add(userID);
  }
  selectedUserIDs.value = next;
}

function toggleAllUsersOnPage() {
  const next = new Set(selectedUserIDs.value);
  if (allUsersCheckedOnPage.value) {
    for (const item of users.value) {
      next.delete(String(item?.id || ""));
    }
  } else {
    for (const item of users.value) {
      const id = String(item?.id || "");
      if (id) next.add(id);
    }
  }
  selectedUserIDs.value = next;
}

function searchUsers() {
  query.page = 1;
  void loadUsers();
}

function changeUserPage(next: number) {
  query.page = Math.max(1, Math.min(next, totalPages.value));
  void loadUsers();
}

async function createUser() {
  if (!createForm.username.trim() || !createForm.password.trim()) {
    toast(tx("用户名和密码不能为空", "username and password are required"), "err");
    return;
  }
  try {
    await apiRequest("/admin/auth/users", {
      method: "POST",
      body: JSON.stringify({
        username: createForm.username.trim(),
        password: createForm.password,
        email: createForm.email.trim(),
        role: createForm.role,
        group: createForm.group.trim(),
        quota: Number(createForm.quota || 0)
      })
    });
    toast(tx("用户已创建", "User created"));
    createForm.password = "";
    await loadUsers();
  } catch (err: any) {
    toast(`${tx("创建用户失败", "Create user failed")}: ${err.message || err}`, "err");
  }
}

async function removeUser(userID: string) {
  if (!window.confirm(tx(`确认删除用户 ${userID} ?`, `Delete user ${userID}?`))) {
    return;
  }
  try {
    await apiRequest(`/admin/auth/users/${encodeURIComponent(userID)}`, { method: "DELETE" });
    const next = new Set(selectedUserIDs.value);
    next.delete(String(userID));
    selectedUserIDs.value = next;
    if (selectedUserID.value === userID) {
      selectedUserID.value = "";
      tokens.value = [];
      quotaSnapshot.value = null;
      previewText.value = "";
    }
    toast(tx("用户已删除", "User deleted"));
    await loadUsers();
  } catch (err: any) {
    toast(`${tx("删除用户失败", "Delete user failed")}: ${err.message || err}`, "err");
  }
}

function selectedUserList(): string[] {
  return Array.from(selectedUserIDs.value.values()).filter(Boolean);
}

async function batchSetUserStatus(status: number) {
  const ids = selectedUserList();
  if (!ids.length) {
    toast(tx("请先勾选用户", "Please select users first"), "err");
    return;
  }
  try {
    for (const id of ids) {
      await apiRequest(`/admin/auth/users/${encodeURIComponent(id)}`, {
        method: "PUT",
        body: JSON.stringify({ status })
      });
    }
    toast(
      status === 1
        ? tx(`已启用 ${ids.length} 个用户`, `Enabled ${ids.length} users`)
        : tx(`已禁用 ${ids.length} 个用户`, `Disabled ${ids.length} users`)
    );
    await loadUsers();
  } catch (err: any) {
    toast(`${tx("批量更新用户状态失败", "Batch update user status failed")}: ${err.message || err}`, "err");
  }
}

async function batchDeleteUsers() {
  const ids = selectedUserList();
  if (!ids.length) {
    toast(tx("请先勾选用户", "Please select users first"), "err");
    return;
  }
  if (!window.confirm(tx(`确认批量删除 ${ids.length} 个用户？`, `Delete ${ids.length} users?`))) {
    return;
  }
  try {
    for (const id of ids) {
      await apiRequest(`/admin/auth/users/${encodeURIComponent(id)}`, { method: "DELETE" });
    }
    if (ids.includes(String(selectedUserID.value || ""))) {
      selectedUserID.value = "";
      tokens.value = [];
      quotaSnapshot.value = null;
    }
    selectedUserIDs.value = new Set();
    toast(tx(`已删除 ${ids.length} 个用户`, `Deleted ${ids.length} users`));
    await loadUsers();
  } catch (err: any) {
    toast(`${tx("批量删除用户失败", "Batch delete users failed")}: ${err.message || err}`, "err");
  }
}

async function loadQuotaSnapshot() {
  try {
    const userID = mustUserID();
    quotaSnapshot.value = await apiRequest<any>(`/admin/auth/users/${encodeURIComponent(userID)}/quota`);
  } catch {
    quotaSnapshot.value = null;
  }
}

function selectUser(userID: string) {
  selectedUserID.value = String(userID || "");
  void Promise.all([loadTokens(), loadQuotaSnapshot()]);
}

function openUserEdit(user: any) {
  editUser.id = String(user?.id || "");
  editUser.username = String(user?.username || "");
  editUser.display_name = String(user?.display_name || "");
  editUser.email = String(user?.email || "");
  editUser.role = String(user?.role || "user");
  editUser.group = String(user?.group || "default");
  editUser.status = Number(user?.status ?? 1);
  editUserOpen.value = true;
}

async function saveUserEdit() {
  const userID = String(editUser.id || "").trim();
  if (!userID) {
    toast(tx("无效用户 ID", "Invalid user id"), "err");
    return;
  }
  try {
    const data = await apiRequest<any>(`/admin/auth/users/${encodeURIComponent(userID)}`, {
      method: "PUT",
      body: JSON.stringify({
        username: String(editUser.username || "").trim(),
        display_name: String(editUser.display_name || "").trim(),
        email: String(editUser.email || "").trim(),
        role: String(editUser.role || "user"),
        group: String(editUser.group || "default"),
        status: Number(editUser.status ?? 1)
      })
    });
    previewText.value = JSON.stringify(data, null, 2);
    editUserOpen.value = false;
    toast(tx("用户已更新", "User updated"));
    await loadUsers();
  } catch (err: any) {
    toast(`${tx("更新用户失败", "Update user failed")}: ${err.message || err}`, "err");
  }
}

async function showUser(userID: string) {
  try {
    const data = await apiRequest<any>(`/admin/auth/users/${encodeURIComponent(String(userID || ""))}`);
    previewText.value = JSON.stringify(data, null, 2);
    selectedUserID.value = String(userID || "");
    await Promise.all([loadTokens(), loadQuotaSnapshot()]);
  } catch (err: any) {
    toast(`${tx("加载用户详情失败", "Load user detail failed")}: ${err.message || err}`, "err");
  }
}

async function addQuota() {
  let userID = "";
  try {
    userID = mustUserID();
  } catch (err: any) {
    toast(err.message || err, "err");
    return;
  }

  if (Number(quotaDelta.value || 0) === 0) {
    toast(tx("额度增减不能为 0", "quota delta cannot be 0"), "err");
    return;
  }
  try {
    const data = await apiRequest<any>(`/admin/auth/users/${encodeURIComponent(userID)}/quota`, {
      method: "POST",
      body: JSON.stringify({ amount: Number(quotaDelta.value || 0) })
    });
    previewText.value = JSON.stringify(data, null, 2);
    toast(tx("额度已更新", "Quota updated"));
    await Promise.all([loadUsers(), loadQuotaSnapshot()]);
  } catch (err: any) {
    toast(`${tx("调整额度失败", "Adjust quota failed")}: ${err.message || err}`, "err");
  }
}

async function loadTokens() {
  let userID = "";
  try {
    userID = mustUserID();
  } catch (err: any) {
    toast(err.message || err, "err");
    return;
  }
  try {
    const data = await apiRequest<any>(`/admin/auth/users/${encodeURIComponent(userID)}/tokens`);
    tokens.value = Array.isArray(data?.data) ? data.data : [];
  } catch (err: any) {
    toast(`${tx("加载令牌失败", "Load tokens failed")}: ${err.message || err}`, "err");
  }
}

async function createToken() {
  let userID = "";
  try {
    userID = mustUserID();
  } catch (err: any) {
    toast(err.message || err, "err");
    return;
  }
  try {
    const data = await apiRequest<any>(`/admin/auth/users/${encodeURIComponent(userID)}/tokens`, {
      method: "POST",
      body: JSON.stringify({
        quota: Number(tokenForm.quota || 0),
        name: tokenForm.name.trim(),
        models: tokenForm.models.trim(),
        subnet: tokenForm.subnet.trim(),
        expired_at: Number(tokenForm.expiredAt || 0)
      })
    });
    previewText.value = JSON.stringify(data, null, 2);
    toast(tx("令牌已创建", "Token created"));
    await loadTokens();
  } catch (err: any) {
    toast(`${tx("创建令牌失败", "Create token failed")}: ${err.message || err}`, "err");
  }
}

function openTokenEdit(token: any) {
  editToken.id = Number(token?.id || 0);
  editToken.name = String(token?.name || "");
  editToken.quota = Number(token?.quota || 0);
  editToken.status = Number(token?.status ?? 1);
  editToken.models = String(token?.models || "");
  editToken.subnet = String(token?.subnet || "");
  editToken.expired_at = Number(token?.expired_at || 0);
  editTokenOpen.value = true;
}

async function saveTokenEdit() {
  let userID = "";
  try {
    userID = mustUserID();
  } catch (err: any) {
    toast(err.message || err, "err");
    return;
  }
  if (!editToken.id) {
    toast(tx("无效令牌 ID", "Invalid token id"), "err");
    return;
  }
  try {
    const data = await apiRequest<any>(
      `/admin/auth/users/${encodeURIComponent(userID)}/tokens/${encodeURIComponent(String(editToken.id))}`,
      {
        method: "PUT",
        body: JSON.stringify({
          name: String(editToken.name || "").trim(),
          quota: Number(editToken.quota || 0),
          status: Number(editToken.status ?? 1),
          models: String(editToken.models || "").trim(),
          subnet: String(editToken.subnet || "").trim(),
          expired_at: Number(editToken.expired_at || 0)
        })
      }
    );
    previewText.value = JSON.stringify(data, null, 2);
    editTokenOpen.value = false;
    toast(tx("令牌已更新", "Token updated"));
    await loadTokens();
  } catch (err: any) {
    toast(`${tx("更新令牌失败", "Update token failed")}: ${err.message || err}`, "err");
  }
}

async function showToken(tokenID: number) {
  let userID = "";
  try {
    userID = mustUserID();
  } catch (err: any) {
    toast(err.message || err, "err");
    return;
  }
  try {
    const data = await apiRequest<any>(`/admin/auth/users/${encodeURIComponent(userID)}/tokens/${tokenID}`);
    previewText.value = JSON.stringify(data, null, 2);
  } catch (err: any) {
    toast(`${tx("加载令牌详情失败", "Load token detail failed")}: ${err.message || err}`, "err");
  }
}

async function deleteToken(tokenID: number) {
  let userID = "";
  try {
    userID = mustUserID();
  } catch (err: any) {
    toast(err.message || err, "err");
    return;
  }
  if (!window.confirm(tx(`确认删除令牌 ${tokenID} ?`, `Delete token ${tokenID}?`))) {
    return;
  }
  try {
    await apiRequest(`/admin/auth/users/${encodeURIComponent(userID)}/tokens/${tokenID}`, {
      method: "DELETE"
    });
    toast(tx("令牌已删除", "Token deleted"));
    await loadTokens();
  } catch (err: any) {
    toast(`${tx("删除令牌失败", "Delete token failed")}: ${err.message || err}`, "err");
  }
}

onMounted(loadUsers);
</script>
