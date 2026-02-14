<template>
  <div>
    <div class="section-header">
      <h2>{{ tx("智能评估", "Intelligence Evaluation") }}</h2>
      <button class="btn" @click="runEval">{{ tx("执行", "Run") }}</button>
    </div>

    <div class="panel">
      <div class="panel-body">
        <div class="grid-2">
          <div class="form-group">
            <label>{{ tx("模型", "Model") }}</label>
            <input v-model="form.model" placeholder="claude-3.5-sonnet" />
          </div>
        </div>
        <div class="form-group">
          <label>{{ tx("提示词", "Prompt") }}</label>
          <textarea v-model="form.prompt" :placeholder="tx('输入评估提示词', 'Enter evaluation prompt')" />
        </div>
        <div class="form-group">
          <label>{{ tx("响应（可选）", "Response (optional)") }}</label>
          <textarea v-model="form.response" :placeholder="tx('可选响应文本', 'optional response text')" />
        </div>
      </div>
    </div>

    <div v-if="result">
      <div class="card-grid">
        <div class="card">
          <div class="label">{{ tx("总分", "Overall") }}</div>
          <div class="value">{{ Number(result.score || 0).toFixed(1) }}/10</div>
        </div>
        <div v-for="(val, key) in result.criteria || {}" :key="key" class="card">
          <div class="label">{{ String(key).replaceAll("_", " ") }}</div>
          <div class="value">{{ Number(val).toFixed(1) }}</div>
        </div>
      </div>
      <div class="panel">
        <div class="panel-head">
          <strong>{{ tx("分析", "Analysis") }}</strong>
        </div>
        <div class="panel-body">
          <pre style="white-space: pre-wrap; margin: 0">{{ result.analysis || tx("暂无分析", "No analysis") }}</pre>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref } from "vue";
import { apiRequest } from "../../lib/api";
import { useAdminI18n } from "../../lib/i18n";
import { toast } from "../../lib/toast";

const { tx } = useAdminI18n();
const form = reactive({
  model: "",
  prompt: "",
  response: ""
});

const result = ref<any | null>(null);

async function runEval() {
  if (!form.prompt.trim()) {
    toast(tx("提示词不能为空", "prompt is required"), "err");
    return;
  }
  try {
    const data = await apiRequest<any>("/v1/cc/eval", {
      method: "POST",
      body: JSON.stringify({
        model: form.model.trim(),
        prompt: form.prompt,
        response: form.response.trim() || undefined
      })
    });
    result.value = data?.result || data;
    toast(tx("评估完成", "Evaluation complete"));
  } catch (err: any) {
    toast(`${tx("评估失败", "Eval failed")}: ${err.message || err}`, "err");
  }
}
</script>
