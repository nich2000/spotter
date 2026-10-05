package platform

import (
	"net/http"
	"spotter/internal/core"
	"time"
)

func (a *API) calendarProposal(w http.ResponseWriter, r *http.Request) {
	var p core.Object
	if err := body(w, r, &p); err != nil {
		problem(w, err)
		return
	}
	s, err := a.Store.Read(r.Context())
	if err != nil {
		problem(w, err)
		return
	}
	blocks, err := core.ProposeBlocks(s, p, time.Now())
	if err != nil {
		problem(w, err)
		return
	}
	send(w, 200, core.Object{"blocks": blocks, "workspaceRevision": s.Revision, "payload": p})
}
