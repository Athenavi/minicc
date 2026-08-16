package main

// 本地服务实例状态管理 — .pids/state.json
// start 写入、stop 清除、instance/logs 读取

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// InstanceState 描述一个本地服务实例
type InstanceState struct {
	Name      string `json:"name"`       // 服务名，如 gateway / python-engine
	PID       int    `json:"pid"`        // 进程 PID
	Port      int    `json:"port"`       // 监听端口（0 = 未知）
	Mode      string `json:"mode"`       // monolith | microservices
	StartedAt string `json:"started_at"` // RFC3339
	LogFile   string `json:"log_file"`   // 日志文件（相对或绝对路径）
}

// State 持久化结构
type State struct {
	Instances []InstanceState `json:"instances"`
}

// stateFilePath 返回状态文件路径（基于当前工作目录，与 run.py 的 .pids/ 约定一致）
func stateFilePath() string {
	cwd, err := os.Getwd()
	if err != nil {
		return filepath.Join(".", ".pids", "state.json")
	}
	return filepath.Join(cwd, ".pids", "state.json")
}

func loadState() (*State, error) {
	path := stateFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &State{Instances: []InstanceState{}}, nil
		}
		return nil, fmt.Errorf("读取状态文件失败: %w", err)
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("解析状态文件失败 (%s): %w", path, err)
	}
	if s.Instances == nil {
		s.Instances = []InstanceState{}
	}
	return &s, nil
}

func saveState(s *State) error {
	path := stateFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建 .pids 目录失败: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化状态失败: %w", err)
	}
	// 原子写：tmp + rename，避免写一半崩溃留下损坏的 JSON
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("写入状态文件失败: %w", err)
	}
	return os.Rename(tmp, path)
}

// UpsertInstance 按名称新增或更新实例
func (s *State) UpsertInstance(inst InstanceState) {
	for i := range s.Instances {
		if s.Instances[i].Name == inst.Name {
			s.Instances[i] = inst
			return
		}
	}
	s.Instances = append(s.Instances, inst)
}

// RemoveInstance 按名称移除实例，返回是否命中
func (s *State) RemoveInstance(name string) bool {
	for i, inst := range s.Instances {
		if inst.Name == name {
			s.Instances = append(s.Instances[:i], s.Instances[i+1:]...)
			return true
		}
	}
	return false
}

func (s *State) FindInstance(name string) *InstanceState {
	for i := range s.Instances {
		if s.Instances[i].Name == name {
			return &s.Instances[i]
		}
	}
	return nil
}

func newInstance(name string, pid, port int, mode, logFile string) InstanceState {
	return InstanceState{
		Name:      name,
		PID:       pid,
		Port:      port,
		Mode:      mode,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		LogFile:   logFile,
	}
}
