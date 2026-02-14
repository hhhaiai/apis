<template>
  <div>
    <div class="section-header">
      <h2>{{ tx("Agent 团队", "Agent Teams") }}</h2>
      <button class="btn btn-outline" @click="loadTeams">{{ tx("刷新", "Refresh") }}</button>
    </div>

    <div class="panel">
      <div class="panel-body">
        <div class="grid-4">
          <div class="form-group">
            <label>{{ tx("团队 ID", "Team ID") }}</label>
            <input v-model="teamForm.id" placeholder="team_alpha" />
          </div>
          <div class="form-group">
            <label>{{ tx("团队名称", "Team Name") }}</label>
            <input v-model="teamForm.name" :placeholder="tx('Alpha 团队', 'Alpha Team')" />
          </div>
          <div class="form-group">
            <label>{{ tx("队长 Agent ID", "Lead Agent ID") }}</label>
            <input v-model="teamForm.leadID" placeholder="lead_alpha" />
          </div>
          <div class="form-group">
            <label>{{ tx("队长模型", "Lead Model") }}</label>
            <input v-model="teamForm.leadModel" placeholder="gpt-4o" />
          </div>
        </div>
        <div class="btn-row">
          <button class="btn" @click="createTeam">{{ tx("创建团队", "Create Team") }}</button>
        </div>
      </div>
    </div>

    <div class="panel">
      <table>
        <thead>
          <tr>
            <th>ID</th>
            <th>{{ tx("名称", "Name") }}</th>
            <th>{{ tx("成员数", "Agents") }}</th>
            <th>{{ tx("任务数", "Tasks") }}</th>
            <th>{{ tx("操作", "Actions") }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="teams.length === 0">
            <td colspan="5" class="small">{{ tx("暂无团队", "No teams") }}</td>
          </tr>
          <tr v-for="item in teams" :key="item.id">
            <td class="mono">{{ item.id }}</td>
            <td>{{ item.name || "—" }}</td>
            <td>{{ item.agent_count || 0 }}</td>
            <td>{{ item.task_count || 0 }}</td>
            <td>
              <div class="btn-row">
                <button class="btn btn-outline" @click="selectTeam(item.id)">{{ tx("选择", "Select") }}</button>
                <button class="btn btn-outline" @click="loadTasks(item.id)">{{ tx("任务", "Tasks") }}</button>
                <button class="btn" @click="orchestrate(item.id)">{{ tx("执行", "Run") }}</button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="panel">
      <div class="panel-body">
        <div class="grid-3">
          <div class="form-group">
            <label>{{ tx("团队 ID", "Team ID") }}</label>
            <input v-model="taskForm.teamID" placeholder="team_alpha" />
          </div>
          <div class="form-group">
            <label>{{ tx("任务标题", "Task Title") }}</label>
            <input v-model="taskForm.title" :placeholder="tx('拆分工作', 'Split work')" />
          </div>
          <div class="form-group">
            <label>{{ tx("指派给", "Assigned To") }}</label>
            <input v-model="taskForm.assignedTo" placeholder="lead_alpha" />
          </div>
        </div>
        <div class="btn-row">
          <button class="btn" @click="createTask">{{ tx("新增任务", "Add Task") }}</button>
          <button class="btn btn-outline" @click="orchestrate(taskForm.teamID)">{{ tx("编排执行", "Orchestrate") }}</button>
          <button class="btn btn-outline" @click="loadTasks(taskForm.teamID)">{{ tx("加载任务", "Load Tasks") }}</button>
        </div>
        <div class="form-group" style="margin-top: 12px">
          <label>{{ tx("任务看板详情", "Task Board Detail") }}</label>
          <textarea v-model="taskPreview" readonly style="min-height: 150px" />
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
const teams = ref<any[]>([]);
const taskPreview = ref("");

const teamForm = reactive({
  id: "",
  name: "",
  leadID: "lead",
  leadModel: "default"
});

const taskForm = reactive({
  teamID: "",
  title: "",
  assignedTo: ""
});

async function loadTeams() {
  try {
    const data = await apiRequest<any>("/v1/cc/teams?limit=200");
    teams.value = Array.isArray(data?.data) ? data.data : [];
  } catch (err: any) {
    toast(`${tx("加载团队失败", "Load teams failed")}: ${err.message || err}`, "err");
  }
}

function selectTeam(teamID: string) {
  taskForm.teamID = teamID;
}

async function createTeam() {
  if (!teamForm.name.trim()) {
    toast(tx("团队名称不能为空", "team name is required"), "err");
    return;
  }
  try {
    await apiRequest("/v1/cc/teams", {
      method: "POST",
      body: JSON.stringify({
        id: teamForm.id.trim() || undefined,
        name: teamForm.name.trim(),
        agents: [
          {
            id: teamForm.leadID.trim() || "lead",
            name: teamForm.leadID.trim() || "lead",
            role: "lead",
            model: teamForm.leadModel.trim() || "default"
          }
        ]
      })
    });
    if (teamForm.id.trim()) {
      taskForm.teamID = teamForm.id.trim();
    }
    toast(tx("团队已创建", "Team created"));
    await loadTeams();
  } catch (err: any) {
    toast(`${tx("创建团队失败", "Create team failed")}: ${err.message || err}`, "err");
  }
}

async function createTask() {
  if (!taskForm.teamID.trim() || !taskForm.title.trim()) {
    toast(tx("团队 ID 和任务标题不能为空", "team id and task title are required"), "err");
    return;
  }
  try {
    const data = await apiRequest<any>(`/v1/cc/teams/${encodeURIComponent(taskForm.teamID)}/tasks`, {
      method: "POST",
      body: JSON.stringify({
        title: taskForm.title.trim(),
        assigned_to: taskForm.assignedTo.trim()
      })
    });
    taskPreview.value = JSON.stringify(data, null, 2);
    toast(tx("任务已创建", "Task created"));
    await loadTeams();
  } catch (err: any) {
    toast(`${tx("创建任务失败", "Create task failed")}: ${err.message || err}`, "err");
  }
}

async function loadTasks(teamID: string) {
  const id = (teamID || "").trim();
  if (!id) {
    toast(tx("团队 ID 不能为空", "team id is required"), "err");
    return;
  }
  try {
    const data = await apiRequest<any>(`/v1/cc/teams/${encodeURIComponent(id)}/tasks`);
    taskPreview.value = JSON.stringify(data, null, 2);
  } catch (err: any) {
    toast(`${tx("加载任务失败", "Load tasks failed")}: ${err.message || err}`, "err");
  }
}

async function orchestrate(teamID: string) {
  const id = (teamID || "").trim();
  if (!id) {
    toast(tx("团队 ID 不能为空", "team id is required"), "err");
    return;
  }
  try {
    const data = await apiRequest<any>(`/v1/cc/teams/${encodeURIComponent(id)}/orchestrate`, {
      method: "POST"
    });
    taskPreview.value = JSON.stringify(data, null, 2);
    toast(tx("团队编排执行完成", "Team orchestrated"));
    await loadTeams();
  } catch (err: any) {
    toast(`${tx("编排执行失败", "Orchestrate failed")}: ${err.message || err}`, "err");
  }
}

onMounted(loadTeams);
</script>
