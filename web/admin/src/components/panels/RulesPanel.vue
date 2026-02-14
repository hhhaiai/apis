<template>
  <div>
    <div class="section-header">
      <h2>{{ tx("规则（本地草稿）", "Rules (Local Draft)") }}</h2>
    </div>

    <div class="panel">
      <div class="panel-body">
        <div class="grid-4">
          <div class="form-group"><label>{{ tx("名称", "Name") }}</label><input v-model="form.name" placeholder="rule_name" /></div>
          <div class="form-group"><label>{{ tx("匹配模式", "Pattern") }}</label><input v-model="form.pattern" placeholder="*.go" /></div>
          <div class="form-group">
            <label>{{ tx("动作", "Action") }}</label>
            <select v-model="form.action">
              <option value="allow">allow</option>
              <option value="ask">ask</option>
              <option value="deny">deny</option>
            </select>
          </div>
          <div class="form-group"><label>{{ tx("优先级", "Priority") }}</label><input v-model.number="form.priority" type="number" /></div>
          <div class="form-group"><label>{{ tx("范围", "Scope") }}</label><input v-model="form.scope" placeholder="global" /></div>
        </div>
        <div class="btn-row">
          <button class="btn" @click="add">{{ tx("新增规则", "Add Rule") }}</button>
        </div>
      </div>
    </div>

    <div class="panel">
      <table>
        <thead>
          <tr>
            <th>{{ tx("名称", "Name") }}</th>
            <th>{{ tx("匹配模式", "Pattern") }}</th>
            <th>{{ tx("动作", "Action") }}</th>
            <th>{{ tx("优先级", "Priority") }}</th>
            <th>{{ tx("范围", "Scope") }}</th>
            <th>{{ tx("操作", "Actions") }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="items.length === 0">
            <td colspan="6" class="small">{{ tx("暂无本地规则", "No local rules") }}</td>
          </tr>
          <tr v-for="(item, idx) in items" :key="item.name + idx">
            <td class="mono">{{ item.name }}</td>
            <td class="mono">{{ item.pattern }}</td>
            <td>{{ item.action }}</td>
            <td>{{ item.priority }}</td>
            <td>{{ item.scope }}</td>
            <td>
              <button class="btn btn-danger" @click="remove(idx)">{{ tx("删除", "Remove") }}</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <p class="small">{{ tx("说明：当前后端未开放 rules 管理 API，这里是本地编辑草稿。", "Note: rules management API is not exposed yet; this is a local draft editor.") }}</p>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref } from "vue";
import { useAdminI18n } from "../../lib/i18n";
import { toast } from "../../lib/toast";

const { tx } = useAdminI18n();
const form = reactive({
  name: "",
  pattern: "",
  action: "allow",
  priority: 0,
  scope: "global"
});

const items = ref<any[]>([]);

function add() {
  if (!form.name.trim() || !form.pattern.trim()) {
    toast(tx("名称和匹配模式不能为空", "name and pattern are required"), "err");
    return;
  }
  items.value.push({
    name: form.name.trim(),
    pattern: form.pattern.trim(),
    action: form.action,
    priority: form.priority,
    scope: form.scope.trim() || "global"
  });
  items.value.sort((a, b) => b.priority - a.priority);
  toast(tx("规则已新增（本地）", "Rule added (local)"));
}

function remove(idx: number) {
  items.value.splice(idx, 1);
}
</script>
