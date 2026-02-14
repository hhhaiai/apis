<template>
  <div>
    <div class="section-header">
      <h2>{{ tx("技能引擎", "Skill Engine") }}</h2>
      <button class="btn btn-outline" @click="load">{{ tx("刷新", "Refresh") }}</button>
    </div>

    <div class="panel">
      <div class="panel-body">
        <div class="grid-2">
          <div class="form-group"><label>{{ tx("名称", "Name") }}</label><input v-model="form.name" placeholder="my_skill" /></div>
          <div class="form-group"><label>{{ tx("描述", "Description") }}</label><input v-model="form.description" :placeholder="tx('技能描述', 'Skill description')" /></div>
        </div>
        <div class="form-group"><label>{{ tx("模板", "Template") }}</label><textarea v-model="form.template" placeholder="Hello {{name}}, task={{task}}" /></div>
        <div class="form-group"><label>{{ tx("参数 (JSON 对象)", "Parameters (JSON object)") }}</label><textarea v-model="form.parameters" /></div>
        <div class="btn-row">
          <button class="btn" @click="register">{{ tx("注册技能", "Register Skill") }}</button>
        </div>
      </div>
    </div>

    <div class="panel">
      <table>
        <thead>
          <tr>
            <th>{{ tx("名称", "Name") }}</th>
            <th>{{ tx("描述", "Description") }}</th>
            <th>{{ tx("参数", "Parameters") }}</th>
            <th>{{ tx("操作", "Actions") }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="items.length === 0">
            <td colspan="4" class="small">{{ tx("暂无技能", "No skills") }}</td>
          </tr>
          <tr v-for="item in items" :key="item.name">
            <td class="mono">{{ item.name }}</td>
            <td>{{ item.description || "—" }}</td>
            <td class="mono">{{ Object.keys(item.parameters || {}).join(", ") || tx("无", "none") }}</td>
            <td>
              <div class="btn-row">
                <button class="btn btn-danger" @click="remove(item.name)">{{ tx("删除", "Delete") }}</button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";
import { apiRequest } from "../../lib/api";
import { useAdminI18n } from "../../lib/i18n";
import { toast } from "../../lib/toast";

const { tx } = useAdminI18n();
const items = ref<any[]>([]);
const form = reactive({
  name: "",
  description: "",
  template: "",
  parameters: "{}"
});

function parseObject(raw: string): Record<string, any> {
  const parsed = JSON.parse(raw || "{}");
  if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
    throw new Error(tx("parameters 必须是 JSON 对象", "parameters must be JSON object"));
  }
  return parsed;
}

async function load() {
  try {
    const data = await apiRequest<any>("/v1/cc/skills");
    items.value = Array.isArray(data?.data) ? data.data : [];
  } catch (err: any) {
    toast(`${tx("加载技能失败", "Load skills failed")}: ${err.message || err}`, "err");
  }
}

async function register() {
  if (!form.name.trim()) {
    toast(tx("技能名称不能为空", "skill name is required"), "err");
    return;
  }
  try {
    await apiRequest("/v1/cc/skills", {
      method: "POST",
      body: JSON.stringify({
        name: form.name.trim(),
        description: form.description.trim(),
        template: form.template,
        parameters: parseObject(form.parameters)
      })
    });
    toast(tx("技能已注册", "Skill registered"));
    await load();
  } catch (err: any) {
    toast(`${tx("注册技能失败", "Register skill failed")}: ${err.message || err}`, "err");
  }
}

async function remove(name: string) {
  try {
    await apiRequest(`/v1/cc/skills/${encodeURIComponent(name)}`, { method: "DELETE" });
    toast(tx("技能已删除", "Skill deleted"));
    await load();
  } catch (err: any) {
    toast(`${tx("删除技能失败", "Delete skill failed")}: ${err.message || err}`, "err");
  }
}

onMounted(load);
</script>
