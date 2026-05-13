package storetest

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/store"
)

// Run executes the full Store conformance suite against the store
// returned by factory. Each sub-test gets a fresh store instance.
func Run(t *testing.T, factory func(t *testing.T) store.Store) {
	t.Run("AgentCRUDRoundTrip", func(t *testing.T) {
		s := factory(t)

		agent := &store.Agent{
			Name:         "test-agent",
			Instructions: "You are a test agent.",
			Tools:        []string{"bash", "read"},
			Provider:     "anthropic",
			Model:        "claude-sonnet",
			Options:      map[string]any{"temperature": 0.7},
			OutputSchema: map[string]any{"type": "object"},
		}

		if err := s.CreateAgent(agent); err != nil {
			t.Fatalf("create agent failed: %v", err)
		}
		if agent.ID == "" {
			t.Fatal("expected agent ID to be set")
		}
		if !strings.HasPrefix(agent.ID, "agt_") {
			t.Errorf("expected ID prefix 'agt_', got %q", agent.ID)
		}

		originalUpdatedAt := agent.UpdatedAt

		got, err := s.GetAgent(agent.ID)
		if err != nil {
			t.Fatalf("get agent failed: %v", err)
		}
		if got.Name != agent.Name {
			t.Errorf("name mismatch: got %q, want %q", got.Name, agent.Name)
		}
		if got.Instructions != agent.Instructions {
			t.Errorf("instructions mismatch: got %q, want %q", got.Instructions, agent.Instructions)
		}
		if !reflect.DeepEqual(got.Tools, agent.Tools) {
			t.Errorf("tools mismatch: got %v, want %v", got.Tools, agent.Tools)
		}
		if got.Provider != agent.Provider {
			t.Errorf("provider mismatch: got %q, want %q", got.Provider, agent.Provider)
		}
		if got.Model != agent.Model {
			t.Errorf("model mismatch: got %q, want %q", got.Model, agent.Model)
		}
		if got.ModelRef != agent.Provider+"/"+agent.Model {
			t.Errorf("model_ref mismatch: got %q, want %q", got.ModelRef, agent.Provider+"/"+agent.Model)
		}
		if !reflect.DeepEqual(got.Options, agent.Options) {
			t.Errorf("options mismatch: got %v, want %v", got.Options, agent.Options)
		}
		if !reflect.DeepEqual(got.OutputSchema, agent.OutputSchema) {
			t.Errorf("output_schema mismatch: got %v, want %v", got.OutputSchema, agent.OutputSchema)
		}

		time.Sleep(1100 * time.Millisecond)

		agent.Name = "updated-agent"
		if err := s.UpdateAgent(agent); err != nil {
			t.Fatalf("update agent failed: %v", err)
		}

		got, err = s.GetAgent(agent.ID)
		if err != nil {
			t.Fatalf("get agent after update failed: %v", err)
		}
		if got.Name != "updated-agent" {
			t.Errorf("expected updated name, got %q", got.Name)
		}
		if got.UpdatedAt == originalUpdatedAt {
			t.Error("expected UpdatedAt to change after update")
		}

		agents, err := s.ListAgents()
		if err != nil {
			t.Fatalf("list agents failed: %v", err)
		}
		found := false
		for _, a := range agents {
			if a.ID == agent.ID {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected agent to be in list")
		}

		if err := s.DeleteAgent(agent.ID); err != nil {
			t.Fatalf("delete agent failed: %v", err)
		}
		if _, err := s.GetAgent(agent.ID); err == nil {
			t.Fatal("expected error getting deleted agent")
		}
		if err := s.DeleteAgent(agent.ID); err == nil {
			t.Fatal("expected error deleting agent twice")
		} else if !strings.Contains(err.Error(), "not found") {
			t.Fatalf("expected 'not found' error, got %v", err)
		}
	})

	t.Run("UpsertMessageInsertsAndListReturnsIt", func(t *testing.T) {
		s := factory(t)

		sess := &store.Session{Title: "upsert-msg", WorkDir: "/tmp"}
		if err := s.CreateSession(sess); err != nil {
			t.Fatalf("create session failed: %v", err)
		}

		msg := store.StoredMessage{
			ID:        store.NewID(store.PrefixMessage),
			SessionID: sess.ID,
			Idx:       0,
			Role:      "user",
			CreatedAt: time.Now().UTC(),
		}
		if err := s.UpsertMessage(context.Background(), msg); err != nil {
			t.Fatalf("upsert message failed: %v", err)
		}

		msgs, err := s.ListMessages(context.Background(), sess.ID)
		if err != nil {
			t.Fatalf("list messages failed: %v", err)
		}
		if len(msgs) != 1 {
			t.Fatalf("expected 1 message, got %d", len(msgs))
		}
		if msgs[0].ID != msg.ID {
			t.Errorf("expected message ID %q, got %q", msg.ID, msgs[0].ID)
		}
		if msgs[0].Role != msg.Role {
			t.Errorf("expected role %q, got %q", msg.Role, msgs[0].Role)
		}
		if len(msgs[0].Parts) != 0 {
			t.Errorf("expected empty parts, got %d", len(msgs[0].Parts))
		}
	})

	t.Run("UpsertMessageUpdatesPreservesCreatedAtAndIdx", func(t *testing.T) {
		s := factory(t)

		sess := &store.Session{Title: "upsert-update", WorkDir: "/tmp"}
		if err := s.CreateSession(sess); err != nil {
			t.Fatalf("create session failed: %v", err)
		}

		msgID := store.NewID(store.PrefixMessage)
		createdAt := time.Now().UTC().Add(-time.Hour)
		msg := store.StoredMessage{
			ID:        msgID,
			SessionID: sess.ID,
			Idx:       5,
			Role:      "user",
			CreatedAt: createdAt,
		}
		if err := s.UpsertMessage(context.Background(), msg); err != nil {
			t.Fatalf("first upsert failed: %v", err)
		}

		msg.Role = "assistant"
		if err := s.UpsertMessage(context.Background(), msg); err != nil {
			t.Fatalf("second upsert failed: %v", err)
		}

		msgs, err := s.ListMessages(context.Background(), sess.ID)
		if err != nil {
			t.Fatalf("list messages failed: %v", err)
		}
		if len(msgs) != 1 {
			t.Fatalf("expected 1 message, got %d", len(msgs))
		}
		if msgs[0].Role != "assistant" {
			t.Errorf("expected updated role 'assistant', got %q", msgs[0].Role)
		}
		if msgs[0].Idx != 5 {
			t.Errorf("expected idx 5, got %d", msgs[0].Idx)
		}
		if !msgs[0].CreatedAt.Truncate(time.Second).Equal(createdAt.Truncate(time.Second)) {
			t.Errorf("expected created_at preserved as %v, got %v", createdAt, msgs[0].CreatedAt)
		}
	})

	t.Run("UpsertPartInsertsAndListReturnsIt", func(t *testing.T) {
		s := factory(t)

		sess := &store.Session{Title: "upsert-part", WorkDir: "/tmp"}
		if err := s.CreateSession(sess); err != nil {
			t.Fatalf("create session failed: %v", err)
		}

		msg := store.StoredMessage{
			ID:        store.NewID(store.PrefixMessage),
			SessionID: sess.ID,
			Idx:       0,
			Role:      "user",
			CreatedAt: time.Now().UTC(),
		}
		if err := s.UpsertMessage(context.Background(), msg); err != nil {
			t.Fatalf("upsert message failed: %v", err)
		}

		part := store.StoredPart{
			ID:          store.NewID(store.PrefixPart),
			MessageID:   msg.ID,
			Sequence:    0,
			Kind:        "text",
			PayloadJSON: []byte(`{"text":"hello"}`),
		}
		if err := s.UpsertPart(context.Background(), part); err != nil {
			t.Fatalf("upsert part failed: %v", err)
		}

		msgs, err := s.ListMessages(context.Background(), sess.ID)
		if err != nil {
			t.Fatalf("list messages failed: %v", err)
		}
		if len(msgs) != 1 {
			t.Fatalf("expected 1 message, got %d", len(msgs))
		}
		if len(msgs[0].Parts) != 1 {
			t.Fatalf("expected 1 part, got %d", len(msgs[0].Parts))
		}
		if msgs[0].Parts[0].ID != part.ID {
			t.Errorf("expected part ID %q, got %q", part.ID, msgs[0].Parts[0].ID)
		}
		if string(msgs[0].Parts[0].PayloadJSON) != string(part.PayloadJSON) {
			t.Errorf("expected payload %q, got %q", string(part.PayloadJSON), string(msgs[0].Parts[0].PayloadJSON))
		}
	})

	t.Run("UpsertPartUpdatesPreservesSequence", func(t *testing.T) {
		s := factory(t)

		sess := &store.Session{Title: "upsert-part-update", WorkDir: "/tmp"}
		if err := s.CreateSession(sess); err != nil {
			t.Fatalf("create session failed: %v", err)
		}

		msg := store.StoredMessage{
			ID:        store.NewID(store.PrefixMessage),
			SessionID: sess.ID,
			Idx:       0,
			Role:      "user",
			CreatedAt: time.Now().UTC(),
		}
		if err := s.UpsertMessage(context.Background(), msg); err != nil {
			t.Fatalf("upsert message failed: %v", err)
		}

		partID := store.NewID(store.PrefixPart)
		part := store.StoredPart{
			ID:          partID,
			MessageID:   msg.ID,
			Sequence:    3,
			Kind:        "text",
			PayloadJSON: []byte(`{"text":"first"}`),
		}
		if err := s.UpsertPart(context.Background(), part); err != nil {
			t.Fatalf("first upsert part failed: %v", err)
		}

		part.Kind = "tool_call"
		part.PayloadJSON = []byte(`{"call_id":"c1"}`)
		if err := s.UpsertPart(context.Background(), part); err != nil {
			t.Fatalf("second upsert part failed: %v", err)
		}

		msgs, err := s.ListMessages(context.Background(), sess.ID)
		if err != nil {
			t.Fatalf("list messages failed: %v", err)
		}
		if len(msgs[0].Parts) != 1 {
			t.Fatalf("expected 1 part, got %d", len(msgs[0].Parts))
		}
		p := msgs[0].Parts[0]
		if p.Kind != "tool_call" {
			t.Errorf("expected updated kind 'tool_call', got %q", p.Kind)
		}
		if string(p.PayloadJSON) != `{"call_id":"c1"}` {
			t.Errorf("expected updated payload, got %q", string(p.PayloadJSON))
		}
		if p.Sequence != 3 {
			t.Errorf("expected sequence 3, got %d", p.Sequence)
		}
	})

	t.Run("ListMessagesOrdersMessagesAndParts", func(t *testing.T) {
		s := factory(t)

		sess := &store.Session{Title: "order-test", WorkDir: "/tmp"}
		if err := s.CreateSession(sess); err != nil {
			t.Fatalf("create session failed: %v", err)
		}

		msg1 := store.StoredMessage{
			ID:        store.NewID(store.PrefixMessage),
			SessionID: sess.ID,
			Idx:       1,
			Role:      "assistant",
			CreatedAt: time.Now().UTC(),
		}
		if err := s.UpsertMessage(context.Background(), msg1); err != nil {
			t.Fatalf("upsert msg1 failed: %v", err)
		}

		msg0 := store.StoredMessage{
			ID:        store.NewID(store.PrefixMessage),
			SessionID: sess.ID,
			Idx:       0,
			Role:      "user",
			CreatedAt: time.Now().UTC(),
		}
		if err := s.UpsertMessage(context.Background(), msg0); err != nil {
			t.Fatalf("upsert msg0 failed: %v", err)
		}

		part0 := store.StoredPart{
			ID:          store.NewID(store.PrefixPart),
			MessageID:   msg0.ID,
			Sequence:    1,
			Kind:        "text",
			PayloadJSON: []byte(`{"text":"second"}`),
		}
		part1 := store.StoredPart{
			ID:          store.NewID(store.PrefixPart),
			MessageID:   msg0.ID,
			Sequence:    0,
			Kind:        "text",
			PayloadJSON: []byte(`{"text":"first"}`),
		}
		if err := s.UpsertPart(context.Background(), part0); err != nil {
			t.Fatalf("upsert part0 failed: %v", err)
		}
		if err := s.UpsertPart(context.Background(), part1); err != nil {
			t.Fatalf("upsert part1 failed: %v", err)
		}

		msgs, err := s.ListMessages(context.Background(), sess.ID)
		if err != nil {
			t.Fatalf("list messages failed: %v", err)
		}
		if len(msgs) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(msgs))
		}
		if msgs[0].Idx != 0 {
			t.Errorf("expected first message idx 0, got %d", msgs[0].Idx)
		}
		if msgs[1].Idx != 1 {
			t.Errorf("expected second message idx 1, got %d", msgs[1].Idx)
		}
		if len(msgs[0].Parts) != 2 {
			t.Fatalf("expected 2 parts, got %d", len(msgs[0].Parts))
		}
		if msgs[0].Parts[0].Sequence != 0 {
			t.Errorf("expected first part sequence 0, got %d", msgs[0].Parts[0].Sequence)
		}
		if msgs[0].Parts[1].Sequence != 1 {
			t.Errorf("expected second part sequence 1, got %d", msgs[0].Parts[1].Sequence)
		}
	})

	t.Run("ListMessagesNonExistentSessionReturnsErrSessionNotFound", func(t *testing.T) {
		s := factory(t)

		_, err := s.ListMessages(context.Background(), "ses_nonexistent")
		if err == nil {
			t.Fatal("expected error for non-existent session")
		}
		if err != store.ErrSessionNotFound {
			t.Fatalf("expected ErrSessionNotFound, got %v", err)
		}
	})

	t.Run("ListMessagesEmptySessionReturnsEmptySlice", func(t *testing.T) {
		s := factory(t)

		sess := &store.Session{Title: "empty", WorkDir: "/tmp"}
		if err := s.CreateSession(sess); err != nil {
			t.Fatalf("create session failed: %v", err)
		}

		msgs, err := s.ListMessages(context.Background(), sess.ID)
		if err != nil {
			t.Fatalf("list messages failed: %v", err)
		}
		if msgs == nil {
			t.Fatal("expected non-nil slice, got nil")
		}
		if len(msgs) != 0 {
			t.Fatalf("expected empty slice, got %d", len(msgs))
		}
	})

	t.Run("DeleteSessionCascadesToMessagesAndParts", func(t *testing.T) {
		s := factory(t)

		sess := &store.Session{Title: "cascade-test", WorkDir: "/tmp"}
		if err := s.CreateSession(sess); err != nil {
			t.Fatalf("create session failed: %v", err)
		}

		msg := store.StoredMessage{
			ID:        store.NewID(store.PrefixMessage),
			SessionID: sess.ID,
			Idx:       0,
			Role:      "user",
			CreatedAt: time.Now().UTC(),
		}
		if err := s.UpsertMessage(context.Background(), msg); err != nil {
			t.Fatalf("upsert message failed: %v", err)
		}
		part := store.StoredPart{
			ID:          store.NewID(store.PrefixPart),
			MessageID:   msg.ID,
			Sequence:    0,
			Kind:        "text",
			PayloadJSON: []byte(`{"text":"hello"}`),
		}
		if err := s.UpsertPart(context.Background(), part); err != nil {
			t.Fatalf("upsert part failed: %v", err)
		}

		if err := s.DeleteSession(sess.ID); err != nil {
			t.Fatalf("delete session failed: %v", err)
		}

		_, err := s.ListMessages(context.Background(), sess.ID)
		if err != store.ErrSessionNotFound {
			t.Fatalf("expected ErrSessionNotFound after cascade delete, got %v", err)
		}
	})

	t.Run("SessionWorkDirRoundTrip", func(t *testing.T) {
		t.Run("with workdir", func(t *testing.T) {
			s := factory(t)
			sess := &store.Session{Title: "with-workdir", WorkDir: "/tmp"}
			if err := s.CreateSession(sess); err != nil {
				t.Fatalf("create failed: %v", err)
			}
			got, err := s.GetSession(sess.ID)
			if err != nil {
				t.Fatalf("get failed: %v", err)
			}
			if got.WorkDir != "/tmp" {
				t.Errorf("expected workdir '/tmp', got %q", got.WorkDir)
			}
		})

		t.Run("without workdir", func(t *testing.T) {
			s := factory(t)
			sess := &store.Session{Title: "without-workdir"}
			if err := s.CreateSession(sess); err != nil {
				t.Fatalf("create failed: %v", err)
			}
			got, err := s.GetSession(sess.ID)
			if err != nil {
				t.Fatalf("get failed: %v", err)
			}
			if got.WorkDir != "" {
				t.Errorf("expected empty workdir, got %q", got.WorkDir)
			}
		})
	})

	t.Run("ClientCRUDRoundTrip", func(t *testing.T) {
		s := factory(t)

		client, err := s.CreateClient("wingbase")
		if err != nil {
			t.Fatalf("create client failed: %v", err)
		}
		if client.ID == "" {
			t.Fatal("expected client ID to be set")
		}
		if !strings.HasPrefix(client.ID, "cli_") {
			t.Errorf("expected ID prefix 'cli_', got %q", client.ID)
		}
		if client.Name != "wingbase" {
			t.Errorf("expected name 'wingbase', got %q", client.Name)
		}
		if client.CreatedAt == "" {
			t.Error("expected created_at to be set")
		}

		got, err := s.GetClient(client.ID)
		if err != nil {
			t.Fatalf("get client failed: %v", err)
		}
		if got.Name != client.Name {
			t.Errorf("name mismatch: got %q, want %q", got.Name, client.Name)
		}

		clients, err := s.ListClients()
		if err != nil {
			t.Fatalf("list clients failed: %v", err)
		}
		found := false
		for _, c := range clients {
			if c.ID == client.ID {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected client to appear in list")
		}
	})

	t.Run("SessionCreatedWithValidClientIDRoundTripsIt", func(t *testing.T) {
		s := factory(t)

		client, err := s.CreateClient("test-client")
		if err != nil {
			t.Fatalf("create client failed: %v", err)
		}

		sess := &store.Session{Title: "scoped", ClientID: client.ID}
		if err := s.CreateSession(sess); err != nil {
			t.Fatalf("create session failed: %v", err)
		}

		got, err := s.GetSession(sess.ID)
		if err != nil {
			t.Fatalf("get session failed: %v", err)
		}
		if got.ClientID != client.ID {
			t.Errorf("expected client_id %q, got %q", client.ID, got.ClientID)
		}
	})

	t.Run("SessionCreatedWithNoClientIDHasNull", func(t *testing.T) {
		s := factory(t)

		sess := &store.Session{Title: "unscoped"}
		if err := s.CreateSession(sess); err != nil {
			t.Fatalf("create session failed: %v", err)
		}

		got, err := s.GetSession(sess.ID)
		if err != nil {
			t.Fatalf("get session failed: %v", err)
		}
		if got.ClientID != "" {
			t.Errorf("expected empty client_id, got %q", got.ClientID)
		}
	})

	t.Run("SessionCreatedWithNonexistentClientIDErrors", func(t *testing.T) {
		s := factory(t)

		sess := &store.Session{Title: "bad", ClientID: "cli_doesnotexist"}
		if err := s.CreateSession(sess); err == nil {
			t.Fatal("expected error creating session with non-existent client")
		} else if !strings.Contains(err.Error(), "client not found") {
			t.Fatalf("expected 'client not found' error, got %v", err)
		}
	})

	t.Run("ListSessionsByClientReturnsOnlyMatchingSessions", func(t *testing.T) {
		s := factory(t)

		clientA, err := s.CreateClient("client-a")
		if err != nil {
			t.Fatalf("create client a failed: %v", err)
		}
		clientB, err := s.CreateClient("client-b")
		if err != nil {
			t.Fatalf("create client b failed: %v", err)
		}

		sessA := &store.Session{Title: "a", ClientID: clientA.ID}
		sessB := &store.Session{Title: "b", ClientID: clientB.ID}
		sessUnscoped := &store.Session{Title: "unscoped"}
		for _, sess := range []*store.Session{sessA, sessB, sessUnscoped} {
			if err := s.CreateSession(sess); err != nil {
				t.Fatalf("create session failed: %v", err)
			}
		}

		listA, err := s.ListSessionsByClient(clientA.ID)
		if err != nil {
			t.Fatalf("list by client a failed: %v", err)
		}
		if len(listA) != 1 {
			t.Fatalf("expected 1 session for client A, got %d", len(listA))
		}
		if listA[0].ID != sessA.ID {
			t.Errorf("expected session A, got %q", listA[0].ID)
		}

		listB, err := s.ListSessionsByClient(clientB.ID)
		if err != nil {
			t.Fatalf("list by client b failed: %v", err)
		}
		if len(listB) != 1 {
			t.Fatalf("expected 1 session for client B, got %d", len(listB))
		}

		all, err := s.ListSessions()
		if err != nil {
			t.Fatalf("list all failed: %v", err)
		}
		if len(all) != 3 {
			t.Fatalf("expected 3 total sessions, got %d", len(all))
		}
	})

	t.Run("UpdateSessionDoesNotChangeClientID", func(t *testing.T) {
		s := factory(t)

		client, err := s.CreateClient("immutable-client")
		if err != nil {
			t.Fatalf("create client failed: %v", err)
		}

		sess := &store.Session{Title: "original", ClientID: client.ID}
		if err := s.CreateSession(sess); err != nil {
			t.Fatalf("create session failed: %v", err)
		}

		sess.Title = "updated"
		if err := s.UpdateSession(sess); err != nil {
			t.Fatalf("update session failed: %v", err)
		}

		got, err := s.GetSession(sess.ID)
		if err != nil {
			t.Fatalf("get failed: %v", err)
		}
		if got.ClientID != client.ID {
			t.Errorf("expected client_id unchanged as %q, got %q", client.ID, got.ClientID)
		}
	})

	t.Run("UpsertMessageRoundTripsMetadataJSONAndUpdatedAt", func(t *testing.T) {
		s := factory(t)

		sess := &store.Session{Title: "metadata-test", WorkDir: "/tmp"}
		if err := s.CreateSession(sess); err != nil {
			t.Fatalf("create session failed: %v", err)
		}

		createdAt := time.Now().UTC().Add(-time.Hour)
		updatedAt := time.Now().UTC().Add(-30 * time.Minute)
		msg := store.StoredMessage{
			ID:           store.NewID(store.PrefixMessage),
			SessionID:    sess.ID,
			Idx:          0,
			Role:         "user",
			MetadataJSON: []byte(`{"k":1}`),
			CreatedAt:    createdAt,
			UpdatedAt:    updatedAt,
		}
		if err := s.UpsertMessage(context.Background(), msg); err != nil {
			t.Fatalf("upsert message failed: %v", err)
		}

		msgs, err := s.ListMessages(context.Background(), sess.ID)
		if err != nil {
			t.Fatalf("list messages failed: %v", err)
		}
		if len(msgs) != 1 {
			t.Fatalf("expected 1 message, got %d", len(msgs))
		}
		if string(msgs[0].MetadataJSON) != `{"k":1}` {
			t.Errorf("expected metadata_json %q, got %q", `{"k":1}`, string(msgs[0].MetadataJSON))
		}
		if !msgs[0].UpdatedAt.Truncate(time.Second).Equal(updatedAt.Truncate(time.Second)) {
			t.Errorf("expected updated_at %v, got %v", updatedAt, msgs[0].UpdatedAt)
		}
	})

	t.Run("UpsertMessageWithNilMetadataJSONIsNull", func(t *testing.T) {
		s := factory(t)

		sess := &store.Session{Title: "nil-metadata", WorkDir: "/tmp"}
		if err := s.CreateSession(sess); err != nil {
			t.Fatalf("create session failed: %v", err)
		}

		msg := store.StoredMessage{
			ID:        store.NewID(store.PrefixMessage),
			SessionID: sess.ID,
			Idx:       0,
			Role:      "user",
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		if err := s.UpsertMessage(context.Background(), msg); err != nil {
			t.Fatalf("upsert message failed: %v", err)
		}

		msgs, err := s.ListMessages(context.Background(), sess.ID)
		if err != nil {
			t.Fatalf("list messages failed: %v", err)
		}
		if len(msgs) != 1 {
			t.Fatalf("expected 1 message, got %d", len(msgs))
		}
		if msgs[0].MetadataJSON != nil {
			t.Fatalf("expected nil metadata_json from list, got %q", string(msgs[0].MetadataJSON))
		}
	})

	t.Run("UpsertMessageSecondCallUpdatesMetadataJSONAndUpdatedAt", func(t *testing.T) {
		s := factory(t)

		sess := &store.Session{Title: "update-metadata", WorkDir: "/tmp"}
		if err := s.CreateSession(sess); err != nil {
			t.Fatalf("create session failed: %v", err)
		}

		createdAt := time.Now().UTC().Add(-time.Hour)
		msg := store.StoredMessage{
			ID:           store.NewID(store.PrefixMessage),
			SessionID:    sess.ID,
			Idx:          7,
			Role:         "user",
			MetadataJSON: []byte(`{"v":1}`),
			CreatedAt:    createdAt,
			UpdatedAt:    createdAt,
		}
		if err := s.UpsertMessage(context.Background(), msg); err != nil {
			t.Fatalf("first upsert failed: %v", err)
		}

		msg.Role = "assistant"
		msg.MetadataJSON = []byte(`{"v":2}`)
		msg.UpdatedAt = time.Now().UTC()
		if err := s.UpsertMessage(context.Background(), msg); err != nil {
			t.Fatalf("second upsert failed: %v", err)
		}

		msgs, err := s.ListMessages(context.Background(), sess.ID)
		if err != nil {
			t.Fatalf("list messages failed: %v", err)
		}
		if len(msgs) != 1 {
			t.Fatalf("expected 1 message, got %d", len(msgs))
		}
		if msgs[0].Role != "assistant" {
			t.Errorf("expected updated role, got %q", msgs[0].Role)
		}
		if string(msgs[0].MetadataJSON) != `{"v":2}` {
			t.Errorf("expected updated metadata_json, got %q", string(msgs[0].MetadataJSON))
		}
		if msgs[0].Idx != 7 {
			t.Errorf("expected idx 7, got %d", msgs[0].Idx)
		}
		if !msgs[0].CreatedAt.Truncate(time.Second).Equal(createdAt.Truncate(time.Second)) {
			t.Errorf("expected created_at preserved as %v, got %v", createdAt, msgs[0].CreatedAt)
		}
	})

	t.Run("UpsertPartRoundTripsCreatedAtAndUpdatedAt", func(t *testing.T) {
		s := factory(t)

		sess := &store.Session{Title: "part-timestamps", WorkDir: "/tmp"}
		if err := s.CreateSession(sess); err != nil {
			t.Fatalf("create session failed: %v", err)
		}

		msg := store.StoredMessage{
			ID:        store.NewID(store.PrefixMessage),
			SessionID: sess.ID,
			Idx:       0,
			Role:      "user",
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		if err := s.UpsertMessage(context.Background(), msg); err != nil {
			t.Fatalf("upsert message failed: %v", err)
		}

		createdAt := time.Now().UTC().Add(-time.Hour)
		updatedAt := time.Now().UTC().Add(-30 * time.Minute)
		part := store.StoredPart{
			ID:          store.NewID(store.PrefixPart),
			MessageID:   msg.ID,
			Sequence:    0,
			Kind:        "text",
			PayloadJSON: []byte(`{"text":"hello"}`),
			CreatedAt:   createdAt,
			UpdatedAt:   updatedAt,
		}
		if err := s.UpsertPart(context.Background(), part); err != nil {
			t.Fatalf("upsert part failed: %v", err)
		}

		msgs, err := s.ListMessages(context.Background(), sess.ID)
		if err != nil {
			t.Fatalf("list messages failed: %v", err)
		}
		if len(msgs) != 1 || len(msgs[0].Parts) != 1 {
			t.Fatalf("expected 1 message with 1 part, got %d msgs, %d parts", len(msgs), len(msgs[0].Parts))
		}
		p := msgs[0].Parts[0]
		if !p.CreatedAt.Truncate(time.Second).Equal(createdAt.Truncate(time.Second)) {
			t.Errorf("expected created_at %v, got %v", createdAt, p.CreatedAt)
		}
		if !p.UpdatedAt.Truncate(time.Second).Equal(updatedAt.Truncate(time.Second)) {
			t.Errorf("expected updated_at %v, got %v", updatedAt, p.UpdatedAt)
		}
	})

	t.Run("UpsertPartSecondCallUpdatesUpdatedAt", func(t *testing.T) {
		s := factory(t)

		sess := &store.Session{Title: "part-update-timestamps", WorkDir: "/tmp"}
		if err := s.CreateSession(sess); err != nil {
			t.Fatalf("create session failed: %v", err)
		}

		msg := store.StoredMessage{
			ID:        store.NewID(store.PrefixMessage),
			SessionID: sess.ID,
			Idx:       0,
			Role:      "user",
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		if err := s.UpsertMessage(context.Background(), msg); err != nil {
			t.Fatalf("upsert message failed: %v", err)
		}

		createdAt := time.Now().UTC().Add(-time.Hour)
		part := store.StoredPart{
			ID:          store.NewID(store.PrefixPart),
			MessageID:   msg.ID,
			Sequence:    5,
			Kind:        "text",
			PayloadJSON: []byte(`{"text":"first"}`),
			CreatedAt:   createdAt,
			UpdatedAt:   createdAt,
		}
		if err := s.UpsertPart(context.Background(), part); err != nil {
			t.Fatalf("first upsert part failed: %v", err)
		}

		part.Kind = "tool_call"
		part.PayloadJSON = []byte(`{"call_id":"c1"}`)
		part.UpdatedAt = time.Now().UTC()
		if err := s.UpsertPart(context.Background(), part); err != nil {
			t.Fatalf("second upsert part failed: %v", err)
		}

		msgs, err := s.ListMessages(context.Background(), sess.ID)
		if err != nil {
			t.Fatalf("list messages failed: %v", err)
		}
		p := msgs[0].Parts[0]
		if p.Sequence != 5 {
			t.Errorf("expected sequence 5, got %d", p.Sequence)
		}
		if !p.CreatedAt.Truncate(time.Second).Equal(createdAt.Truncate(time.Second)) {
			t.Errorf("expected created_at preserved as %v, got %v", createdAt, p.CreatedAt)
		}
		if !p.UpdatedAt.After(createdAt) {
			t.Errorf("expected updated_at to be after created_at, got %v", p.UpdatedAt)
		}
	})

	t.Run("ClientCreatedAtRoundTripsAsText", func(t *testing.T) {
		s := factory(t)

		client, err := s.CreateClient("text-client")
		if err != nil {
			t.Fatalf("create client failed: %v", err)
		}
		if client.CreatedAt == "" {
			t.Fatal("expected created_at to be set")
		}
		if _, err := time.Parse(time.RFC3339, client.CreatedAt); err != nil {
			t.Errorf("created_at is not valid RFC3339: %v", err)
		}

		got, err := s.GetClient(client.ID)
		if err != nil {
			t.Fatalf("get client failed: %v", err)
		}
		if got.CreatedAt != client.CreatedAt {
			t.Errorf("created_at mismatch: got %q, want %q", got.CreatedAt, client.CreatedAt)
		}
	})
}
