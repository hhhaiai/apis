package memory_test

import (
	"context"
	"testing"
	"time"

	"ccgateway/internal/memory"
)

func TestInMemoryStore_WorkingMemory(t *testing.T) {
	store := memory.NewInMemoryStore()
	ctx := context.Background()
	sessionID := "test-session-1"

	// Test Get (should return empty)
	wm, err := store.GetWorkingMemory(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetWorkingMemory failed: %v", err)
	}
	if wm.SessionID != sessionID {
		t.Errorf("Expected SessionID %s, got %s", sessionID, wm.SessionID)
	}
	if len(wm.Messages) != 0 {
		t.Errorf("Expected empty messages, got %d", len(wm.Messages))
	}

	// Test Update
	wm.Messages = append(wm.Messages, memory.Message{
		Role:      "user",
		Content:   "Hello",
		Timestamp: time.Now(),
	})
	wm.TokenCount = 10

	err = store.UpdateWorkingMemory(ctx, wm)
	if err != nil {
		t.Fatalf("UpdateWorkingMemory failed: %v", err)
	}

	// Test Get (should return updated)
	wm2, err := store.GetWorkingMemory(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetWorkingMemory failed: %v", err)
	}
	if len(wm2.Messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(wm2.Messages))
	}
	if wm2.TokenCount != 10 {
		t.Errorf("Expected TokenCount 10, got %d", wm2.TokenCount)
	}
}

func TestInMemoryStore_SessionMemory(t *testing.T) {
	store := memory.NewInMemoryStore()
	ctx := context.Background()
	sessionID := "test-session-2"

	// Test Get (should return empty)
	sm, err := store.GetSessionMemory(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetSessionMemory failed: %v", err)
	}
	if sm.SessionID != sessionID {
		t.Errorf("Expected SessionID %s, got %s", sessionID, sm.SessionID)
	}

	// Test Update
	sm.Summary = "Test summary"
	sm.ProjectMeta["language"] = "Go"
	sm.FileOperations = append(sm.FileOperations, memory.FileOp{
		Action:    "create",
		Path:      "/test/file.go",
		Timestamp: time.Now(),
	})

	err = store.UpdateSessionMemory(ctx, sm)
	if err != nil {
		t.Fatalf("UpdateSessionMemory failed: %v", err)
	}

	// Test Get (should return updated)
	sm2, err := store.GetSessionMemory(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetSessionMemory failed: %v", err)
	}
	if sm2.Summary != "Test summary" {
		t.Errorf("Expected summary 'Test summary', got '%s'", sm2.Summary)
	}
	if len(sm2.FileOperations) != 1 {
		t.Errorf("Expected 1 file operation, got %d", len(sm2.FileOperations))
	}
}

func TestInMemoryStore_LongTermMemory(t *testing.T) {
	store := memory.NewInMemoryStore()
	ctx := context.Background()
	userID := "test-user-1"

	// Test Get (should return empty)
	ltm, err := store.GetLongTermMemory(ctx, userID)
	if err != nil {
		t.Fatalf("GetLongTermMemory failed: %v", err)
	}
	if ltm.UserID != userID {
		t.Errorf("Expected UserID %s, got %s", userID, ltm.UserID)
	}

	// Test Update
	ltm.CodingStyle = "functional"
	ltm.TechStack = []string{"Go", "Python", "JavaScript"}
	ltm.ProjectHistory = append(ltm.ProjectHistory, memory.ProjectSummary{
		Name:        "Test Project",
		Description: "A test project",
		TechStack:   []string{"Go"},
		Timestamp:   time.Now(),
	})

	err = store.UpdateLongTermMemory(ctx, ltm)
	if err != nil {
		t.Fatalf("UpdateLongTermMemory failed: %v", err)
	}

	// Test Get (should return updated)
	ltm2, err := store.GetLongTermMemory(ctx, userID)
	if err != nil {
		t.Fatalf("GetLongTermMemory failed: %v", err)
	}
	if ltm2.CodingStyle != "functional" {
		t.Errorf("Expected CodingStyle 'functional', got '%s'", ltm2.CodingStyle)
	}
	if len(ltm2.TechStack) != 3 {
		t.Errorf("Expected 3 tech stack items, got %d", len(ltm2.TechStack))
	}
}

func TestInMemoryStore_CleanupExpired(t *testing.T) {
	store := memory.NewInMemoryStore()
	ctx := context.Background()

	// Add some working memory with explicit timestamps
	wm1 := &memory.WorkingMemory{
		SessionID:  "session-1",
		Messages:   []memory.Message{{Role: "user", Content: "test"}},
		LastUpdate: time.Now().Add(-2 * time.Hour),
	}
	store.UpdateWorkingMemory(ctx, wm1)

	wm2 := &memory.WorkingMemory{
		SessionID:  "session-2",
		Messages:   []memory.Message{{Role: "user", Content: "test"}},
		LastUpdate: time.Now(),
	}
	store.UpdateWorkingMemory(ctx, wm2)

	// Verify both exist before cleanup
	statsBefore := store.GetStats()
	if statsBefore["working_memory_count"] != 2 {
		t.Errorf("Expected 2 working memory before cleanup, got %d", statsBefore["working_memory_count"])
	}

	// Cleanup expired (older than 1 hour)
	err := store.CleanupExpired(ctx, 1*time.Hour)
	if err != nil {
		t.Fatalf("CleanupExpired failed: %v", err)
	}

	// Check stats after cleanup
	statsAfter := store.GetStats()
	if statsAfter["working_memory_count"] != 1 {
		t.Errorf("Expected 1 working memory after cleanup, got %d", statsAfter["working_memory_count"])
	}

	// Verify session-2 still exists and has messages
	wm, err := store.GetWorkingMemory(ctx, "session-2")
	if err != nil {
		t.Fatalf("GetWorkingMemory failed: %v", err)
	}
	if wm.SessionID != "session-2" {
		t.Errorf("Expected session-2 to still exist")
	}
	if len(wm.Messages) != 1 {
		t.Errorf("Expected session-2 to have 1 message, got %d", len(wm.Messages))
	}
}

func TestInMemoryStore_GetStats(t *testing.T) {
	store := memory.NewInMemoryStore()
	ctx := context.Background()

	// Initially empty
	stats := store.GetStats()
	if stats["working_memory_count"] != 0 {
		t.Errorf("Expected 0 working memory, got %d", stats["working_memory_count"])
	}

	// Add some data
	store.UpdateWorkingMemory(ctx, &memory.WorkingMemory{SessionID: "s1"})
	store.UpdateWorkingMemory(ctx, &memory.WorkingMemory{SessionID: "s2"})
	store.UpdateSessionMemory(ctx, &memory.SessionMemory{SessionID: "s1"})
	store.UpdateLongTermMemory(ctx, &memory.LongTermMemory{UserID: "u1"})

	stats = store.GetStats()
	if stats["working_memory_count"] != 2 {
		t.Errorf("Expected 2 working memory, got %d", stats["working_memory_count"])
	}
	if stats["session_memory_count"] != 1 {
		t.Errorf("Expected 1 session memory, got %d", stats["session_memory_count"])
	}
	if stats["long_term_memory_count"] != 1 {
		t.Errorf("Expected 1 long term memory, got %d", stats["long_term_memory_count"])
	}
}
