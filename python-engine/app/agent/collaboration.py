"""Agent 鍗忓悓宸ヤ綔鍙? 澶?Agent 骞惰鎵ц + 鍏变韩涓婁笅鏂?

鏋舵瀯璁捐:
- AgentHub 浣滀负涓灑,绠＄悊澶氫釜 Agent 瀹炰緥
- 姣忎釜 Agent 鐙珛杩愯 ReAct 寰幆
- 閫氳繃鍏变韩 context store (Redis/PG) 浜ゆ崲鐘舵€?
- Go 缃戝叧缁熶竴閴存潈 + 闄愭祦 + trace 杩借釜

SaaS 瀹夊叏:
- 绉熸埛闅旂: 鎵€鏈?context 鎼哄甫 tenant_id
- 璧勬簮閰嶉: 姣忕鎴锋渶澶氬惎鍔?N 涓苟鍙?Agent
- 閾捐矾杩借釜: 璺?Agent 璇锋眰浼犻€?trace_id
"""
from __future__ import annotations

import asyncio
import json
import logging
import time
from typing import AsyncIterator, Optional
from dataclasses import dataclass, field
from enum import Enum

from app.agent.runtime import AgentRuntime, AgentEvent, CompactionConfig
from app.config import settings
from app.gateway.router import GatewayRouter
from app.trace import record_span

logger = logging.getLogger(__name__)


class AgentRole(str, Enum):
    """Agent 瑙掕壊瀹氫箟"""
    RESEARCHER = "researcher"      # 淇℃伅鏀堕泦涓庣爺绌?
    CODER = "coder"                # 浠ｇ爜鐢熸垚涓庡疄鐜?
    REVIEWER = "reviewer"          # 浠ｇ爜瀹℃煡涓庢祴璇?
    PLANNER = "planner"            # 浠诲姟瑙勫垝涓庢媶瑙?
    ORCHESTRATOR = "orchestrator"  # 缂栨帓鍗忚皟(涓?Agent)


@dataclass
class AgentSpec:
    """Agent 瑙掕壊瑙勬牸瀹氫箟"""
    role: AgentRole
    description: str
    system_prompt: str
    max_turns: int = 10
    model: str = "gpt-4o-mini"
    mode: str = "normal"  # 杩愯妯″紡锛坣ormal/minimal/ptc/creative锛夛紝瑙?app.agent.modes
    compaction_config: Optional[CompactionConfig] = None  # 閫?agent 鎴柇绛栫暐瑕嗙洊


@dataclass
class CollaborativeTask:
    """鍗忓悓浠诲姟瀹氫箟"""
    task_id: str
    original_query: str
    tenant_id: str  # SaaS 瀹夊叏: 绉熸埛闅旂
    trace_id: str  # 閾捐矾杩借釜
    subtasks: list[dict]  # [{agent_role, description, dependencies}]
    shared_context: dict = field(default_factory=dict)  # 鍏变韩涓婁笅鏂?
    status: str = "pending"  # pending/running/completed/failed


class AgentContextStore:
    """鍏变韩涓婁笅鏂囧瓨鍌?(SaaS 绉熸埛闅旂)
    
    瀹炵幇鏂瑰紡:
    - Redis (鍒嗗竷寮忓満鏅?: key = "chiron:context:{tenant_id}:{context_id}"
    - 鍐呭瓨 (鍗曞疄渚嬮檷绾?: dict[tenant_id][context_id] = data
    """
    
    def __init__(self, tenant_id: str):
        self.tenant_id = tenant_id
        self._local_store: dict[str, dict] = {}
        self._redis_client = None  # Lazy load
    
    async def set(self, context_id: str, data: dict, ttl: int = 3600) -> None:
        """璁剧疆涓婁笅鏂?(甯?TTL 鑷姩杩囨湡)"""
        data["_ttl"] = time.time() + ttl
        data["_tenant_id"] = self.tenant_id  # SaaS 瀹夊叏: 鍏冩暟鎹爣璁?
        
        if self._redis_client:
            import redis.asyncio as aioredis
            await self._redis_client.setex(
                f"chiron:context:{self.tenant_id}:{context_id}",
                ttl,
                json.dumps(data, ensure_ascii=False)
            )
        else:
            self._local_store[context_id] = data
    
    async def get(self, context_id: str) -> Optional[dict]:
        """鑾峰彇涓婁笅鏂?""
        if self._redis_client:
            import redis.asyncio as aioredis
            data = await self._redis_client.get(
                f"chiron:context:{self.tenant_id}:{context_id}"
            )
            return json.loads(data) if data else None
        else:
            return self._local_store.get(context_id)
    
    async def delete(self, context_id: str) -> None:
        """鍒犻櫎涓婁笅鏂?""
        if self._redis_client:
            import redis.asyncio as aioredis
            await self._redis_client.delete(
                f"chiron:context:{self.tenant_id}:{context_id}"
            )
        else:
            self._local_store.pop(context_id, None)
    
    async def list_keys(self) -> list[str]:
        """鍒楀嚭璇ョ鎴蜂笅鐨勬墍鏈?context_id"""
        if self._redis_client:
            import redis.asyncio as aioredis
            keys = await self._redis_client.keys(
                f"chiron:context:{self.tenant_id}:*"
            )
            return [k.split(":")[-1] for k in keys]
        else:
            return list(self._local_store.keys())


class AgentHub:
    """Agent 鍗忓悓涓灑
    
    鍔熻兘:
    1. 鎺ユ敹澶嶆潅浠诲姟,鎷嗚В涓哄瓙浠诲姟鍒嗛厤缁欎笓涓?Agent
    2. 绠＄悊 Agent 鐢熷懡鍛ㄦ湡涓庡苟鍙戝害
    3. 鑱氬悎鍚?Agent 杈撳嚭鍒板叡浜笂涓嬫枃
    4. 缁熶竴 trace 璁板綍
    
    SaaS 瀹夊叏:
    - 姣忕鎴峰苟鍙?Agent 鏁伴檺鍒?(榛樿 3)
    - 鎵€鏈?Agent 鍏变韩鍚屼竴 trace_id
    - Context Store 鎸夌鎴烽殧绂?
    """
    
    # 棰勫畾涔?Agent 瑙掕壊閰嶇疆
    AGENT_ROLES: dict[AgentRole, AgentSpec] = {
        AgentRole.RESEARCHER: AgentSpec(
            role=AgentRole.RESEARCHER,
            description="璐熻矗淇℃伅鎼滅储銆佹枃妗ｅ垎鏋愩€佺煡璇嗗簱妫€绱?,
            system_prompt="""浣犳槸涓€涓笓涓氱殑鐮旂┒鍔╂墜銆傝锛?
1. 浠旂粏鍒嗘瀽鐢ㄦ埛闂,鎻愬彇鍏抽敭瀹炰綋鍜屾蹇?
2. 浣跨敤鎼滅储宸ュ叿鑾峰彇鐩稿叧淇℃伅
3. 鏁寸悊骞舵€荤粨鍙戠幇,娉ㄦ槑鏉ユ簮
4. 淇濈暀鏈В鍐崇殑涓嶇‘瀹氭€?"",
            max_turns=15,
        ),
        AgentRole.CODER: AgentSpec(
            role=AgentRole.CODER,
            description="璐熻矗浠ｇ爜鐢熸垚銆佽剼鏈紪鍐欍€佹矙绠辨墽琛?,
            system_prompt="""浣犳槸涓€涓祫娣辩▼搴忓憳銆傝锛?
1. 鏍规嵁闇€姹傝璁℃竻鏅扮殑浠ｇ爜缁撴瀯
2. 缂栧啓鍙墽琛屻€佹湁娉ㄩ噴鐨勪唬鐮?
3. 鍦ㄦ矙绠变腑娴嬭瘯楠岃瘉
4. 鎶ュ憡鎵ц缁撴灉鍜屾綔鍦ㄩ棶棰?"",
            max_turns=20,
        ),
        AgentRole.REVIEWER: AgentSpec(
            role=AgentRole.REVIEWER,
            description="璐熻矗浠ｇ爜瀹℃煡銆佹祴璇曠敤渚嬨€佽川閲忎繚闅?,
            system_prompt="""浣犳槸涓€涓弗鏍肩殑浠ｇ爜瀹℃煡鍛樸€傝锛?
1. 妫€鏌ヤ唬鐮佺殑閫昏緫姝ｇ‘鎬?
2. 璇嗗埆杈圭晫鎯呭喌鍜屽紓甯稿鐞?
3. 寤鸿鎬ц兘浼樺寲
4. 鐢熸垚娴嬭瘯鐢ㄤ緥骞舵墽琛?"",
            max_turns=10,
        ),
        AgentRole.PLANNER: AgentSpec(
            role=AgentRole.PLANNER,
            description="璐熻矗浠诲姟鎷嗚В銆佷緷璧栧垎鏋愩€佽繘搴﹁窡韪?,
            system_prompt="""浣犳槸涓€涓」鐩鍒掍笓瀹躲€傝锛?
1. 灏嗗鏉備换鍔℃媶瑙ｄ负鍙苟琛岀殑瀛愪换鍔?
2. 璇嗗埆瀛愪换鍔￠棿鐨勪緷璧栧叧绯?
3. 鍒跺畾鎵ц椤哄簭鍜岃祫婧愬垎閰?
4. 鐩戞帶杩涘害骞跺姩鎬佽皟鏁磋鍒?"",
            max_turns=5,
        ),
        AgentRole.ORCHESTRATOR: AgentSpec(
            role=AgentRole.ORCHESTRATOR,
            description="璐熻矗鏁翠綋鍗忚皟銆佸啿绐佽В鍐炽€佹渶缁堟眹鎬?,
            system_prompt="""浣犳槸鍗忓悓绯荤粺鐨勬€荤紪鎺掕€呫€傝锛?
1. 鐞嗚В鐢ㄦ埛鍘熷闇€姹?鍒跺畾楂樺眰绛栫暐
2. 鍗忚皟鍚勪笓涓?Agent 鐨勫伐浣?
3. 瑙ｅ喅瀛愪换鍔￠棿鐨勫啿绐?
4. 鑱氬悎杈撳嚭,鐢熸垚鏈€缁堝洖绛?
5. 纭繚璐ㄩ噺涓庝竴鑷存€?"",
            max_turns=10,
        ),
    }
    
    def __init__(self, gateway: GatewayRouter):
        self.gateway = gateway
        self._runtime_pool: dict[AgentRole, AgentRuntime] = {}
        self._max_concurrent_per_tenant = 3  # SaaS 閰嶉
        self._tenant_running_agents: dict[str, int] = {}  # tenant_id -> count
    
    async def run_collaborative_task(
        self,
        task: CollaborativeTask,
    ) -> AsyncIterator[AgentEvent]:
        """鎵ц鍗忓悓浠诲姟
        
        娴佺▼:
        1. Planner 鎷嗚В浠诲姟 (濡傛灉灏氭湭鎷嗚В)
        2. 骞惰鎵ц鏃犱緷璧栫殑瀛愪换鍔?
        3. 绛夊緟渚濊禆瀹屾垚
        4. Orchestrator 鑱氬悎杈撳嚭
        """
        import uuid as uuid_mod
        
        # 鈹€鈹€ 鐢熸垚 trace_id (濡傛灉涓嶅瓨鍦? 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
        trace_id = task.trace_id or uuid_mod.uuid4().hex[:12]
        
        # 鈹€鈹€ SaaS 瀹夊叏: 妫€鏌ョ鎴峰苟鍙戦厤棰?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
        tenant_id = task.tenant_id
        current_count = self._tenant_running_agents.get(tenant_id, 0)
        if current_count >= self._max_concurrent_per_tenant:
            yield AgentEvent(
                type="error",
                content=f"绉熸埛骞跺彂 Agent 鏁板凡杈句笂闄?({self._max_concurrent_per_tenant})",
                trace_id=trace_id,
            )
            return
        
        self._tenant_running_agents[tenant_id] = current_count + 1
        
        try:
            # 鈹€鈹€ 闃舵 1: 浠诲姟瑙勫垝 (濡傛灉闇€瑕? 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
            if not task.subtasks:
                planner_spec = self.AGENT_ROLES[AgentRole.PLANNER]
                planner_runtime = self._get_or_create_runtime(planner_spec)
                
                planning_query = f"""璇蜂负浠ヤ笅闇€姹傛媶瑙ｄ换鍔?
{task.original_query}

璇蜂互 JSON 鏍煎紡杩斿洖瀛愪换鍔″垪琛?
[
  {{
    "role": "<role>",
    "description": "<鎻忚堪>",
    "dependencies": ["<渚濊禆鐨勫瓙浠诲姟id>"]
  }}
]"""
                
                async for event in planner_runtime.run(
                    task=self._make_agent_task(
                        spec=planner_spec,
                        task_id=f"planning_{trace_id}",
                        content=planning_query,
                        tenant_id=tenant_id,
                    ),
                ):
                    event.trace_id = trace_id
                    yield event
                    
                    # 瑙ｆ瀽 Planner 杈撳嚭,鎻愬彇 subtasks
                    if event.type == "text":
                        task.subtasks = self._parse_planner_output(event.content)
            
            # 鈹€鈹€ 闃舵 2: DAG 璋冨害鎵ц瀛愪换鍔?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
            task.status = "running"

            async for event in self._execute_subtask_dag(task, trace_id, tenant_id):
                event.trace_id = trace_id
                yield event
            
            # 鈹€鈹€ 闃舵 3: Orchestrator 鑱氬悎 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
            orchestrator_spec = self.AGENT_ROLES[AgentRole.ORCHESTRATOR]
            orchestrator_runtime = self._get_or_create_runtime(orchestrator_spec)
            
            aggregation_query = f"""璇锋牴鎹互涓嬪悇涓撲笟 Agent 鐨勮緭鍑?鐢熸垚鏈€缁堝洖绛?

銆愬師濮嬮渶姹傘€?
{task.original_query}

銆愬悇 Agent 杈撳嚭銆?
{json.dumps(task.shared_context, ensure_ascii=False, indent=2)}

璇风患鍚堟暣鐞?鎻愪緵娓呮櫚銆佸噯纭€佸畬鏁寸殑鍥炵瓟銆?""
            
            final_event = AgentEvent(
                type="done",
                content="鍗忓悓浠诲姟瀹屾垚",
                trace_id=trace_id,
            )
            
            async for event in orchestrator_runtime.run(
                task=self._make_agent_task(
                    spec=orchestrator_spec,
                    task_id=f"aggregation_{trace_id}",
                    content=aggregation_query,
                    tenant_id=tenant_id,
                ),
            ):
                event.trace_id = trace_id
                yield event
            
            task.status = "completed"
            final_event.trace_id = trace_id
            yield final_event
            
            logger.info(
                "Collaborative task done (task=%s, trace_id=%s, subtasks=%d)",
                task.task_id, trace_id, len(task.subtasks),
            )
            
        finally:
            # 閲婃斁閰嶉
            self._tenant_running_agents[tenant_id] = max(
                0, self._tenant_running_agents.get(tenant_id, 1) - 1
            )
    
    # 鈹€鈹€ DAG 璋冨害鍣?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

    def _topological_waves(self, subtasks: list[dict]) -> list[list[int]]:
        """灏嗗瓙浠诲姟鎸変緷璧栧叧绯绘嫇鎵戞帓搴忎负鎵ц娉㈡銆?

        姣忎竴娉㈡鍐呯殑瀛愪换鍔℃棤浜掔浉渚濊禆锛屽彲骞跺彂鎵ц銆?
        渚濊禆椤瑰紩鐢ㄦ牸寮? "subtask_N" (N 涓虹储寮?銆?

        杩斿洖 [[idx, ...], [idx, ...], ...]
        """
        n = len(subtasks)
        # 鏋勫缓 dependency map: idx 鈫?set of dependency idx
        deps: dict[int, set[int]] = {}
        for i, st in enumerate(subtasks):
            raw_deps = st.get("dependencies", [])
            dep_indices: set[int] = set()
            for d in raw_deps:
                # 瑙ｆ瀽 "subtask_N" 鏍煎紡
                if isinstance(d, str) and d.startswith("subtask_"):
                    try:
                        dep_indices.add(int(d.split("_", 1)[1]))
                    except (ValueError, IndexError):
                        pass
                elif isinstance(d, int):
                    dep_indices.add(d)
            deps[i] = dep_indices

        # Kahn 绠楁硶鍒嗗眰
        completed: set[int] = set()
        waves: list[list[int]] = []

        while len(completed) < n:
            # 鎵惧嚭鎵€鏈変緷璧栧凡婊¤冻鐨勬湭鎵ц鑺傜偣
            ready = [
                i for i in range(n)
                if i not in completed and deps[i].issubset(completed)
            ]
            if not ready:
                # 渚濊禆鐜細鎵撶牬鐜紝鎸夊師濮嬮『搴忔墽琛屽墿浣欎换鍔?
                remaining = [i for i in range(n) if i not in completed]
                logger.warning(
                    "DAG cycle detected, executing remaining subtasks linearly: %s",
                    remaining,
                )
                waves.append(remaining)
                break
            waves.append(ready)
            completed.update(ready)

        return waves

    async def _execute_subtask_dag(
        self,
        task: CollaborativeTask,
        trace_id: str,
        tenant_id: str,
    ) -> AsyncIterator[AgentEvent]:
        """鍩轰簬 DAG 渚濊禆鍏崇郴璋冨害瀛愪换鍔℃墽琛屻€?

        鎷撴墤鎺掑簭涓烘尝娆★紝姣忔尝鍐呯殑瀛愪换鍔″苟鍙戞墽琛岋紙鍙?Semaphore 闄愬埗锛夈€?
        姣忎釜瀛愪换鍔＄殑瀹為檯鑰楁椂琚褰曞埌 span 涓€?
        """
        waves = self._topological_waves(task.subtasks)
        sem = asyncio.Semaphore(self._max_concurrent_per_tenant)

        for wave_idx, wave in enumerate(waves):
            # 骞跺彂鎵ц褰撳墠娉㈡
            event_queue: asyncio.Queue = asyncio.Queue()
            running_tasks: list[asyncio.Task] = []

            for subtask_idx in wave:
                t = asyncio.create_task(
                    self._run_single_subtask(
                        subtask_idx, task, trace_id, tenant_id, sem, event_queue
                    ),
                    name=f"subtask_{subtask_idx}",
                )
                running_tasks.append(t)

            # 杈规墽琛岃竟 yield 浜嬩欢
            done_count = 0
            total = len(running_tasks)
            while done_count < total:
                try:
                    event = await event_queue.get()
                    if event is None:
                        # 鏌愪釜瀛愪换鍔＄粨鏉熶俊鍙?
                        done_count += 1
                        continue
                    yield event
                except asyncio.CancelledError:
                    for t in running_tasks:
                        t.cancel()
                    raise

            # 绛夊緟鎵€鏈変换鍔″畬鎴愶紙搴旇宸插畬鎴愶級
            await asyncio.gather(*running_tasks, return_exceptions=True)

            logger.debug(
                "DAG wave %d/%d completed (%d subtasks)",
                wave_idx + 1, len(waves), len(wave),
            )

    async def _run_single_subtask(
        self,
        subtask_idx: int,
        task: CollaborativeTask,
        trace_id: str,
        tenant_id: str,
        sem: asyncio.Semaphore,
        event_queue: asyncio.Queue,
    ) -> None:
        """鎵ц鍗曚釜瀛愪换鍔★紝灏嗕簨浠舵帹鍏ラ槦鍒椾緵璋冨害鍣?yield銆?""
        subtask = task.subtasks[subtask_idx]
        role_str = subtask.get("role", "researcher")
        try:
            role = AgentRole(role_str)
        except ValueError:
            role = AgentRole.RESEARCHER

        spec = self.AGENT_ROLES[role]
        runtime = self._get_or_create_runtime(spec)

        # 娉ㄥ叆鍏变韩涓婁笅鏂?
        context_injection = f"""
銆愬叡浜笂涓嬫枃銆?
{json.dumps(task.shared_context.get(role_str, {}), ensure_ascii=False)}

璇风洿鎺ュ洖绛旈棶棰?涓嶈閲嶅宸茬‘璁ょ殑浜嬪疄銆?""

        sub_query = f"""銆愬瓙浠诲姟 {subtask_idx + 1}/{len(task.subtasks)}銆?
{subtask['description']}

{context_injection}

鍘熷鐢ㄦ埛闇€姹? {task.original_query}"""

        start_time = time.time()

        async with sem:
            async for event in runtime.run(
                task=self._make_agent_task(
                    spec=spec,
                    task_id=f"subtask_{subtask_idx}_{trace_id}",
                    content=sub_query,
                    tenant_id=tenant_id,
                ),
            ):
                # 灏嗚緭鍑哄啓鍏ュ叡浜笂涓嬫枃
                if event.type == "text" and event.content:
                    task.shared_context.setdefault(role_str, {})[f"subtask_{subtask_idx}"] = event.content

                # 鎺ㄥ叆浜嬩欢闃熷垪
                await event_queue.put(event)

        # 璁板綍 span锛堝惈瀹為檯鑰楁椂锛?
        duration_ms = int((time.time() - start_time) * 1000)
        await record_span(
            trace_id=trace_id,
            span_name=f"agent:{role_str}",
            duration_ms=duration_ms,
            metadata={
                "subtask_index": subtask_idx,
                "dependencies": subtask.get("dependencies", []),
                "tenant_id": tenant_id,
            },
            tenant_id=tenant_id,
        )

        # 鍙戦€佺粨鏉熶俊鍙?
        await event_queue.put(None)

    def _get_or_create_runtime(self, spec: AgentSpec) -> AgentRuntime:
        """鑾峰彇鎴栧垱寤烘寚瀹氳鑹茬殑 Agent Runtime

        mode/compaction 涓嶅湪鏋勯€犳湡娉ㄥ叆鈥斺€擜gentRuntime.run() 鎸変换鍔?
        浠?task.llm_config 瑙ｆ瀽锛堣 runtime.py: get_mode_config锛夛紝
        鐢?_make_agent_task 璐熻矗鎶?spec 鐨?mode/model/compaction 瑁呰繘浠诲姟銆?
        """
        if spec.role not in self._runtime_pool:
            from app.agent.runtime import AgentRuntime
            self._runtime_pool[spec.role] = AgentRuntime(gateway=self.gateway)
        return self._runtime_pool[spec.role]

    def _make_agent_task(
        self,
        spec: AgentSpec,
        task_id: str,
        content: str,
        tenant_id: str,
    ):
        """鎸?spec 鏋勯€犲畬鏁寸殑 AgentTask锛堝惈 mode/model/compaction 閰嶇疆娉ㄥ叆锛夈€?

        鍘嗗彶缂洪櫡淇锛氭鍓嶇敤 type('obj', ...) 浼€犱换鍔″璞★紝缂?
        llm_config/user_id/session_id 绛夊睘鎬э紝runtime.run() 涓€杩涘叆灏?
        AttributeError锛涗笖璋冪敤杩囦笉瀛樺湪鐨?run_single_turn銆?
        """
        from dataclasses import asdict
        from app.agent.runtime import AgentTask

        llm_config: dict = {"mode": spec.mode, "model": spec.model}
        if spec.compaction_config is not None:
            # 閫?agent 鎴柇绛栫暐瑕嗙洊锛坮untime 渚ф寜 llm_config["compaction"] 璇诲彇锛?
            llm_config["compaction"] = asdict(spec.compaction_config)

        return AgentTask(
            id=task_id,
            tenant_id=tenant_id,
            user_id=f"collab:{tenant_id}",
            session_id=f"collab:{task_id}",
            content=content,
            system_prompt=spec.system_prompt,
            llm_config=llm_config,
            max_turns=spec.max_turns,
        )
    
    def _parse_planner_output(self, output: str) -> list[dict]:
        """瑙ｆ瀽 Planner 鐨?JSON 杈撳嚭"""
        import re
        
        # 灏濊瘯鎻愬彇 JSON 鏁扮粍
        match = re.search(r'\[.*\]', output, re.DOTALL)
        if match:
            try:
                subtasks = json.loads(match.group())
                return subtasks
            except json.JSONDecodeError:
                logger.warning("Failed to parse planner output as JSON")
        
        # 闄嶇骇: 杩斿洖鍗曚换鍔?
        return [{"role": "researcher", "description": output, "dependencies": []}]


