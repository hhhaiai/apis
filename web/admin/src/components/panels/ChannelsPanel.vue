<template>
  <div>
    <div class="section-header">
      <h2>{{ tx("渠道管理", "Channels") }}</h2>
      <div class="btn-row">
        <button class="btn btn-outline" @click="loadChannels">{{ tx("刷新", "Refresh") }}</button>
        <button class="btn" @click="openCreate">{{ tx("新建渠道", "New Channel") }}</button>
      </div>
    </div>

    <div class="panel">
      <div class="panel-head">
        <strong>{{ tx("筛选与分页", "Filter & Pagination") }}</strong>
      </div>
      <div class="panel-body">
        <div class="toolbar">
          <div class="toolbar-item grow">
            <label>{{ tx("关键词", "Keyword") }}</label>
            <input
              v-model="query.keyword"
              :placeholder="tx('名称/类型/模型/分组', 'name/type/models/group')"
              @keyup.enter="applyFilter"
            />
          </div>
          <div class="toolbar-item">
            <label>{{ tx("类型", "Type") }}</label>
            <select v-model="query.type" @change="applyFilter">
              <option value="">{{ tx("全部", "All") }}</option>
              <option value="openai">openai</option>
              <option value="anthropic">anthropic</option>
              <option value="custom">custom</option>
            </select>
          </div>
          <div class="toolbar-item">
            <label>{{ tx("状态", "Status") }}</label>
            <select v-model="query.status" @change="applyFilter">
              <option value="">{{ tx("全部", "All") }}</option>
              <option value="1">{{ tx("启用", "enabled") }}</option>
              <option value="2">{{ tx("手动禁用", "manual disabled") }}</option>
              <option value="3">{{ tx("自动禁用", "auto disabled") }}</option>
            </select>
          </div>
          <div class="toolbar-item">
            <label>{{ tx("每页", "Page Size") }}</label>
            <select v-model.number="query.pageSize" @change="applyFilter">
              <option :value="10">10</option>
              <option :value="20">20</option>
              <option :value="50">50</option>
            </select>
          </div>
        </div>
        <p class="small">
          {{ tx("共", "Total") }} {{ filteredChannels.length }} {{ tx("条渠道", "channels") }}
          · {{ tx("当前页", "Page") }} {{ query.page }}
        </p>
      </div>
    </div>

    <div class="panel">
      <div class="panel-head">
        <strong>{{ tx("渠道列表", "Channel List") }}</strong>
        <div class="btn-row">
          <button class="btn btn-outline" @click="batchSetChannelStatus(1)">
            {{ tx("批量启用", "Batch Enable") }}
          </button>
          <button class="btn btn-outline" @click="batchSetChannelStatus(2)">
            {{ tx("批量禁用", "Batch Disable") }}
          </button>
          <button class="btn btn-danger" @click="batchDeleteChannels">
            {{ tx("批量删除", "Batch Delete") }}
          </button>
        </div>
      </div>
      <table>
        <thead>
          <tr>
            <th style="width: 44px">
              <input type="checkbox" :checked="allChannelsCheckedOnPage" @change="toggleAllChannelsOnPage" />
            </th>
            <th>ID</th>
            <th>{{ tx("名称", "Name") }}</th>
            <th>{{ tx("类型", "Type") }}</th>
            <th>{{ tx("模型", "Models") }}</th>
            <th>{{ tx("分组", "Group") }}</th>
            <th>{{ tx("状态", "Status") }}</th>
            <th>{{ tx("权重", "Weight") }}</th>
            <th>{{ tx("优先级", "Priority") }}</th>
            <th>{{ tx("操作", "Actions") }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="pagedChannels.length === 0">
            <td colspan="10" class="small">{{ tx("暂无渠道", "No channels") }}</td>
          </tr>
          <tr v-for="item in pagedChannels" :key="item.id">
            <td>
              <input
                type="checkbox"
                :checked="selectedChannelIDs.has(String(item.id))"
                @change="toggleChannelCheck(String(item.id))"
              />
            </td>
            <td class="mono">{{ item.id }}</td>
            <td>{{ item.name || "—" }}</td>
            <td>{{ item.type || "—" }}</td>
            <td class="mono">{{ item.models || "—" }}</td>
            <td class="mono">{{ item.group || "default" }}</td>
            <td>
              <span class="badge" :class="item.status === 1 ? 'badge-green' : 'badge-red'">
                {{ statusText(item.status) }}
              </span>
            </td>
            <td>{{ item.weight || 0 }}</td>
            <td>{{ item.priority || 0 }}</td>
            <td>
              <div class="btn-row">
                <button class="btn btn-outline" @click="openEdit(item)">{{ tx("编辑", "Edit") }}</button>
                <button class="btn btn-outline" @click="viewChannel(item.id)">{{ tx("详情", "Detail") }}</button>
                <button class="btn btn-outline" @click="toggleChannel(item.id, item.status === 1 ? 2 : 1)">
                  {{ item.status === 1 ? tx("禁用", "Disable") : tx("启用", "Enable") }}
                </button>
                <button class="btn btn-danger" @click="removeChannel(item.id)">{{ tx("删除", "Delete") }}</button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
      <div class="panel-body">
        <div class="pagination">
          <button class="btn btn-outline" :disabled="query.page <= 1" @click="changePage(query.page - 1)">
            {{ tx("上一页", "Prev") }}
          </button>
          <span class="small">{{ query.page }} / {{ totalPages }}</span>
          <button class="btn btn-outline" :disabled="query.page >= totalPages" @click="changePage(query.page + 1)">
            {{ tx("下一页", "Next") }}
          </button>
        </div>
      </div>
    </div>

    <div class="panel">
      <div class="panel-head">
        <strong>{{ tx("详情预览", "Detail Preview") }}</strong>
      </div>
      <div class="panel-body">
        <textarea v-model="previewText" readonly style="min-height: 160px" />
      </div>
    </div>

    <div v-if="editor.open" class="modal-mask" @click.self="closeEditor">
      <div class="modal modal-wide">
        <div class="modal-head">
          <strong>
            {{ editor.form.id ? tx("编辑渠道", "Edit Channel") : tx("新建渠道", "New Channel") }}
          </strong>
          <button class="btn btn-outline" @click="closeEditor">{{ tx("关闭", "Close") }}</button>
        </div>
        <div class="modal-body">
          <div class="grid-4">
            <div class="form-group">
              <label>ID</label>
              <input v-model="editor.form.id" disabled :placeholder="tx('自动生成', 'auto generated')" />
            </div>
            <div class="form-group">
              <label>{{ tx("名称", "Name") }}</label>
              <input v-model="editor.form.name" placeholder="openai-main" />
            </div>
            <div class="form-group">
              <label>{{ tx("类型", "Type") }}</label>
              <select v-model="editor.form.type">
                <option value="openai">openai</option>
                <option value="anthropic">anthropic</option>
                <option value="custom">custom</option>
              </select>
            </div>
            <div class="form-group">
              <label>API Key</label>
              <input
                v-model="editor.form.key"
                type="password"
                :placeholder="tx('更新时可留空', 'optional for update')"
              />
            </div>
            <div class="form-group">
              <label>Base URL</label>
              <input v-model="editor.form.baseURL" placeholder="https://api.openai.com/v1" />
            </div>
            <div class="form-group">
              <label>{{ tx("模型列表", "Models") }}</label>
              <input v-model="editor.form.models" placeholder="gpt-4o,gpt-4.1" />
            </div>
            <div class="form-group">
              <label>{{ tx("分组", "Group") }}</label>
              <input v-model="editor.form.group" placeholder="default" />
            </div>
            <div class="form-group">
              <label>{{ tx("状态", "Status") }}</label>
              <select v-model.number="editor.form.status">
                <option :value="1">{{ tx("启用", "enabled") }}</option>
                <option :value="2">{{ tx("手动禁用", "manual disabled") }}</option>
                <option :value="3">{{ tx("自动禁用", "auto disabled") }}</option>
              </select>
            </div>
            <div class="form-group">
              <label>{{ tx("权重", "Weight") }}</label>
              <input v-model.number="editor.form.weight" type="number" min="1" />
            </div>
            <div class="form-group">
              <label>{{ tx("优先级", "Priority") }}</label>
              <input v-model.number="editor.form.priority" type="number" />
            </div>
          </div>
        </div>
        <div class="modal-foot">
          <button class="btn" @click="saveChannel">
            {{ editor.form.id ? tx("保存更新", "Save Update") : tx("创建渠道", "Create Channel") }}
          </button>
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
const channels = ref<any[]>([]);
const previewText = ref("");

const query = reactive({
  keyword: "",
  type: "",
  status: "",
  page: 1,
  pageSize: 20
});
const selectedChannelIDs = ref<Set<string>>(new Set());

const editor = reactive({
  open: false,
  form: {
    id: "",
    name: "",
    type: "openai",
    key: "",
    baseURL: "",
    models: "",
    group: "default",
    status: 1,
    weight: 100,
    priority: 0
  }
});

const filteredChannels = computed(() => {
  const keyword = query.keyword.trim().toLowerCase();
  return channels.value.filter((item) => {
    if (query.type && String(item?.type || "") !== query.type) return false;
    if (query.status && String(item?.status || "") !== query.status) return false;
    if (!keyword) return true;
    const hay = [
      String(item?.name || ""),
      String(item?.type || ""),
      String(item?.models || ""),
      String(item?.group || "")
    ]
      .join(" ")
      .toLowerCase();
    return hay.includes(keyword);
  });
});

const totalPages = computed(() => {
  return Math.max(1, Math.ceil(filteredChannels.value.length / query.pageSize));
});

const pagedChannels = computed(() => {
  const start = (query.page - 1) * query.pageSize;
  return filteredChannels.value.slice(start, start + query.pageSize);
});
const allChannelsCheckedOnPage = computed(() => {
  if (!pagedChannels.value.length) return false;
  return pagedChannels.value.every((c) => selectedChannelIDs.value.has(String(c?.id || "")));
});

function statusText(status: number): string {
  if (status === 1) return tx("启用", "enabled");
  if (status === 2) return tx("手动禁用", "manual disabled");
  if (status === 3) return tx("自动禁用", "auto disabled");
  return tx("未知", "unknown");
}

function resetForm() {
  editor.form.id = "";
  editor.form.name = "";
  editor.form.type = "openai";
  editor.form.key = "";
  editor.form.baseURL = "";
  editor.form.models = "";
  editor.form.group = "default";
  editor.form.status = 1;
  editor.form.weight = 100;
  editor.form.priority = 0;
}

function openCreate() {
  resetForm();
  editor.open = true;
}

function openEdit(item: any) {
  editor.form.id = String(item?.id || "");
  editor.form.name = String(item?.name || "");
  editor.form.type = String(item?.type || "openai");
  editor.form.key = "";
  editor.form.baseURL = String(item?.base_url || "");
  editor.form.models = String(item?.models || "");
  editor.form.group = String(item?.group || "default");
  editor.form.status = Number(item?.status || 1);
  editor.form.weight = Number(item?.weight || 100);
  editor.form.priority = Number(item?.priority || 0);
  editor.open = true;
}

function closeEditor() {
  editor.open = false;
}

function applyFilter() {
  query.page = 1;
}

function toggleChannelCheck(channelID: string) {
  const next = new Set(selectedChannelIDs.value);
  if (next.has(channelID)) {
    next.delete(channelID);
  } else {
    next.add(channelID);
  }
  selectedChannelIDs.value = next;
}

function toggleAllChannelsOnPage() {
  const next = new Set(selectedChannelIDs.value);
  if (allChannelsCheckedOnPage.value) {
    for (const item of pagedChannels.value) {
      next.delete(String(item?.id || ""));
    }
  } else {
    for (const item of pagedChannels.value) {
      const id = String(item?.id || "");
      if (id) next.add(id);
    }
  }
  selectedChannelIDs.value = next;
}

function changePage(next: number) {
  query.page = Math.max(1, Math.min(next, totalPages.value));
}

async function loadChannels() {
  try {
    const data = await apiRequest<any>("/admin/channels");
    channels.value = Array.isArray(data?.data) ? data.data : [];
    if (query.page > totalPages.value) query.page = totalPages.value;
  } catch (err: any) {
    toast(`${tx("加载渠道失败", "Load channels failed")}: ${err.message || err}`, "err");
  }
}

function buildPayload() {
  const payload: Record<string, any> = {
    name: editor.form.name.trim(),
    type: editor.form.type,
    models: editor.form.models.trim(),
    group: editor.form.group.trim() || "default",
    status: Number(editor.form.status || 1),
    weight: Number(editor.form.weight || 100),
    priority: Number(editor.form.priority || 0)
  };
  if (editor.form.key.trim()) payload.key = editor.form.key.trim();
  if (editor.form.baseURL.trim()) payload.base_url = editor.form.baseURL.trim();
  return payload;
}

async function saveChannel() {
  if (!editor.form.name.trim()) {
    toast(tx("渠道名称不能为空", "channel name is required"), "err");
    return;
  }
  try {
    const payload = buildPayload();
    if (!editor.form.id) {
      const created = await apiRequest<any>("/admin/channels", {
        method: "POST",
        body: JSON.stringify(payload)
      });
      previewText.value = JSON.stringify(created, null, 2);
      toast(tx("渠道已创建", "Channel created"));
    } else {
      const updated = await apiRequest<any>(`/admin/channels/${encodeURIComponent(editor.form.id)}`, {
        method: "PUT",
        body: JSON.stringify(payload)
      });
      previewText.value = JSON.stringify(updated, null, 2);
      toast(tx("渠道已更新", "Channel updated"));
    }
    editor.form.key = "";
    closeEditor();
    await loadChannels();
  } catch (err: any) {
    toast(`${tx("保存渠道失败", "Save channel failed")}: ${err.message || err}`, "err");
  }
}

async function viewChannel(id: number | string) {
  try {
    const data = await apiRequest<any>(`/admin/channels/${encodeURIComponent(String(id || ""))}`);
    previewText.value = JSON.stringify(data, null, 2);
  } catch (err: any) {
    toast(`${tx("加载渠道详情失败", "Load channel detail failed")}: ${err.message || err}`, "err");
  }
}

async function toggleChannel(id: number | string, status: number) {
  try {
    await apiRequest(`/admin/channels/${encodeURIComponent(String(id || ""))}`, {
      method: "PUT",
      body: JSON.stringify({ status })
    });
    toast(status === 1 ? tx("渠道已启用", "Channel enabled") : tx("渠道已禁用", "Channel disabled"));
    await loadChannels();
  } catch (err: any) {
    toast(`${tx("更新渠道状态失败", "Update channel status failed")}: ${err.message || err}`, "err");
  }
}

async function removeChannel(id: number | string) {
  if (!window.confirm(tx(`确认删除渠道 ${id} ?`, `Delete channel ${id}?`))) {
    return;
  }
  try {
    await apiRequest(`/admin/channels/${encodeURIComponent(String(id || ""))}`, { method: "DELETE" });
    const next = new Set(selectedChannelIDs.value);
    next.delete(String(id || ""));
    selectedChannelIDs.value = next;
    toast(tx("渠道已删除", "Channel deleted"));
    await loadChannels();
  } catch (err: any) {
    toast(`${tx("删除渠道失败", "Delete channel failed")}: ${err.message || err}`, "err");
  }
}

function selectedChannelList(): string[] {
  return Array.from(selectedChannelIDs.value.values()).filter(Boolean);
}

async function batchSetChannelStatus(status: number) {
  const ids = selectedChannelList();
  if (!ids.length) {
    toast(tx("请先勾选渠道", "Please select channels first"), "err");
    return;
  }
  try {
    for (const id of ids) {
      await apiRequest(`/admin/channels/${encodeURIComponent(id)}`, {
        method: "PUT",
        body: JSON.stringify({ status })
      });
    }
    toast(
      status === 1
        ? tx(`已启用 ${ids.length} 个渠道`, `Enabled ${ids.length} channels`)
        : tx(`已禁用 ${ids.length} 个渠道`, `Disabled ${ids.length} channels`)
    );
    await loadChannels();
  } catch (err: any) {
    toast(`${tx("批量更新渠道状态失败", "Batch update channel status failed")}: ${err.message || err}`, "err");
  }
}

async function batchDeleteChannels() {
  const ids = selectedChannelList();
  if (!ids.length) {
    toast(tx("请先勾选渠道", "Please select channels first"), "err");
    return;
  }
  if (!window.confirm(tx(`确认批量删除 ${ids.length} 个渠道？`, `Delete ${ids.length} channels?`))) {
    return;
  }
  try {
    for (const id of ids) {
      await apiRequest(`/admin/channels/${encodeURIComponent(id)}`, { method: "DELETE" });
    }
    selectedChannelIDs.value = new Set();
    toast(tx(`已删除 ${ids.length} 个渠道`, `Deleted ${ids.length} channels`));
    await loadChannels();
  } catch (err: any) {
    toast(`${tx("批量删除渠道失败", "Batch delete channels failed")}: ${err.message || err}`, "err");
  }
}

onMounted(loadChannels);
</script>
