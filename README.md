# chiron

> 澶氱鎴?SaaS AI Agent 骞冲彴 鈥?Go 缃戝叧 + Python AI 寮曟搸 + Vue 3 鍓嶇銆?> A multi-tenant SaaS AI Agent platform powered by a Go gateway, a Python AI engine and a Vue 3 frontend.

[![CI](https://img.shields.io/github/actions/workflow/status/athenavi/chiron/ci.yml?branch=main&logo=github&label=CI)](https://github.com/athenavi/chiron/actions)
[![Coverage](https://img.shields.io/codecov/c/github/athenavi/chiron?logo=codecov&label=coverage)](https://codecov.io/gh/athenavi/chiron)
[![Go Version](https://img.shields.io/github/go-mod/go-version/athenavi/chiron?logo=go)](https://github.com/athenavi/chiron/blob/main/go.mod)
[![Release](https://img.shields.io/github/v/release/athenavi/chiron?logo=github)](https://github.com/athenavi/chiron/releases)
[![License](https://img.shields.io/github/license/athenavi/chiron)](LICENSE)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](CONTRIBUTING.md)

chiron 鏄竴濂楀紑绠卞嵆鐢ㄧ殑**澶氱鎴?AI Agent 鎺у埗鍙?*:瀵硅瘽銆丄gent銆佸伐浣滄祦銆佹妧鑳姐€佺煡璇嗗簱涓庢彃浠跺叚澶у伐浣滃彴浜掕仈浜掗€?璁?LLM 鍦ㄧ湡瀹炲満鏅腑鎸佺画宸ヤ綔銆傚畠閲囩敤 **Go 缃戝叧 + Python AI 寮曟搸**鐨勫垎绂绘灦鏋勨€斺€擥o 璐熻矗璁よ瘉銆侀檺娴併€佽璐逛笌娴佸紡杞彂,Python 鎵胯浇 Agent 鎺ㄧ悊銆佷换鍔＄紪鎺掍笌 RAG銆?
## 鉁?鏍稿績浜偣

- **澶氱鎴峰畨鍏ㄩ殧绂?* 鈥?鏁版嵁鍦ㄧ鎴?鐢ㄦ埛涓ょ骇寮哄埗闅旂:PostgreSQL 琛岀骇 `tenant_id` 杩囨护銆丷edis 鍒嗙鎴?key 鍛藉悕绌洪棿銆丮ilvus 鍚戦噺妫€绱?`tenant_id` filter銆佸獟浣撹祫婧愬綊灞炴牎楠屻€佹瘡鐢ㄦ埛鎻掍欢閰嶇疆鐩綍銆佹瘡绉熸埛鐙珛閰嶉涓庨檺娴併€?- **鍏ぇ宸ヤ綔鍙颁簰鑱斾簰閫?* 鈥?瀵硅瘽 / Agent / 宸ヤ綔娴?/ 鎶€鑳?/ 鐭ヨ瘑搴?/ 鎻掍欢閫氳繃 Capability Registry + TaskRouter 缁熶竴缂栨帓:鑷劧璇█浠诲姟鑷姩鍒嗚В涓哄瓙浠诲姟 DAG,璺ㄥ伐浣滃彴鍗忓悓鎵ц骞惰仛鍚堢粨鏋?`POST /v1/chat/submit`)銆?- **缃戝叧 + 寮曟搸鏋舵瀯** 鈥?Go 缃戝叧鏄敮涓€瀵瑰鍏ュ彛(璁よ瘉 / API Key / 闄愭祦 / 璁¤垂 / SSE 杞彂),Python 寮曟搸涓撴敞 AI 鎺ㄧ悊;鍐呴儴閫氳繃 `X-Internal-Token` 鍙屽悜閴存潈,**fail-close** 鎷掔粷韬唤閫忎紶缁曡繃銆?
## 馃殌 蹇€熷紑濮?
### Docker Compose(鎺ㄨ崘,涓€鏉″懡浠よ捣鍏ㄦ爤)

```bash
cp .env.example .env        # 鑷冲皯濉啓 APP_SECRET(鍞竴涓诲瘑閽?,涓氬姟閰嶇疆鍙湪鍚庡彴銆岀郴缁熻缃€嶇淮鎶?docker compose up -d --build
```

鍚姩鍚庤闂?<http://localhost:3000>(鍓嶇),API 缃戝叧鍦?<http://localhost:8080>,鍋ュ悍妫€鏌?`GET /health`銆?
> 棣栨鍚姩浼氳嚜鍔ㄦ墽琛屾暟鎹簱杩佺Щ(`migrate` 鏈嶅姟)銆俙APP_SECRET` 娲剧敓 JWT 涓庡唴閮ㄤ簰淇′护鐗?LLM/瀛樺偍/鏀粯绛夊瘑閽ョ粡鍔犲瘑钀藉簱,鍙湪鍚庡彴缁熶竴绠＄悊銆?
### 鏈湴寮€鍙戞ā寮?
```bash
# 1. 鍙惎鍔ㄥ熀纭€璁炬柦(PostgreSQL + Redis,鍙€?MinIO/Milvus/Temporal)
docker compose up -d postgres redis

# 2. 閰嶇疆
cp .env.example .env        # 濉啓 APP_SECRET 鍗冲彲;鍏朵綑鍦ㄥ悗鍙般€岀郴缁熻缃€嶇淮鎶?
# 3. 鍚姩 Go 缃戝叧 + Python 寮曟搸 + 鍓嶇(鑷姩鏋勫缓)
python run.py start
```

璁块棶 <http://localhost:5173>銆傚父鐢ㄥ懡浠?`python run.py start|stop|restart|status|logs|build|setup`銆?
### 鐜鍙橀噺閫熸煡

| 鍙橀噺 | 蹇呭～ | 璇存槑 |
|---|---|---|
| `APP_SECRET` | 鉁?| **鍞竴閮ㄧ讲涓诲瘑閽?鑷冲皯 32 瀛楃**,`python -c "import secrets; print(secrets.token_urlsafe(32))"` 鐢熸垚銆傛淳鐢?JWT_SECRET / INTERNAL_TOKEN,骞跺姞瀵嗙鐞嗗悗鍙扮殑鏁忔劅閰嶇疆;鏈缃綉鍏虫嫆缁濆惎鍔?|
| `POSTGRES_DSN` | 鍙€?| PostgreSQL 寮曞杩炴帴涓?缂虹渷鐢?`postgres://postgres@localhost:5432/chiron`銆傝繛鎺ュ悗浠庡悗鍙般€岀郴缁熻缃€嶈鍙栭泦缇ら厤缃鐩?鍒囨崲鏁版嵁搴撻泦缇ら噸鍚敓鏁?|
| `JWT_SECRET` / `INTERNAL_TOKEN` | 鍙€?| 榛樿鐢?`APP_SECRET` 娲剧敓;浠呭湪闇€瑕佹洿寮哄瘑閽ラ殧绂绘椂鏄惧紡瑕嗙洊 |

> 鍏朵綑涓氬姟/鍩虹璁炬柦閰嶇疆(Redis銆丆ORS銆佸瓨鍌ㄣ€佹ā鍨嬨€侀檺娴併€佹敮浠樼瓑)鍧囧凡杩佺Щ鑷冲悗鍙般€岀郴缁熻缃€?鏁忔劅鍊?瀵嗛挜/瀵嗙爜)缁?`APP_SECRET` 娲剧敓瀵嗛挜 AES-GCM 鍔犲瘑钀藉簱,鍙湪鍚庡彴缁熶竴淇敼鈥斺€斾究浜庡垏鎹?Redis 闆嗙兢銆佹暟鎹簱闆嗙兢绛夈€傜敓浜?HTTPS 閮ㄧ讲璇峰湪鍚庡彴/鐜璁剧疆 `COOKIE_SECURE=true`銆?
瀹屾暣鍙橀噺瑙?[.env.example](.env.example)銆?
## 馃З 鐗规€ф€昏

**AI 瀵硅瘽(4 绉嶆ā寮?**

- 甯歌 `normal` / 鏋佺畝 `minimal` / PTC `ptc` / 鍒涢€?`creative` 鍥涚鎺ㄧ悊妯″紡,鎸変换鍔¤嚜鐢卞垏鎹?- 宸ュ叿璋冪敤鍏ㄧ▼鍙鍖?宸ュ叿閾捐繕鍘熴€丼SE 娴佸紡杈撳嚭銆佷細璇濆彲鍙栨秷)

**Agent**

- 澶氭櫤鑳戒綋鍗忓悓銆佷换鍔″垎鍙戜笌缁撴灉杩借釜(`POST /v1/agents/dispatch`)
- 涓婁笅鏂囧帇缂┿€佸伐鍏蜂笁鎬佽鍐?鎷掔粷 / 鏇挎崲 / 纭)銆佹矙绠卞伐浣滃尯 `sandbox/{tenant}/{user}/workspace`

**宸ヤ綔娴?DAG**

- 鍙鍖栫紪鎺掑姝ヤ换鍔?鑺傜偣鑷敱杩炵嚎,杩愯鏃剁紪杈?鏀寔骞惰浼樺寲涓庝緷璧栬皟搴?
**鎶€鑳藉競鍦?/ Agent 甯傚満 / MCP 甯傚満**

- 浼佷笟鑳藉姏甯傚満(`/v1/ent/market/items`),鎶€鑳?/ Agent / MCP 涓夌被鏉＄洰缁熶竴鐩綍銆佺鎴风骇瀹夎涓庡惎鍋?
**鐭ヨ瘑搴?RAG**

- 鏂囨。鍏ュ簱銆佸垎鍧椾笌鍚戦噺鍖?pgvector / Milvus 鍙屽悗绔?銆丠NSW 浣欏鸡妫€绱€乣tenant_id` 绾у悜閲忔暟鎹殧绂?
**濯掍綋搴?*

- 涓婁紶 / 鍒嗙墖涓婁紶 / S3 棰勭鍚嶄笂浼?濯掍綋璧勬簮**绛惧悕 URL** 璁块棶(HMAC-SHA256,15 鍒嗛挓鏈夋晥鏈?褰掑睘鏍￠獙)

**澶氱鎴蜂笌瀹夊叏**

- 绉熸埛 / 鐢ㄦ埛涓ょ骇鏁版嵁闅旂銆丣WT + API Key + OAuth/OIDC/SMS 璁よ瘉銆丳rompt 娉ㄥ叆妫€娴嬨€丼SRF 绔彛鐧藉悕鍗曘€佹彃浠跺懡浠ょ櫧鍚嶅崟銆佸垎甯冨紡闄愭祦銆佸彲淇′唬鐞?CIDR銆佸璁℃棩蹇?
**浼佷笟鐗?*

- 閰嶉绠＄悊銆佹垚鏈腑蹇冦€丷BAC 瑙掕壊 / 缇ょ粍銆佹搷浣滃璁°€佹ā鍨嬬瓥鐣ャ€侀殣绉佹ā寮忋€佸鍩熷悕绠＄悊銆佺嫭绔嬪畨瑁呭悜瀵?`/v1/install/setup`)

## 馃彈锔?鏋舵瀯

```
鈹屸攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?  HTTP / SSE / WS    鈹屸攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?  Internal API     鈹屸攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?鈹?frontend-vue 鈹?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈻?鈹?  Go Gateway :8080 鈹?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈻?鈹?  Python AI Engine :8000 鈹?鈹?  (Vue 3)    鈹?鈼€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€ 鈹? JWT / API Key     鈹? X-Internal-Token  鈹? FastAPI / Agent / RAG   鈹?鈹斺攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?   SSE 娴佸紡杩斿洖       鈹? 闄愭祦 / 璁¤垂 / CORS 鈹?鈼€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€ 鈹? TaskRouter / Workflow   鈹?                                      鈹斺攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?   SSE / streaming   鈹斺攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?                                                鈹?                                            鈹?                鈹屸攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹尖攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?     鈹屸攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹尖攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?          PostgreSQL (pgvector)           Redis (蹇呴渶)                MinIO / S3     Redis     PostgreSQL   Milvus
          浼氳瘽 / 濯掍綋鍏冩暟鎹?/ 甯傚満      闄愭祦 / 闃熷垪 / 璇箟缂撳瓨      濯掍綋瀵硅薄瀛樺偍    闃熷垪/缂撳瓨    pgvector    鍚戦噺妫€绱?                鈹斺攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹尖攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?               鈹?                                                鈹斺攢鈹€ Temporal(宸ヤ綔娴? 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?```

- **Go 缃戝叧**(`:8080`,`internal/`):璁よ瘉 / 璁¤垂 / SSE 杞彂 / 浼氳瘽涓庢秷鎭惤搴?/ 濯掍綋 / 鐭ヨ瘑搴?/ 甯傚満 / 浼佷笟绠℃帶
- **Python 寮曟搸**(`:8000`,`python-engine/`):Agent 寰幆(4 妯″紡)銆乀askRouter 缁熶竴缂栨帓銆佸伐鍏锋矙绠遍殧绂汇€佷笁鏍忓畨鍏?杈撳叆鍑€鍖?/ 宸ュ叿涓夋€?/ 杈撳嚭鑴辨晱)銆佸 Provider 娑堟伅鏍煎紡閫傞厤
- **鍓嶇**(Vue 3 + Vite):鑱婂ぉ鐣岄潰(铏氭嫙婊氬姩 / 娴佸紡鎬濈淮閾?/ 宸ュ叿閾捐繕鍘?銆佸叚澶у伐浣滃彴銆佺鐞嗗悗鍙?
## 馃О 鎶€鏈爤

| 灞?| 鎶€鏈?|
|---|---|
| 缃戝叧 | Go 1.26,鏍囧噯搴?`net/http`(1.22+ 璺敱),`pgx/v5`, `go-redis/v9`, `minio-go/v7`, `golang-jwt/v5`, `gorilla/websocket`, `cobra` |
| AI 寮曟搸 | Python 3.11, FastAPI, uvicorn, anthropic / openai SDK, pymilvus / qdrant-client, asyncpg, redis, PyJWT, structlog, Prometheus + OpenTelemetry |
| 鍓嶇 | Vue 3.5, Vite, TypeScript, Ant Design Vue 4, Pinia, Vue Router, @vue-flow(DAG 鐢诲竷), mermaid, KaTeX, ECharts, vitest |
| 瀛樺偍 | PostgreSQL 16(pgvector), Redis 7, MinIO / S3, Milvus 2.5, Temporal |

## 馃搧 鐩綍缁撴瀯

```
鈹溾攢鈹€ cmd/                  # Go 鍏ュ彛(chiron 缃戝叧 / migrate / chiron-cli / stress)
鈹溾攢鈹€ config/               # 缃戝叧閰嶇疆鍔犺浇(.env + config.json)
鈹溾攢鈹€ internal/             # Go 缃戝叧:api / auth / billing / broadcast / db / engine
鈹?                        #          / enterprise / id / model / monitor / session / storage
鈹溾攢鈹€ python-engine/        # Python AI 寮曟搸:app/{agent, core, gateway, llm, mcp, rag,
鈹?                        #          skill, workflow, tools, knowledge, memory, media, ...}
鈹溾攢鈹€ frontend-vue/         # Vue 3 + Vite + TS 鍓嶇(src/{views, components, router, stores, api})
鈹溾攢鈹€ migrations/           # 鏁版嵁搴撹縼绉?Atlas,鍗曟枃浠跺熀绾?+ 澧為噺)
鈹溾攢鈹€ skills/               # 鍐呯疆鎶€鑳藉畾涔?鈹溾攢鈹€ docs/                 # openapi.yaml銆佹灦鏋勬枃妗?鈹溾攢鈹€ deploy/               # 閮ㄧ讲杈呭姪
鈹溾攢鈹€ data/plugins/         # 姣忕敤鎴?MCP 鎻掍欢閰嶇疆(杩愯鏃?
鈹溾攢鈹€ docker-compose.yml    # 鍏ㄦ爤缂栨帓:postgres/redis/minio/temporal/milvus/gateway/engine/frontend
鈹溾攢鈹€ run.py                # 鏈湴寮€鍙戠紪鎺?start/stop/status/logs/build/setup)
鈹溾攢鈹€ Makefile              # build / test / lint / fmt / docker-build
鈹斺攢鈹€ atlas.hcl             # 杩佺Щ宸ュ叿閰嶇疆
```

## 馃椇锔?璺嚎鍥?
| Now(宸插畬鎴? | Next | Later |
|---|---|---|
| 鍏ぇ宸ヤ綔鍙颁簰鑱斾簰閫?TaskRouter 缁熶竴缂栨帓) | CI 闆嗘垚娴嬭瘯 job(Postgres/Redis services) | 鎻掍欢 SDK 涓庡紑鍙戣€呯敓鎬?|
| Redis 蹇呴渶鍖?fail-fast,鏃犻檷绾фā寮? | 瑕嗙洊鐜囨帴鍏?Codecov 涓?badge | 瀹瑰櫒绾ф矙绠?gVisor)寮哄寲 |
| 鎶€鑳?/ Agent / MCP 涓夊ぇ甯傚満 | GHCR 闀滃儚鍙戝竷涓庣増鏈寲 Release | Helm / K8s 鐢熶骇閮ㄧ讲 |
| 濯掍綋绛惧悕 URL 鍏ㄩ摼璺?| 鍓嶇 i18n 鍥介檯鍖?| 澶氬尯鍩熼珮鍙敤涓庡彧璇诲壇鏈?|
| 澶氱鎴烽殧绂?+ 瀹夊叏鍔犲浐(SSRF / 鍛戒护鐧藉悕鍗?/ 娉ㄥ叆妫€娴? | 鏂囨。绔欎笌鏇村鍐呯疆鎶€鑳?| 璁¤垂鏀粯鐢熶骇鍖?鏀粯瀹?/ 寰俊 / PayPal) |
| 涓夌 CI 娴佹按绾?go vet/test / ruff+pytest / vue-tsc+build) | 浼佷笟 SSO / SCIM 瀹屽杽 | 璇箟缂撳瓨涓?RAG 鏁堟灉璇勪及宸ュ叿 |

## 馃 鍙備笌璐＄尞

娆㈣繋鎻愪氦 Issue 涓?PR!璇峰厛闃呰 [CONTRIBUTING.md](CONTRIBUTING.md)(寮€鍙戠幆澧冦€佷唬鐮佽鑼冦€佹彁浜や俊鎭害瀹?涓?[SECURITY.md](SECURITY.md)(婕忔礊鎶ュ憡娴佺▼)銆?
## 馃摳 鎴浘

鎴浘鍗犱綅:璇峰皢鐣岄潰鎴浘(棣栭〉 / 瀵硅瘽 / 宸ヤ綔娴佺敾甯?/ 鐭ヨ瘑搴?/ 绠＄悊鍚庡彴绛?鏀惧叆 `docs/screenshots/` 鐩綍,骞跺湪 `README` 鐨?Screenshots 灏忚妭寮曠敤銆傛垜浠鍒掓敹褰?鐧诲綍涓庡畨瑁呭悜瀵笺€佸叚宸ヤ綔鍙伴椤点€佸璇濇祦寮忚緭鍑轰笌宸ュ叿閾捐繕鍘熴€佸伐浣滄祦 DAG 鐢诲竷銆佸獟浣撳簱涓庣鍚嶅垎浜€佷紒涓氬競鍦轰笌閰嶉鐪嬫澘銆?
## 馃摎 鏂囨。

- [鏋舵瀯鏂囨。](docs/ARCHITECTURE.md)(鍒嗗眰銆佽璇侀摼璺€佺粺涓€鍏ュ彛銆佸绉熸埛闅旂鐭╅樀銆佸獟浣撶鍚嶆祦绋?
- [OpenAPI](docs/openapi.yaml)

## License

[Apache-2.0](LICENSE)

