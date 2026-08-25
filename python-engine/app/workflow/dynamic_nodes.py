"""宸ヤ綔娴佽妭鐐瑰姩鎬佺粦瀹?Skill/MCP (WorkflowNode Skill Binding)

鍔熻兘:
- 宸ヤ綔娴佽妭鐐规敮鎸佸姩鎬佺粦瀹氫换鎰忓凡娉ㄥ唽鑳藉姏
- 杩愯鏃朵粠 Capabilities Registry 鏌ヨ鍙敤 Skill
- 鏀寔鑺傜偣绾у弬鏁扮儹鏇存柊
"""
from __future__ import annotations

import json
import logging
import time
from typing import Any, Optional
from dataclasses import dataclass, field

from app.core.capabilities import get_registry, Capability, WorkstationType

logger = logging.getLogger(__name__)


@dataclass
class BoundSkill:
    """缁戝畾鐨勬妧鑳?""
    capability_id: str
    name: str
    config: dict[str, Any] = field(default_factory=dict)
    input_mapping: dict[str, str] = field(default_factory=dict)  # {"node_output_key": "skill_input_param"}
    output_mapping: dict[str, str] = field(default_factory=dict)  # {"skill_output_key": "next_node_input_key"}


class WorkflowNodeWithSkill:
    """甯︽妧鑳界粦瀹氱殑宸ヤ綔娴佽妭鐐?    
    澧炲己鍘熺増 Node,鏀寔鍔ㄦ€佺粦瀹氫换鎰?Skill
    """
    
    def __init__(
        self,
        node_id: str,
        node_type: str,
        label: str,
        bound_skill_id: Optional[str] = None,
        skill_config: Optional[dict] = None,
    ):
        self.node_id = node_id
        self.node_type = node_type  # "input" / "llm" / "tool" / "condition" / "output" / "skill"
        self.label = label
        
        # 鎶€鑳界粦瀹?        self.bound_skill: Optional[BoundSkill] = None
        if bound_skill_id:
            self.bound_skill = BoundSkill(
                capability_id=bound_skill_id,
                name=bound_skill_id.split(":")[-1],
                config=skill_config or {},
            )
        
        self.execution_count = 0
        self.last_execution_time = 0
        self.last_status: str = "pending"  # pending/running/completed/error
    
    async def execute_with_skill(
        self,
        state: dict[str, Any],
        tenant_id: str,
        trace_id: str = "",
    ) -> dict[str, Any]:
        """浣跨敤缁戝畾鐨勬妧鑳芥墽琛岃妭鐐?        
        娴佺▼:
        1. 浠?state 鎻愬彇杈撳叆 (搴旂敤 input_mapping)
        2. 璋冪敤鑳藉姏娉ㄥ唽涓績鏌ユ壘 executor
        3. 鎵ц骞惰幏鍙栬緭鍑?        4. 搴旂敤 output_mapping 娉ㄥ叆鍒?state
        """
        from app.core.capabilities import get_registry
        
        if not self.bound_skill:
            raise ValueError(f"No skill bound to node {self.node_id}")
        
        registry = get_registry()
        cap = await registry.get_by_id(self.bound_skill.capability_id, tenant_id)
        
        if not cap or not cap._executor:
            raise ValueError(f"Capability not found or no executor: {self.bound_skill.capability_id}")
        
        # 鎻愬彇杈撳叆
        input_params = {}
        for out_key, param_name in self.bound_skill.input_mapping.items():
            if out_key in state:
                input_params[param_name] = state[out_key]
        
        # 鍚堝苟鏄惧紡閰嶇疆
        input_params.update(self.bound_skill.config)
        
        # 鎵ц
        start_time = time.time()
        try:
            if hasattr(cap._executor, "__call__"):
                import asyncio
                if asyncio.iscoroutinefunction(cap._executor):
                    output = await cap._executor(**input_params)
                else:
                    output = cap._executor(**input_params)
            else:
                raise ValueError("Executor is not callable")
            
            duration_ms = int((time.time() - start_time) * 1000)
            
            self.execution_count += 1
            self.last_execution_time = time.time()
            self.last_status = "completed"
            
            # 搴旂敤杈撳嚭鏄犲皠
            result = {}
            for skill_key, state_key in self.bound_skill.output_mapping.items():
                if skill_key in output:
                    result[state_key] = output[skill_key]
            
            # 濡傛灉娌℃湁鏄犲皠,鐩存帴杩斿洖瀹屾暣杈撳嚭
            if not self.bound_skill.output_mapping:
                result[f"__out_{self.node_id}__"] = output
            
            logger.info(
                "Node %s executed with skill %s (duration=%dms)",
                self.node_id, self.bound_skill.capability_id, duration_ms,
            )
            
            return result
            
        except Exception as e:
            duration_ms = int((time.time() - start_time) * 1000)
            
            self.execution_count += 1
            self.last_execution_time = time.time()
            self.last_status = "error"
            
            logger.error(f"Node {self.node_id} execution failed: {e}")
            
            return {
                f"__out_{self.node_id}__": {
                    "error": str(e),
                    "duration_ms": duration_ms,
                }
            }


# 鈹€鈹€ 澧炲己鐨勫伐浣滄祦寮曟搸 (鏀寔鍔ㄦ€佽妭鐐? 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
class DynamicWorkflowEngine:
    """鍔ㄦ€佸伐浣滄祦寮曟搸 (鏀寔鑺傜偣缁戝畾浠绘剰 Skill)
    
    鐢ㄦ硶:
    1. 鍒涘缓 WorkflowNodeWithSkill 瀹炰緥
    2. 娣诲姞鍒板伐浣滄祦鍥句腑
    3. 鎵ц鏃惰嚜鍔ㄨ皟鐢ㄧ粦瀹氱殑 Skill
    """
    
    def __init__(self):
        self.nodes: dict[str, WorkflowNodeWithSkill] = {}
        self.registry = get_registry()
    
    def add_node(
        self,
        node_id: str,
        node_type: str,
        label: str,
        bound_skill_id: Optional[str] = None,
        skill_config: Optional[dict] = None,
    ) -> WorkflowNodeWithSkill:
        """娣诲姞鑺傜偣 (鍙€夌粦瀹?Skill)"""
        node = WorkflowNodeWithSkill(
            node_id=node_id,
            node_type=node_type,
            label=label,
            bound_skill_id=bound_skill_id,
            skill_config=skill_config,
        )
        self.nodes[node_id] = node
        return node
    
    async def discover_and_bind_skills(
        self,
        node_type: str = "skill",
        tenant_id: str = "",
    ) -> list[Capability]:
        """鑷姩鍙戠幇骞舵帹鑽愬彲鐢?Skills"""
        caps = await self.registry.list_by_workstation(
            workstation_type=WorkstationType.SKILL,
            tenant_id=tenant_id,
        )
        
        return caps


# 鈹€鈹€ Go 渚у揩鎹锋墽琛?API Handler 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
"""
Go internal/api/quick_execute_handler.go

package api

import (
	"encoding/json"
	"net/http"
	
	"github.com/athenavi/chiron/internal/auth"
)

// QuickExecuteRequest represents a quick execute request.
type QuickExecuteRequest struct {
	UserInput string `json:"user_input"`
	TenantID  string `json:"tenant_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Mode      string `json:"mode"` // "auto" / "agent" / "workflow"
}

// QuickExecuteHandler proxies natural language requests to Python TaskRouter.
// POST /v1/quick-execute
func (h *GatewayHandler) QuickExecuteHandler(w http.ResponseWriter, r *http.Request) {
	claims := getAuthClaims(r, nil)
	if claims == nil {
		Unauthorized(w, "authentication required")
		return
	}
	
	var req QuickExecuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "invalid request body")
		return
	}
	
	// 鏋勫缓 Python 璇锋眰
	pythonReq := map[string]any{
		"user_input": req.UserInput,
		"tenant_id":  claims.TenantID,
		"session_id": req.SessionID,
		"mode":       req.Mode,
	}
	
	// Proxy to Python unified_executor
	var resp map[string]any
	if err := h.pythonClient.DoJSON(r.Context(), http.MethodPost, "/v1/chat/submit", pythonReq, &resp); err != nil {
		InternalError(w, "python engine unavailable")
		return
	}
	
	OK(w, resp)
}

// RegisterQuickExecuteRoute registers the quick execute endpoint.
func RegisterQuickExecuteRoute(mux *http.ServeMux, h *GatewayHandler, authMW func(http.Handler) http.Handler, rlMW func(http.Handler) http.Handler) {
	mux.Handle("POST /v1/quick-execute", authMW(rlMW(http.HandlerFunc(h.QuickExecuteHandler))))
}
"""

