package platform

import (
	"github.com/pashagolub/pgxmock/v4"
	"net/http"
	"spotter/internal/core"
	"spotter/internal/model"
	"testing"
	"time"
)

func TestCalendarProposalHandler(t *testing.T) {
	s, m := mockStore(t)
	a := API{Store: s}
	state := core.NewState()
	state.Settings["zone"] = "UTC"
	state.Tasks["t"] = core.Object{"id": "t", "estimateMinutes": 100, "lifecycleState": "not_started"}
	for _, tc := range []struct {
		body string
		code int
	}{{`{"taskId":"t","startAt":"2026-10-05T11:00:00Z","zone":"UTC","minutes":50}`, 200}, {`{"taskId":"missing"}`, 404}, {`{`, 400}} {
		if tc.code != 400 {
			m.ExpectQuery("SELECT state").WillReturnRows(pgxmock.NewRows([]string{"state"}).AddRow(raw(state)))
		}
		w := call(http.HandlerFunc(a.calendarProposal), "POST", "/api/v2/calendar/proposal", []byte(tc.body), nil)
		if w.Code != tc.code {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
func TestCalendarSourceValidation(t *testing.T) {
	b := Batch{SchemaVersion: 1, BatchID: "c", Source: "calendar", Sequence: 1, ObservedAt: time.Now(), OK: true}
	if ValidateBatch(b) == nil {
		t.Fatal("missing coverage")
	}
	b.Data.CalendarFrom = time.Now()
	b.Data.CalendarTo = time.Now().Add(time.Hour)
	b.Data.Calendar = []model.CalendarEvent{{Start: time.Now(), End: time.Now().Add(time.Minute), Availability: "busy"}}
	if err := ValidateBatch(b); err != nil {
		t.Fatal(err)
	}
	b.Data.Calendar[0].End = b.Data.Calendar[0].Start
	if ValidateBatch(b) == nil {
		t.Fatal("invalid interval")
	}
}
