package store

import "testing"

func TestSQLiteAuthSessionsCascadeWithClient(t *testing.T) {
	data := newTestSQLiteStore(t)
	client, err := data.CreateClient("Cascade Client")
	if err != nil {
		t.Fatal(err)
	}
	if err := data.CreateAuthSession(&AuthSession{ClientID: client.ID, TokenHash: "cascade-hash"}); err != nil {
		t.Fatal(err)
	}
	if _, err := data.db.Exec(`DELETE FROM clients WHERE id = ?`, client.ID); err != nil {
		t.Fatal(err)
	}
	if session, err := data.AuthenticateAuthSession("cascade-hash"); err != nil || session != nil {
		t.Fatalf("authenticate after client deletion = %#v, %v", session, err)
	}
	if sessions, err := data.ListAuthSessions(client.ID); err != nil || len(sessions) != 0 {
		t.Fatalf("sessions after client deletion = %#v, %v", sessions, err)
	}
}
