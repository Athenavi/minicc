<script setup lang="ts">
import { ref, onMounted } from 'vue'
import {
  Card, Button, Spin, Empty, Input, Tabs, TabPane,
  Tag, message,
} from 'ant-design-vue'
import {
  BlockOutlined, DownloadOutlined, DeleteOutlined, ThunderboltOutlined,
} from '@ant-design/icons-vue'
import { api } from '../api'

interface SkillDef {
  name: string
  description: string
  version: string
  exec: { type: string }
}

const skills = ref<SkillDef[]>([])
const loading = ref(true)
const installURL = ref('')
const installInline = ref('')
const genDesc = ref('')
const genResult = ref<any>(null)
const activeTab = ref('list')

const execColors: Record<string, string> = {
  python: '#635BFF',
  shell: '#00D924',
  http: '#F2D900',
}

onMounted(async () => {
  await loadSkills()
})

async function loadSkills() {
  try {
    const response = await api.get('/v1/skills')
    if (Array.isArray(response.data?.data?.skills)) {
      skills.value = response.data.data.skills
    }
  } catch (error) {
    message.error('获取技能列表失败，请检查网络连接')
  } finally {
    loading.value = false
  }
}

async function handleInstall() {
  const body: any = {}
  if (installURL.value) {
    body.url = installURL.value
  } else if (installInline.value) {
    body.inline = installInline.value
  } else {
    message.error('请输入 URL 或内联 JSON')
    return
  }

  try {
    await api.post('/v1/skills/install', body)
    message.success('技能已安装')
    installURL.value = ''
    installInline.value = ''
    await loadSkills()
  } catch (error: any) {
    message.error(error.message || '安装失败')
  }
}

async function handleGenerate() {
  if (!genDesc.value.trim()) {
    message.error('请输入描述')
    return
  }

  try {
    const response = await api.post('/v1/skills/generate', {
      description: genDesc.value,
      auto_install: true,
    })
    genResult.value = response.data?.data?.skill || response.data?.data
    message.success('技能已生成并安装')
    genDesc.value = ''
    await loadSkills()
  } catch (error: any) {
    message.error(error.message || '生成失败')
  }
}

async function handleDelete(name: string) {
  try {
    await api.delete(`/v1/skills/${name}`)
    message.success(`已删除 ${name}`)
    await loadSkills()
  } catch (error: any) {
    message.error(error.message || '删除失败')
  }
}
</script>

<template>
  <div class="skills-container">
    <div class="skills-header">
      <div class="header-icon">
        <BlockOutlined style="font-size: 24px; color: #8b5cf6" />
      </div>
      <div>
        <h1>技能管理</h1>
        <p class="subtitle">扩展 AI Agent 的自定义能力</p>
      </div>
    </div>

    <Tabs v-model:activeKey="activeTab">
      <TabPane key="list" tab="技能列表">
        <Spin v-if="loading" class="loading-spinner" />

        <Empty v-else-if="skills.length === 0" description="暂无已安装的技能">
          <template #image>
            <ThunderboltOutlined style="font-size: 48px; color: #8b5cf6" />
          </template>
        </Empty>

        <div v-else class="skills-grid">
          <Card v-for="skill in skills" :key="skill.name" class="skill-card">
            <div class="skill-header">
              <span class="skill-name">{{ skill.name }}</span>
              <Tag :color="execColors[skill.exec?.type] || '#6b7280'">
                {{ skill.exec?.type || 'unknown' }}
              </Tag>
            </div>
            <p class="skill-description">{{ skill.description }}</p>
            <div class="skill-footer">
              <span class="skill-version">v{{ skill.version }}</span>
              <Button type="primary" danger size="small" @click="handleDelete(skill.name)">
                <template #icon><DeleteOutlined /></template>
                删除
              </Button>
            </div>
          </Card>
        </div>
      </TabPane>

      <TabPane key="install" tab="安装技能">
        <Card class="install-card">
          <h3><DownloadOutlined /> 安装技能</h3>

          <div class="install-section">
            <label>从 URL 安装</label>
            <Input v-model:value="installURL" placeholder="https://example.com/my-skill.skill.json" />
          </div>

          <div class="divider">或</div>

          <div class="install-section">
            <label>内联 JSON</label>
            <Input.TextArea
              v-model:value="installInline"
              :rows="4"
              placeholder='{ "name": "my-skill", "description": "...", ... }'
            />
          </div>

          <Button type="primary" @click="handleInstall" style="margin-top: 16px">
            <template #icon><DownloadOutlined /></template>
            安装
          </Button>
        </Card>
      </TabPane>

      <TabPane key="generate" tab="AI 生成">
        <Card class="generate-card">
          <h3><ThunderboltOutlined /> AI 生成技能</h3>

          <div class="generate-section">
            <label>描述你想要的技能</label>
            <Input.TextArea
              v-model:value="genDesc"
              :rows="3"
              placeholder="例如：创建一个能分析 Jenkins 构建日志并汇总失败原因的技能"
            />
          </div>

          <Button type="primary" @click="handleGenerate" style="margin-top: 16px">
            <template #icon><ThunderboltOutlined /></template>
            生成并安装
          </Button>

          <Card v-if="genResult" class="gen-result" style="margin-top: 16px">
            <h4>生成结果</h4>
            <pre>{{ JSON.stringify(genResult, null, 2) }}</pre>
          </Card>
        </Card>
      </TabPane>
    </Tabs>
  </div>
</template>

<style scoped>
.skills-container { padding: 24px; }
.skills-header { display: flex; align-items: center; gap: 12px; margin-bottom: 24px; }
.header-icon { width: 48px; height: 48px; border-radius: 12px; background-color: #f3e8ff; display: flex; align-items: center; justify-content: center; }
.skills-header h1 { margin: 0; font-size: 24px; font-weight: 600; }
.subtitle { margin: 4px 0 0; color: #6b7280; font-size: 14px; }
.loading-spinner { display: flex; justify-content: center; padding: 80px 0; }
.skills-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 16px; margin-top: 16px; }
.skill-card { transition: box-shadow 0.2s; }
.skill-card:hover { box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1); }
.skill-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 8px; }
.skill-name { font-weight: 600; font-size: 16px; }
.skill-description { color: #6b7280; font-size: 14px; margin: 0 0 12px; line-height: 1.5; }
.skill-footer { display: flex; align-items: center; justify-content: space-between; }
.skill-version { color: #9ca3af; font-size: 12px; }
.install-card, .generate-card { margin-top: 16px; }
.install-card h3, .generate-card h3 { display: flex; align-items: center; gap: 8px; margin: 0 0 16px; font-size: 16px; }
.install-section, .generate-section { margin-bottom: 16px; }
.install-section label, .generate-section label { display: block; margin-bottom: 8px; font-weight: 500; }
.divider { text-align: center; color: #9ca3af; margin: 16px 0; }
.gen-result pre { background-color: #f3f4f6; padding: 12px; border-radius: 8px; font-size: 12px; overflow-x: auto; }
</style>
