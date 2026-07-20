package api

import (
	"context"
	"testing"
	"time"
)

func TestRPAHub_RegisterUnregister(t *testing.T) {
	hub := NewRPAHub()

	client := &RPAClient{
		ID:       "test-client-1",
		UserID:   "user-1",
		LastSeen: time.Now(),
	}

	hub.Register(client)

	got, ok := hub.GetClient("test-client-1")
	if !ok || got.ID != "test-client-1" {
		t.Fatal("expected client to be registered")
	}

	hub.Unregister("test-client-1")
	_, ok = hub.GetClient("test-client-1")
	if ok {
		t.Fatal("expected client to be unregistered")
	}
}

func TestRPAHub_GetClientByUser(t *testing.T) {
	hub := NewRPAHub()

	c1 := &RPAClient{ID: "c1", UserID: "user-1", LastSeen: time.Now().Add(-time.Minute)}
	c2 := &RPAClient{ID: "c2", UserID: "user-1", LastSeen: time.Now()}

	hub.Register(c1)
	hub.Register(c2)

	got, ok := hub.GetClientByUser("user-1")
	if !ok || got.ID != "c2" {
		t.Fatalf("expected most recent client c2, got %v", got)
	}

	_, ok = hub.GetClientByUser("user-999")
	if ok {
		t.Fatal("expected no client for unknown user")
	}
}

func TestRPAHub_HandleResult(t *testing.T) {
	hub := NewRPAHub()

	ch := make(chan *RPAResult, 1)
	hub.pending["msg-123"] = ch

	msg := &RPAMessage{
		Type: RPAMsgResult,
		ID:   "msg-123",
		Result: map[string]interface{}{
			"clicked": true,
		},
	}

	go hub.HandleResult(msg)

	select {
	case result := <-ch:
		if result.Error != nil {
			t.Fatalf("unexpected error: %v", result.Error)
		}
		if result.Result["clicked"] != true {
			t.Fatal("expected clicked=true")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for result")
	}
}

func TestRPAError_Error(t *testing.T) {
	err := &RPAError{Code: 404, Message: "element not found"}
	expected := "rpa error 404: element not found"
	if err.Error() != expected {
		t.Fatalf("expected %q, got %q", expected, err.Error())
	}
}

func TestRPAHub_SendCommand_Timeout(t *testing.T) {
	hub := NewRPAHub()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := hub.SendCommand(ctx, "nonexistent", &RPACommand{Method: "test"})
	if err == nil {
		t.Fatal("expected error for nonexistent client")
	}
}

func TestRPAHub_ConnectedClients(t *testing.T) {
	hub := NewRPAHub()

	if len(hub.ConnectedClients()) != 0 {
		t.Fatal("expected empty client list")
	}

	hub.Register(&RPAClient{ID: "c1", UserID: "u1", LastSeen: time.Now()})
	hub.Register(&RPAClient{ID: "c2", UserID: "u2", LastSeen: time.Now()})

	clients := hub.ConnectedClients()
	if len(clients) != 2 {
		t.Fatalf("expected 2 clients, got %d", len(clients))
	}
}

func TestRPAMessage_JSON(t *testing.T) {
	msg := RPAMessage{
		Type:   RPAMsgCommand,
		ID:     "cmd-1",
		Method: "browser_click",
		Params: map[string]interface{}{"selector": "#btn"},
		TabID:  42,
		TS:     1234567890,
	}

	if msg.Type != RPAMsgCommand {
		t.Fatal("unexpected type")
	}
	if msg.Method != "browser_click" {
		t.Fatal("unexpected method")
	}
	if msg.Params["selector"] != "#btn" {
		t.Fatal("unexpected param")
	}
}
