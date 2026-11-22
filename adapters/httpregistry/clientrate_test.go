package httpregistry_test

import (
	"net/http"
	"testing"

	"github.com/nerolabs/silt/adapters/httpregistry"
	"github.com/nerolabs/silt/ports"
)

// tooManyOf issues n immediate lookups from one client and counts the 429s.
func tooManyOf(t *testing.T, addr string, n int) int {
	t.Helper()
	refused := 0
	for i := 0; i < n; i++ {
		resp, err := http.Get("http://" + addr + "/lookup?root=" + ports.HashBytes([]byte{byte(i)}).String())
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			refused++
		}
	}
	return refused
}

// The per-client budget is the default unless the operator raises it: 60 immediate
// requests overrun the default burst of 40, and fit a raised budget of 50/s (burst 100).
func TestClientRateSetsThePerClientBudget(t *testing.T) {
	reg := &asyncReg{committed: map[ports.Hash]ports.Entry{}, failed: map[ports.Hash]error{}}
	addr, shutdown, err := httpregistry.Serve("127.0.0.1:0", reg)
	if err != nil {
		t.Fatal(err)
	}
	defer shutdown()
	if n := tooManyOf(t, addr, 60); n == 0 {
		t.Fatal("60 immediate requests at the default budget drew no 429")
	}

	raised, shutdown2, err := httpregistry.Serve("127.0.0.1:0", reg, httpregistry.WithClientRate(50))
	if err != nil {
		t.Fatal(err)
	}
	defer shutdown2()
	if n := tooManyOf(t, raised, 60); n != 0 {
		t.Fatalf("60 immediate requests under a raised budget of 50/s drew %d 429s", n)
	}
}
