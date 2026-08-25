package main

// 鏈湴鏈嶅姟瀹炰緥鐘舵€佺鐞?鈥?.pids/state.json
// start 鍐欏叆銆乻top 娓呴櫎銆乮nstance/logs 璇诲彇

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// InstanceState 鎻忚堪涓€涓湰鍦版湇鍔″疄渚?type InstanceState struct {
	Name      string `json:"name"`       // 鏈嶅姟鍚嶏紝濡?gateway / python-engine
	PID       int    `json:"pid"`        // 杩涚▼ PID
	Port      int    `json:"port"`       // 鐩戝惉绔彛锛? = 鏈煡锛?	Mode      string `json:"mode"`       // monolith | microservices
	StartedAt string `json:"started_at"` // RFC3339
	LogFile   string `json:"log_file"`   // 鏃ュ織鏂囦欢锛堢浉瀵规垨缁濆璺緞锛?}

// State 鎸佷箙鍖栫粨鏋?type State struct {
	Instances []InstanceState `json:"instances"`
}

// stateFilePath 杩斿洖鐘舵€佹枃浠惰矾寰勶紙鍩轰簬褰撳墠宸ヤ綔鐩綍锛屼笌 run.py 鐨?.pids/ 绾﹀畾涓€鑷达級
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
		return nil, fmt.Errorf("璇诲彇鐘舵€佹枃浠跺け璐? %w", err)
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("瑙ｆ瀽鐘舵€佹枃浠跺け璐?(%s): %w", path, err)
	}
	if s.Instances == nil {
		s.Instances = []InstanceState{}
	}
	return &s, nil
}

func saveState(s *State) error {
	path := stateFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("鍒涘缓 .pids 鐩綍澶辫触: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("搴忓垪鍖栫姸鎬佸け璐? %w", err)
	}
	// 鍘熷瓙鍐欙細tmp + rename锛岄伩鍏嶅啓涓€鍗婂穿婧冪暀涓嬫崯鍧忕殑 JSON
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("鍐欏叆鐘舵€佹枃浠跺け璐? %w", err)
	}
	return os.Rename(tmp, path)
}

// UpsertInstance 鎸夊悕绉版柊澧炴垨鏇存柊瀹炰緥
func (s *State) UpsertInstance(inst InstanceState) {
	for i := range s.Instances {
		if s.Instances[i].Name == inst.Name {
			s.Instances[i] = inst
			return
		}
	}
	s.Instances = append(s.Instances, inst)
}

// RemoveInstance 鎸夊悕绉扮Щ闄ゅ疄渚嬶紝杩斿洖鏄惁鍛戒腑
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
