# Contributing to chiron

娆㈣繋璐＄尞!chiron 鏄竴涓绉熸埛 SaaS AI Agent 骞冲彴(Go 缃戝叧 + Python AI 寮曟搸 + Vue 3 鍓嶇)銆傚湪鎻愪氦 PR 鍓?璇峰厛闃呰鏈枃浠朵笌 [SECURITY.md](SECURITY.md)銆?
## 寮€鍙戠幆澧?
### 鍓嶇疆渚濊禆

| 渚濊禆 | 鐗堟湰瑕佹眰 | 鐢ㄩ€?|
|---|---|---|
| Go | 1.26+([go.mod](go.mod) 澹版槑 `go 1.26.6`) | 缃戝叧 |
| Python | 3.11+ | AI 寮曟搸 |
| Node.js | 22+ | 鍓嶇(Vite 8 瑕佹眰) |
| pnpm / npm | 浠绘剰 | 鍓嶇渚濊禆绠＄悊(浠撳簱鍚屾椂鎻愪氦 `pnpm-lock.yaml` 涓?`package-lock.json`,CI 浣跨敤 npm) |
| PostgreSQL | 16(pgvector) | 涓诲簱 / 鍚戦噺搴?|
| Redis | 7 | 蹇呴渶渚濊禆:闃熷垪 / 璇箟缂撳瓨 / 鍒嗗竷寮忛檺娴?|
| MinIO / S3 | 鍙€?| 濯掍綋瀵硅薄瀛樺偍(`STORAGE_BACKEND=s3`) |
| Milvus | 2.5 | 鍙€?鍚戦噺妫€绱?涔熷彲鐢?pgvector) |
| Temporal | 鏈€鏂?| 鍙€?宸ヤ綔娴佸紩鎿?|

### 鎼缓姝ラ

```bash
# 1. 鍏嬮殕
git clone https://github.com/athenavi/chiron.git && cd chiron

# 2. 鍚姩鍩虹璁炬柦(PostgreSQL + Redis;闇€瑕佸獟浣?鍚戦噺鏃跺啀鍔?minio milvus-standalone temporal)
docker compose up -d postgres redis

# 3. 閰嶇疆鐜鍙橀噺
cp .env.example .env
#    缂栬緫 .env:JWT_SECRET(蹇呭～,openssl rand -base64 48)銆丳OSTGRES_DSN銆丩LM API Key

# 4. 瀹夎渚濊禆骞跺惎鍔?python run.py setup      # 棣栨:瀹夎 Python 渚濊禆銆佸墠绔緷璧?python run.py start      # 鍚姩缃戝叧(:8080)+ 寮曟搸(:8000)+ 鍓嶇(:5173)
```

甯哥敤鍛戒护:

```bash
python run.py status | logs | stop | restart | build
make build test lint fmt            # Go 渚?鍙€?CI 宸茶鐩?
```

## 浠ｇ爜瑙勮寖

### Go(缃戝叧,`internal/`銆乣cmd/`銆乣config/`)

- 浣跨敤 `gofmt` 鏍煎紡鍖?`go vet ./...` 闆跺憡璀?
- 鎻愪氦鍓嶈繍琛?`go test ./... -count=1`(鐩稿叧鍖?;
- 閿欒澶勭悊:涓嶅悶閿欍€佷笉瑁?`panic`;鏃ュ織浣跨敤 `log/slog` 缁撴瀯鍖栬緭鍑恒€?
### Python(寮曟搸,`python-engine/`)

- `ruff check .` 闆跺憡璀?閰嶇疆瑙?`python-engine/pyproject.toml`:E/F/W/I/N/UP/B,line-length 120);
- 绫诲瀷鏍囨敞:鏂颁唬鐮佸敖閲忛€氳繃 `mypy app/ --ignore-missing-imports`;
- 娴嬭瘯:`cd python-engine && python -m pytest tests`(寮傛娴嬭瘯 `asyncio_mode=auto`);闆嗘垚绫绘祴璇曟墦 `@pytest.mark.integration` 鏍囪銆?
### Vue 3 / TypeScript(鍓嶇,`frontend-vue/`)

- `npm run lint`(eslint + eslint-plugin-vue)闆跺憡璀?
- `npx vue-tsc --noEmit -p tsconfig.app.json` 绫诲瀷妫€鏌ラ€氳繃;
- 缁勪欢浣跨敤 `<script setup lang="ts">`;璺敱銆佺姸鎬?Pinia)涓?API 灏佽鍒嗗眰娓呮櫚;
- 鍗曟祴:`npm test`(vitest)銆?
## 娴嬭瘯

| 灞?| 鍛戒护 |
|---|---|
| 缃戝叧 | `go test ./... -race -count=1 -timeout=120s` |
| 寮曟搸 | `cd python-engine && python -m pytest tests` |
| 鍓嶇 | `cd frontend-vue && npm test` |

CI([.github/workflows/ci.yml](.github/workflows/ci.yml))浼氬湪 push / PR 鍒?`main` 鏃惰繍琛屼互涓婁笁绔鏌?PR 蹇呴』鍏ㄩ儴閫氳繃銆?
## 鎻愪氦淇℃伅瑙勮寖

浣跨敤 Conventional Commits 椋庢牸:`<type>(<scope>): <subject>`,渚嬪:

```text
feat(media): 澧炲姞濯掍綋绛惧悕 URL 鍏ㄩ摼璺?fix(security): 淇瀛樺偍鍨?XSS(鍒嗙墖涓婁紶 + /media/ 鏈嶅姟)
refactor(gateway): 鎶藉彇缁熶竴涓棿浠堕摼
test(api): /ready 鏂板绾﹂€傞厤
docs(architecture): 琛ュ厖澶氱鎴烽殧绂荤煩闃?ci: 涓夌 CI 娴佹按绾?go/pytest/vue-tsc)
```

- `type`:`feat` / `fix` / `refactor` / `docs` / `test` / `chore` / `ci` / `perf` / `style`;
- `scope`(鍙€?:`gateway` / `engine` / `frontend` / `media` / `market` / `security` / `redis` 绛?
- subject 鐢ㄧ浣垮彞銆佸皬鍐欏紑澶?涓枃鎴栬嫳鏂囧潎鍙?浣嗗悓涓€ PR 鍐呬繚鎸佷竴鑷淬€?
## PR 娴佺▼

1. 浠庢渶鏂?`main` 鍒囧嚭鍒嗘敮:`git checkout -b feat/my-feature`;
2. 灏忔鎻愪氦,姣忎釜鎻愪氦淇濇寔鍙瀯寤恒€佸彲娴嬭瘯;
3. 鎺ㄩ€佸悗鍒涘缓 PR,鐩爣鍒嗘敮 `main`,鎻忚堪鏀瑰姩鍔ㄦ満涓庡奖鍝嶈寖鍥?娑夊強 UI 璇烽檮鎴浘);
4. CI 涓夌妫€鏌?gateway / engine / frontend)鍏ㄩ儴閫氳繃;
5. 鑷冲皯 1 鍚嶇淮鎶よ€?review 閫氳繃鍚?squash 鍚堝苟銆?
## 琛屼负鍑嗗垯(Code of Conduct)

鏆傜敤 [Contributor Covenant](https://www.contributor-covenant.org/) 2.1 鐗堜綔涓洪粯璁よ涓哄噯鍒?鍦ㄤ粨搴撴寮忓彂甯冨墠,璇蜂繚鎸佸皧閲嶃€佸寘瀹广€佸缓璁炬€х殑鍗忎綔姘涘洿銆傜淮鎶よ€呮湁鏉冩嫆缁濊繚鍙嶅崗浣滅簿绁炵殑鍐呭銆?
## 闂鍙嶉

- Bug / 鍔熻兘寤鸿:GitHub Issues;
- **瀹夊叏婕忔礊:璇疯蛋 [SECURITY.md](SECURITY.md) 鐨勭鏈夋姤鍛婃笭閬?鍕垮叕寮€鎻愪氦銆?*

