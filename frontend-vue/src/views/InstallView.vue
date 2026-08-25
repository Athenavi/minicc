<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { message } from 'ant-design-vue'
import {
  getInstallStatus,
  getInstallStep1,
  postInstallStep2,
  postInstallStep3,
  setupSystem,
  hasInstallToken,
  saveInstallToken,
  type InstallDep,
} from '../api/install'

const router = useRouter()
const loading = ref(true)
const error = ref(false)
const submitting = ref(false)

// 鈹€鈹€ 閫氱敤鐘舵€?鈹€鈹€
const deps = ref<InstallDep[]>([])
const installed = ref(false)

// 鈹€鈹€ 鎵嬪姩杈撳叆浠ょ墝 鈹€鈹€
const manualToken = ref('')

// 鈹€鈹€ 瀹夎妯″紡锛坰etup锛変笁姝ュ悜瀵肩姸鎬?鈹€鈹€
// wizard: env=鐜妫€娴?APP_SECRET) / db=鏁版嵁搴撻厤缃?/ admin=鍒涘缓绠＄悊鍛?/ done=瀹夎瀹屾垚 /
//         token=缂哄皯浠ょ墝 / legacy=姝ｅ父妯″紡锛圖B 灏辩华浣嗘棤 owner锛夊崟姝ュ垱寤虹鐞嗗憳
const wizard = ref<'env' | 'db' | 'admin' | 'done' | 'token' | 'legacy'>('env')
const appSecretSet = ref(false)
const dataWritable = ref(true)
const step2Done = ref(false)

const dbForm = ref({ app_secret: '', postgres_dsn: '', redis_addr: '', redis_password: '', redis_db: 0 })
const adminForm = ref({ email: '', password: '', confirm: '', name: '' })

// 鈹€鈹€ 姝ｅ父妯″紡锛圖B 灏辩华浣嗘棤绠＄悊鍛橈級鍗曟琛ㄥ崟 鈹€鈹€
const legacyForm = ref({ email: '', password: '', confirm: '', name: '' })

const depsReady = computed(() => deps.value.length > 0 && deps.value.every((d) => d.ok))
const postgresOk = computed(() => deps.value.find((d) => d.name === 'postgres')?.ok ?? false)

async function fetchStatus() {
  loading.value = true
  error.value = false
  try {
    const s = await getInstallStatus()
    deps.value = s.deps ?? []
    if (deps.value.length === 0) {
      deps.value = [
        { name: 'postgres', ok: s.db, message: s.db ? 'PostgreSQL 杩炴帴姝ｅ父' : 'PostgreSQL 涓嶅彲鐢? },
        { name: 'redis', ok: s.redis, message: s.redis ? 'Redis 杩炴帴姝ｅ父' : 'Redis 涓嶅彲鐢? },
      ]
    }
    if (!s.needed) {
      // 鍏堟鏌?install.lock 鏄惁瀹為檯瀹屾垚锛岄伩鍏嶆暟鎹簱娈嬬暀鏃?owner 璁板綍瀵艰嚧璇垽
      try {
        const step1 = await getInstallStep1()
        if (step1.completed) {
          installed.value = true
          return
        }
      } catch {
        // 蹇界暐
      }
      // 鏈畬鎴愪絾鍚庣杩斿洖 needed=false 鈫?寮哄埗杩涘叆鍚戝
      if (!postgresOk.value) {
        await loadWizard()
      } else {
        wizard.value = 'legacy'
      }
    } else if (!postgresOk.value) {
      // PostgreSQL 涓嶅彲杈?鈫?瀹夎妯″紡锛岃蛋涓夋鍚戝锛堥渶瑕佸畨瑁呬护鐗岋級
      await loadWizard()
    } else {
      // PostgreSQL 鍙揪浣嗘棤 owner 鈫?姝ｅ父妯″紡鍗曟鍒涘缓绠＄悊鍛?
      wizard.value = 'legacy'
    }
  } catch (e: any) {
    error.value = true
    message.error('鏃犳硶杩炴帴鍚庣鏈嶅姟锛岃纭鏈嶅姟宸插惎鍔?)
  } finally {
    loading.value = false
  }
}

async function loadWizard() {
  if (!hasInstallToken()) {
    wizard.value = 'token'
    return
  }
  try {
    const s = await getInstallStep1()
    if (s.completed) {
      installed.value = true
      return
    }
    appSecretSet.value = !!s.app_secret_set
    dataWritable.value = !!s.data_writable
    step2Done.value = !!s.step2_done
    if (step2Done.value) {
      wizard.value = 'admin'
    } else if (appSecretSet.value) {
      wizard.value = 'db'
    } else {
      wizard.value = 'env'
    }
  } catch (e: any) {
    if (e?.response?.status === 401) {
      wizard.value = 'token'
    } else {
      error.value = true
      message.error(e?.response?.data?.error || '鏃犳硶璇诲彇瀹夎鐘舵€?)
    }
  }
}

// 鈹€鈹€ 鎵嬪姩杈撳叆浠ょ墝 鈹€鈹€
async function submitToken() {
  const tok = manualToken.value.trim()
  if (!tok) {
    message.warning('璇疯緭鍏ュ畨瑁呬护鐗?)
    return
  }
  // 淇濆瓨鍒?localStorage锛屽悗缁〉闈㈠埛鏂?瀵艰埅鏃犻渶 URL 鍙傛暟
  saveInstallToken(tok)
  message.success('浠ょ墝宸蹭繚瀛橈紝姝ｅ湪鍔犺浇瀹夎鍚戝...')
  await loadWizard()
}

// 鈹€鈹€ Step 1锛氭彁浜?APP_SECRET锛堟垨 APP_SECRET 宸查厤缃椂鐩存帴璺宠浆鍒版暟鎹簱閰嶇疆锛夆攢鈹€
async function skipStep1() {
  console.log('[Install] skipStep1 called, appSecretSet:', appSecretSet.value, 'dbForm.app_secret:', dbForm.value.app_secret)
  // 濡傛灉 APP_SECRET 宸查€氳繃鐜鍙橀噺閰嶇疆锛岀洿鎺ヨ烦杞埌鏁版嵁搴撻厤缃〉
  if (appSecretSet.value) {
    wizard.value = 'db'
    return
  }
  // APP_SECRET 鏈厤缃紝鐢ㄦ埛蹇呴』杈撳叆
  if (!dbForm.value.app_secret.trim()) {
    message.warning('璇疯緭鍏?APP_SECRET锛堥儴缃蹭富瀵嗛挜锛岃嚦灏?32 瀛楃锛?)
    return
  }
  if (dbForm.value.app_secret.trim().length < 32) {
    message.warning('APP_SECRET 闀垮害涓嶈冻锛岃浣跨敤鑷冲皯 32 瀛楃鐨勯殢鏈哄瓧绗︿覆')
    return
  }
  // 灏?APP_SECRET 淇濈暀鍦?dbForm 涓紝鎻愪氦 Step 2 鏃朵竴骞跺彂閫?
  appSecretSet.value = true
  wizard.value = 'db'
  console.log('[Install] skipStep1 done, wizard now:', wizard.value)
  message.success('APP_SECRET 宸茶缃紝璇风户缁厤缃暟鎹簱')
}

// 鈹€鈹€ 涓夋鍚戝鎻愪氦 鈹€鈹€
async function submitStep2() {
  console.log('[Install] submitStep2 called', dbForm.value)
  if (!dbForm.value.postgres_dsn.trim()) {
    message.warning('PostgreSQL 杩炴帴涓诧紙DSN锛夊繀濉?)
    return
  }
  submitting.value = true
  try {
    console.log('[Install] calling postInstallStep2...')
    const result = await postInstallStep2({
      app_secret: dbForm.value.app_secret.trim() || undefined,
      postgres_dsn: dbForm.value.postgres_dsn.trim(),
      redis_addr: dbForm.value.redis_addr.trim() || undefined,
      redis_password: dbForm.value.redis_password || undefined,
      redis_db: dbForm.value.redis_db || undefined,
    })
    console.log('[Install] postInstallStep2 success', result)
    step2Done.value = true
    wizard.value = 'admin'
    message.success('鏁版嵁搴撻厤缃凡淇濆瓨骞堕獙璇侀€氳繃')
  } catch (e: any) {
    console.error('[Install] postInstallStep2 error', e)
    message.error(e?.response?.data?.error || '鏁版嵁搴撻厤缃け璐?)
  } finally {
    submitting.value = false
  }
}

async function submitStep3() {
  console.log('[Install] submitStep3 called', adminForm.value)
  if (!adminForm.value.email || !adminForm.value.name || !adminForm.value.password) {
    message.warning('閭銆佸鍚嶃€佸瘑鐮佸潎蹇呭～')
    return
  }
  if (adminForm.value.password.length < 8) {
    message.warning('瀵嗙爜鑷冲皯 8 浣?)
    return
  }
  if (adminForm.value.password !== adminForm.value.confirm) {
    message.warning('涓ゆ瀵嗙爜涓嶄竴鑷?)
    return
  }
  submitting.value = true
  try {
    console.log('[Install] calling postInstallStep3...')
    await postInstallStep3({
      email: adminForm.value.email,
      password: adminForm.value.password,
      name: adminForm.value.name,
    })
    console.log('[Install] postInstallStep3 success')
    wizard.value = 'done'
    message.success('瀹夎瀹屾垚')
  } catch (e: any) {
    console.error('[Install] postInstallStep3 error', e)
    const errMsg = e?.response?.data?.error || ''
    if (errMsg.includes('鏁版嵁搴撹繛鎺ュ凡澶辨晥') || errMsg.includes('璇烽噸鏂板畬鎴愭暟鎹簱閰嶇疆')) {
      message.warning('瀹夎涓柇鍚庢暟鎹簱杩炴帴宸插け鏁堬紝璇烽噸鏂伴厤缃暟鎹簱')
      wizard.value = 'db'
    } else {
      message.error(errMsg || '鍒涘缓绠＄悊鍛樺け璐?)
    }
  } finally {
    submitting.value = false
  }
}

// 鈹€鈹€ 姝ラ鍥為€€ 鈹€鈹€
function goBackToEnv() {
  wizard.value = 'env'
  message.info('杩斿洖鐜妫€娴嬫楠?)
}

function goBackToDb() {
  wizard.value = 'db'
  message.info('杩斿洖鏁版嵁搴撻厤缃楠?)
}

// 鈹€鈹€ 姝ｅ父妯″紡鎻愪氦锛圖B 灏辩华銆佹棤 owner锛夆攢鈹€
async function submitLegacy() {
  if (!legacyForm.value.email || !legacyForm.value.name || !legacyForm.value.password) {
    message.warning('閭銆佸鍚嶃€佸瘑鐮佸潎蹇呭～')
    return
  }
  if (legacyForm.value.password.length < 8) {
    message.warning('瀵嗙爜鑷冲皯 8 浣?)
    return
  }
  if (legacyForm.value.password !== legacyForm.value.confirm) {
    message.warning('涓ゆ瀵嗙爜涓嶄竴鑷?)
    return
  }
  submitting.value = true
  try {
    await setupSystem({
      email: legacyForm.value.email,
      password: legacyForm.value.password,
      name: legacyForm.value.name,
    })
    message.success('鍒濆鍖栨垚鍔燂紝璇风櫥褰?)
    router.replace('/login')
  } catch (e: any) {
    message.error(e?.response?.data?.error || '鍒濆鍖栧け璐?)
  } finally {
    submitting.value = false
  }
}

onMounted(fetchStatus)
</script>

<template>
  <div class="install-page">
    <div class="install-card">
      <div class="install-brand">
        <span class="brand-mark">MC</span>
        <span>chiron 路 绯荤粺鍒濆鍖?/span>
      </div>

      <a-spin :spinning="loading">
        <!-- 渚濊禆鎺㈡祴锛堝缁堝睍绀猴紝渚夸簬鎺掓煡杩炴帴闂锛?-->
        <div v-if="!error && deps.length" class="dep-list" aria-label="渚濊禆灏辩华鐘舵€?>
          <div v-for="d in deps" :key="d.name" class="dep-item">
            <span class="dep-icon" :class="d.ok ? 'ok' : 'fail'">{{ d.ok ? '鉁? : '鉁? }}</span>
            <span class="dep-name">{{ d.name }}</span>
            <span class="dep-msg">{{ d.message }}</span>
          </div>
          <a-button v-if="!depsReady" size="small" type="link" @click="fetchStatus">閲嶆柊妫€娴?/a-button>
        </div>

        <!-- 閮ㄧ讲妯″瀷璇存槑 -->
        <div v-if="!error && !installed" class="install-hint hint-info">
          鏈儴缃蹭粎闇€鍦?.env 閰嶇疆 <b>APP_SECRET</b>锛堝敮涓€涓诲瘑閽ワ級銆侾ostgreSQL / Redis / CORS / 瀛樺偍 / 妯″瀷 / 鏀粯绛夐厤缃?
          鍒濆鍖栧悗鍙湪鍚庡彴銆岀郴缁熻缃€嶇粺涓€绠＄悊銆傝嫢鏁版嵁搴?Redis 涓嶅湪鏈満榛樿鍦板潃锛屽彲鍦ㄥ畨瑁呭悜瀵间腑濉啓杩炴帴淇℃伅锛?
          淇濆瓨鍚?b>閲嶅惎鏈嶅姟</b>鐢熸晥銆?
        </div>

        <!-- 閿欒锛堟棤娉曡繛鎺ュ悗绔紝浼樺厛鏄剧ず锛?-->
        <template v-if="error">
          <div class="installed-state">
            <div class="error-icon">鈿?/div>
            <h3 class="installed-title">鏃犳硶妫€鏌ョ郴缁熺姸鎬?/h3>
            <p class="installed-desc">璇风‘璁ゅ悗绔湇鍔″凡鍚姩锛堥粯璁ょ鍙?8080锛夈€?/p>
            <a-button type="primary" block @click="fetchStatus">閲嶈瘯</a-button>
          </div>
        </template>

        <!-- 宸插垵濮嬪寲锛堜紭鍏堜簬鍚戝锛?-->
        <template v-else-if="installed">
          <div class="installed-state">
            <div class="installed-icon">鉁?/div>
            <h3 class="installed-title">绯荤粺宸插垵濮嬪寲</h3>
            <p class="installed-desc">
              绠＄悊鍛樿处鎴峰凡鍒涘缓锛岃浣跨敤绠＄悊鍛樺嚟鎹櫥褰曠郴缁熴€?
            </p>
            <a-button type="primary" size="large" block @click="router.push('/login')">鍓嶅線鐧诲綍</a-button>
          </div>
        </template>

        <!-- 鈺愨晲 瀹夎妯″紡锛氱己灏戜护鐗?鈺愨晲 -->
        <template v-else-if="wizard === 'token'">
          <p class="install-hint hint-warn">
            褰撳墠澶勪簬<b>瀹夎妯″紡</b>锛堢郴缁熸湭閰嶇疆鏁版嵁搴?涓诲瘑閽ワ級銆傚畨瑁呴〉闈㈠彈浠ょ墝淇濇姢锛?
          </p>
          <ol class="token-steps">
            <li>鏌ョ湅鏈嶅姟鍚姩鏃ュ織涓殑 <code>install_url</code>锛堝舰濡?<code>/install?token=xxx</code>锛夛紱</li>
            <li>浣跨敤鏃ュ織涓殑瀹屾暣鍦板潃锛堝惈浠ょ墝锛夐噸鏂拌闂湰椤甸潰锛屾垨<b>鍦ㄤ笅鏂硅緭鍏ヤ护鐗?/b>銆?/li>
          </ol>
          <p class="install-hint hint-info">
            鎻愮ず锛氭湭閰嶇疆 APP_SECRET 鏃朵护鐗屼负闅忔満鐢熸垚锛堥噸鍚悗鍙樺寲锛夛紱閰嶇疆 APP_SECRET 鍚庝护鐗岀敱鍏剁‘瀹氭€ф淳鐢熴€?
          </p>
          <a-form layout="vertical" @finish="submitToken">
            <a-form-item label="瀹夎浠ょ墝">
              <a-input v-model:value="manualToken" placeholder="鍦ㄦ绮樿创鍚姩鏃ュ織涓殑 token" />
            </a-form-item>
            <a-button type="primary" html-type="submit" block @click="submitToken">鎻愪氦浠ょ墝</a-button>
          </a-form>
          <a-button type="link" block @click="router.replace('/install')">閲嶆柊璁块棶瀹夎椤?/a-button>
        </template>

        <!-- 鈺愨晲 瀹夎妯″紡 Step 1锛氱幆澧冩娴嬶紙APP_SECRET锛夆晲鈺?-->
        <template v-else-if="wizard === 'env'">
          <a-steps :current="0" size="small" class="wizard-steps">
            <a-step title="鐜妫€娴? />
            <a-step title="鏁版嵁搴撻厤缃? />
            <a-step title="鍒涘缓绠＄悊鍛? />
          </a-steps>
          <p class="install-hint hint-warn">
            绯荤粺鏈娴嬪埌鏈夋晥鐨?<b>APP_SECRET</b>锛堥儴缃茬骇涓诲瘑閽ワ紝鈮?2 瀛楃锛夈€傚畠鏄?JWT / 閰嶇疆鍔犲瘑鐨勫敮涓€瀵嗛挜鏉ユ簮銆?
            鎮ㄥ彲浠?b>鍦ㄤ笅鏂硅緭鍏?APP_SECRET</b>锛屾垨鍦?<code>.env</code> 涓厤缃悗閲嶅惎鏈嶅姟銆?
          </p>
          <div class="env-check">
            <div class="dep-item">
              <span class="dep-icon" :class="appSecretSet ? 'ok' : 'fail'">{{ appSecretSet ? '鉁? : '鉁? }}</span>
              <span class="dep-name">APP_SECRET</span>
              <span class="dep-msg">{{ appSecretSet ? '宸查厤缃? : '鏈厤缃垨涓哄急鍊?鍗犱綅绗? }}</span>
            </div>
            <div class="dep-item">
              <span class="dep-icon" :class="dataWritable ? 'ok' : 'fail'">{{ dataWritable ? '鉁? : '鉁? }}</span>
              <span class="dep-name">鏁版嵁鐩綍</span>
              <span class="dep-msg">{{ dataWritable ? '鍙啓锛坕nstall.lock 鍙惤鐩橈級' : '涓嶅彲鍐欙細璇锋鏌?data/ 鐩綍鏉冮檺' }}</span>
            </div>
          </div>
          <a-form layout="vertical" @finish="skipStep1">
            <a-form-item label="APP_SECRET锛堥儴缃蹭富瀵嗛挜锛岃嚦灏?32 瀛楃锛?>
              <a-input-password v-model:value="dbForm.app_secret" placeholder="鍦ㄦ杈撳叆 APP_SECRET锛堟垨鍏堝湪 .env 涓厤缃悗閲嶅惎鏈嶅姟锛? />
            </a-form-item>
            <a-button type="primary" html-type="submit" :loading="submitting" block @click="skipStep1">
              {{ appSecretSet ? '缁х画閰嶇疆鏁版嵁搴? : '鎻愪氦 APP_SECRET 骞剁户缁? }}
            </a-button>
          </a-form>
        </template>

        <!-- 鈺愨晲 瀹夎妯″紡 Step 2锛氭暟鎹簱閰嶇疆 鈺愨晲 -->
        <template v-else-if="wizard === 'db'">
          <a-steps :current="1" size="small" class="wizard-steps">
            <a-step title="鐜妫€娴? />
            <a-step title="鏁版嵁搴撻厤缃? />
            <a-step title="鍒涘缓绠＄悊鍛? />
          </a-steps>
          <p class="install-hint hint-warn">
            濉啓 PostgreSQL 杩炴帴淇℃伅锛堝繀濉級涓?Redis锛堥€夊～锛岀暀绌哄垯鎸夌幆澧冨彉閲忓苟闄嶇骇杩愯锛夈€?
            鍚庣灏?b>灏濊瘯杩炴帴楠岃瘉</b>锛岄€氳繃鍚庡姞瀵嗕繚瀛樺埌 <code>data/install.lock</code>锛涢噸鍚湇鍔″悗鍏ㄩ潰鐢熸晥銆?
          </p>
          <a-form layout="vertical" @finish="submitStep2">
            <a-form-item label="PostgreSQL 杩炴帴涓诧紙DSN锛? required>
              <a-input v-model:value="dbForm.postgres_dsn" placeholder="postgres://user:pass@host:5432/chiron?sslmode=disable" />
            </a-form-item>
            <a-form-item label="Redis 鍦板潃锛堥€夊～锛?>
              <a-input v-model:value="dbForm.redis_addr" placeholder="localhost:6379" />
            </a-form-item>
            <a-form-item label="Redis 瀵嗙爜锛堥€夊～锛?>
              <a-input-password v-model:value="dbForm.redis_password" placeholder="鏃犲瘑鐮佸彲鐣欑┖" />
            </a-form-item>
            <a-form-item label="Redis DB锛堥€夊～锛?>
              <a-input-number v-model:value="dbForm.redis_db" :min="0" :max="15" style="width: 100%" />
            </a-form-item>
            <a-button type="primary" html-type="submit" :loading="submitting" block @click="submitStep2">淇濆瓨骞堕獙璇佽繛鎺?/a-button>
            <a-button style="margin-top: 8px" block @click="goBackToEnv">涓婁竴姝ワ細淇敼 APP_SECRET</a-button>
          </a-form>
        </template>

        <!-- 鈺愨晲 瀹夎妯″紡 Step 3锛氬垱寤虹鐞嗗憳 鈺愨晲 -->
        <template v-else-if="wizard === 'admin'">
          <a-steps :current="2" size="small" class="wizard-steps">
            <a-step title="鐜妫€娴? />
            <a-step title="鏁版嵁搴撻厤缃? />
            <a-step title="鍒涘缓绠＄悊鍛? />
          </a-steps>
          <p class="install-hint hint-warn">
            鏁版嵁搴撻厤缃凡淇濆瓨骞堕獙璇侀€氳繃銆傝鍒涘缓棣栦釜绠＄悊鍛樿处鎴凤紙owner 瑙掕壊锛夛紝璇ヨ处鎴锋嫢鏈夊叏閮ㄧ鐞嗘潈闄愩€?
            瀹屾垚鍚庡畨瑁呭叆鍙ｅ皢鍏抽棴銆?
          </p>
          <a-form layout="vertical" @finish="submitStep3">
            <a-form-item label="閭" required>
              <a-input v-model:value="adminForm.email" type="email" placeholder="admin@example.com" />
            </a-form-item>
            <a-form-item label="濮撳悕" required>
              <a-input v-model:value="adminForm.name" placeholder="绠＄悊鍛樺鍚? />
            </a-form-item>
            <a-form-item label="瀵嗙爜锛堣嚦灏?8 浣嶏級" required>
              <a-input-password v-model:value="adminForm.password" placeholder="鑷冲皯 8 浣? />
            </a-form-item>
            <a-form-item label="纭瀵嗙爜" required>
              <a-input-password v-model:value="adminForm.confirm" placeholder="鍐嶆杈撳叆瀵嗙爜" />
            </a-form-item>
            <a-button type="primary" html-type="submit" :loading="submitting" block @click="submitStep3">瀹屾垚瀹夎</a-button>
            <a-button style="margin-top: 8px" block @click="goBackToDb">涓婁竴姝ワ細淇敼鏁版嵁搴撻厤缃?/a-button>
          </a-form>
        </template>

        <!-- 鈺愨晲 瀹夎妯″紡锛氬畬鎴?鈺愨晲 -->
        <template v-else-if="wizard === 'done'">
          <div class="installed-state">
            <div class="installed-icon">鉁?/div>
            <h3 class="installed-title">瀹夎瀹屾垚</h3>
            <p class="installed-desc">
              绠＄悊鍛樿处鎴峰凡鍒涘缓锛屾暟鎹簱閰嶇疆宸蹭繚瀛樸€傝<b>閲嶅惎鏈嶅姟</b>浣垮叏閮ㄥ姛鑳界敓鏁堬紝鐒跺悗浣跨敤绠＄悊鍛樺嚟鎹櫥褰曘€?
            </p>
            <a-button type="primary" size="large" block @click="router.replace('/login')">鍓嶅線鐧诲綍</a-button>
          </div>
        </template>

        <!-- 鈺愨晲 姝ｅ父妯″紡锛欴B 灏辩华銆佹棤绠＄悊鍛橈紙鍒涘缓棣栦釜 owner锛汻edis 鍙€夐檷绾э級鈺愨晲 -->
        <template v-else-if="wizard === 'legacy'">
          <p class="install-hint hint-warn">
            <template v-if="depsReady">
              妫€娴嬪埌绯荤粺灏氭湭鍒濆鍖栥€傝鍒涘缓棣栦釜绠＄悊鍛樿处鎴凤紙owner 瑙掕壊锛夛紝璇ヨ处鎴锋嫢鏈夊叏閮ㄧ鐞嗘潈闄愩€?
              鍒濆鍖栧悗姝ゅ叆鍙ｅ皢鑷姩鍏抽棴銆?
            </template>
            <template v-else>
              PostgreSQL 杩炴帴姝ｅ父锛屼絾 Redis 涓嶅彲鐢紙鏈嶅姟浠ラ檷绾фā寮忚繍琛岋級銆備粛鍙洿鎺ュ垱寤虹鐞嗗憳璐︽埛瀹屾垚鍒濆鍖栥€?
            </template>
          </p>
          <a-form layout="vertical" @finish="submitLegacy">
            <a-form-item label="閭" required>
              <a-input v-model:value="legacyForm.email" type="email" placeholder="admin@example.com" />
            </a-form-item>
            <a-form-item label="濮撳悕" required>
              <a-input v-model:value="legacyForm.name" placeholder="绠＄悊鍛樺鍚? />
            </a-form-item>
            <a-form-item label="瀵嗙爜锛堣嚦灏?8 浣嶏級" required>
              <a-input-password v-model:value="legacyForm.password" placeholder="鑷冲皯 8 浣? />
            </a-form-item>
            <a-form-item label="纭瀵嗙爜" required>
              <a-input-password v-model:value="legacyForm.confirm" placeholder="鍐嶆杈撳叆瀵嗙爜" />
            </a-form-item>
            <a-button type="primary" html-type="submit" :loading="submitting" block @click="submitLegacy">鍒濆鍖栫郴缁?/a-button>
          </a-form>
        </template>
      </a-spin>

      <div class="install-footer">
        <a-button type="link" @click="router.push('/login')">杩斿洖鐧诲綍</a-button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.install-page {
  min-height: 100dvh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px 16px;
  background: var(--bg-page);
  position: relative;
  overflow: hidden;
}
.install-page::before {
  content: '';
  position: absolute;
  inset: -45% -20% auto -20%;
  height: 60%;
  background: radial-gradient(ellipse 55% 55% at 50% 0%, var(--primary-bg), transparent 72%);
  pointer-events: none;
}
.install-card {
  width: 440px;
  max-width: calc(100vw - 32px);
  padding: 32px;
  background: var(--bg-card);
  border: 1px solid var(--border-card);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-lg);
  position: relative;
  z-index: 1;
  animation: installFadeIn 0.5s ease;
}
.install-brand {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 24px;
  font-size: 18px;
  font-weight: 600;
  color: var(--text-primary);
}
.brand-mark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  background: linear-gradient(135deg, var(--primary), var(--primary-dark));
  color: #fff;
  border-radius: 8px;
  font-size: 14px;
  box-shadow: var(--shadow-md);
}
.wizard-steps {
  margin: 0 0 20px;
}
.install-hint {
  margin: 0 0 20px;
  color: var(--text-secondary);
  font-size: 13px;
  line-height: 1.6;
  padding: 12px;
  border-radius: var(--radius-sm, 6px);
}
.dep-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 0 0 20px;
  padding: 4px 0;
}
.env-check {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 0 0 16px;
}
.dep-item {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--text-secondary);
  border: 1px solid var(--border-card);
  border-radius: var(--radius-sm, 6px);
  padding: 8px 10px;
}
.dep-icon.ok { color: var(--colorSuccess, #52c41a); }
.dep-icon.fail { color: var(--colorError, #ff4d4f); }
.dep-name {
  font-weight: 600;
  color: var(--text-primary);
  min-width: 90px;
}
.dep-msg { font-size: 12px; }
.token-steps {
  margin: 0 0 20px;
  padding-left: 18px;
  color: var(--text-secondary);
  font-size: 13px;
  line-height: 1.8;
}
.token-steps code {
  background: var(--bg-hover, rgba(128, 128, 128, 0.12));
  padding: 1px 5px;
  border-radius: 4px;
  font-size: 12px;
  word-break: break-all;
}
.hint-warn {
  background: var(--warning-bg, rgba(255, 197, 23, 0.1));
  border: 1px solid var(--warning-border, rgba(255, 197, 23, 0.3));
}
.hint-info {
  background: var(--info-bg, rgba(22, 119, 255, 0.08));
  border: 1px solid var(--info-border, rgba(22, 119, 255, 0.25));
}
.hint-info b { color: var(--text-primary); }
.installed-state {
  text-align: center;
  padding: 24px 0;
}
.installed-icon {
  width: 64px;
  height: 64px;
  margin: 0 auto 16px;
  border-radius: 50%;
  background: var(--success-bg, rgba(82, 196, 26, 0.12));
  color: var(--colorSuccess, #52c41a);
  font-size: 32px;
  line-height: 64px;
  font-weight: bold;
}
.error-icon {
  width: 64px;
  height: 64px;
  margin: 0 auto 16px;
  border-radius: 50%;
  background: var(--error-bg, rgba(255, 77, 79, 0.1));
  color: var(--colorError, #ff4d4f);
  font-size: 32px;
  line-height: 64px;
  font-weight: bold;
}
.installed-title {
  margin: 0 0 8px;
  font-size: 18px;
  font-weight: 600;
  color: var(--text-primary);
}
.installed-desc {
  margin: 0 0 24px;
  color: var(--text-tertiary);
  font-size: 13px;
}
.install-footer {
  margin-top: 16px;
  text-align: center;
}
@keyframes installFadeIn {
  from { opacity: 0; transform: translateY(20px); }
  to { opacity: 1; transform: translateY(0); }
}

/* 绉诲姩绔紙鈮?76px锛屼笌 .u-hide-sm 鏂偣涓€鑷达級锛氬皬灞忛《閮ㄥ榻愶紝渚夸簬闀胯〃鍗曟粴鍔?*/
@media (max-width: 576px) {
  .install-page { align-items: flex-start; padding: 16px 12px; }
  .install-card { width: 100%; max-width: 100%; padding: 24px 16px; }
  .install-brand { font-size: 16px; margin-bottom: 18px; }
  /* iOS 鑱氱劍闃茬缉鏀撅細杈撳叆瀛楀彿 鈮?6px锛堝惈瀵嗙爜妗嗗唴灞?input锛?*/
  .install-card :deep(.ant-input) { font-size: 16px; }
  /* 瑙︽帶鐩爣 鈮?40px锛堟帓闄?small 鎸夐挳锛?*/
  .install-card :deep(.ant-btn:not(.ant-btn-sm)) { min-height: 40px; }
}

/* 鐒︾偣澧炲己 */
.install-card :deep(.ant-input:focus),
.install-card :deep(.ant-btn:focus-visible) {
  outline: 2px solid var(--primary);
  outline-offset: 2px;
}

@media (prefers-reduced-motion: reduce) {
  .install-card { animation: none; }
}
</style>

