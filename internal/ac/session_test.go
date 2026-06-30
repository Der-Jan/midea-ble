package ac_test

import (
	"context"
	"testing"
	"time"

	"github.com/sorinyang/midea-ble-go/internal/ac"
	"github.com/sorinyang/midea-ble-go/internal/ac/sessiontest"
)

func newConnected(t *testing.T) *ac.Session {
	t.Helper()
	sess, err := ac.NewSession(sessiontest.Dial(), sessiontest.Adv, sessiontest.OpenID, sessiontest.Opts())
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Handshake(context.Background()); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	return sess
}

func TestHandshake(t *testing.T) {
	sess := newConnected(t)
	defer sess.Close()
	if !sess.Connected() {
		t.Fatal("应已连接")
	}
}

func TestControlReadModifyWrite(t *testing.T) {
	sess := newConnected(t)
	defer sess.Close()
	st, err := sess.Control(context.Background(), func(a *ac.ACState) {
		a.Mode = ac.ModeCool
		a.TempSet = 26
	})
	if err != nil {
		t.Fatal(err)
	}
	if st.TempSet != 26 || st.ModeName != "cool" {
		t.Fatalf("期望 26/cool，得 %v/%s", st.TempSet, st.ModeName)
	}
}

func TestPubSub(t *testing.T) {
	sess := newConnected(t)
	defer sess.Close()
	ch, cancel := sess.Subscribe()
	defer cancel()
	if _, err := sess.Query(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case st := <-ch:
		if st == nil {
			t.Fatal("订阅应收到状态")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("订阅超时")
	}
}
