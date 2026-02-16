package channel_test

import (
	"testing"

	"ccgateway/internal/channel"
)

func TestAbilityStoreUpdateRebuildsAbilitiesWhenModelsChange(t *testing.T) {
	store := channel.NewAbilityStore()
	baseURL := "https://api.example.com/v1"
	err := store.AddChannel(&channel.Channel{
		Name:    "adapter-a",
		Type:    "openai",
		BaseURL: &baseURL,
		Models:  "model-a",
		Group:   "default",
		Status:  channel.StatusEnabled,
		Weight:  100,
	})
	if err != nil {
		t.Fatalf("add channel: %v", err)
	}

	ch, ok := store.GetChannelByGroupAndModel("default", "model-a")
	if !ok || ch == nil {
		t.Fatalf("expected channel for model-a")
	}

	update, ok := store.GetChannel(ch.ID)
	if !ok || update == nil {
		t.Fatalf("get channel for update failed")
	}
	update.Models = "model-b"
	if err := store.UpdateChannel(update); err != nil {
		t.Fatalf("update channel: %v", err)
	}

	if _, ok := store.GetChannelByGroupAndModel("default", "model-a"); ok {
		t.Fatalf("expected model-a route to be removed after model update")
	}
	if got, ok := store.GetChannelByGroupAndModel("default", "model-b"); !ok || got == nil {
		t.Fatalf("expected model-b route after model update")
	}
}

func TestAbilityStoreGetReturnsClone(t *testing.T) {
	store := channel.NewAbilityStore()
	modelMapping := "old:new"
	err := store.AddChannel(&channel.Channel{
		Name:         "adapter-b",
		Type:         "openai",
		Models:       "model-a",
		Group:        "default",
		Status:       channel.StatusEnabled,
		Weight:       100,
		ModelMapping: &modelMapping,
	})
	if err != nil {
		t.Fatalf("add channel: %v", err)
	}

	original, ok := store.GetChannel(1)
	if !ok || original == nil {
		t.Fatalf("expected stored channel")
	}
	original.Name = "mutated-name"
	if original.ModelMapping != nil {
		*original.ModelMapping = "mutated:mapping"
	}

	again, ok := store.GetChannel(1)
	if !ok || again == nil {
		t.Fatalf("expected stored channel on second get")
	}
	if again.Name == "mutated-name" {
		t.Fatalf("expected store to keep original name, got mutated copy")
	}
	if again.ModelMapping != nil && *again.ModelMapping == "mutated:mapping" {
		t.Fatalf("expected store to keep original model mapping, got mutated copy")
	}
}
