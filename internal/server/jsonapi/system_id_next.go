package jsonapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/cloud-fitter/cloud-fitter/internal/systemidseq"
	"github.com/golang/glog"
)

type nextSystemIDBody struct {
	Kind string `json:"kind"`
}

// NextSystemID POST /apis/system-id/next body: {"kind":"YH"|"D"} -> {"systemId":"YH-000001"}
func NextSystemID(w http.ResponseWriter, r *http.Request, store *systemidseq.Store) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if store == nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "system id store unavailable"})
		return
	}
	b, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	var body nextSystemIDBody
	if err := json.Unmarshal(b, &body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	kind := strings.TrimSpace(body.Kind)
	id, err := store.Next(kind)
	if err != nil {
		glog.Warningf("system-id next kind=%q err=%v", kind, err)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	glog.Infof("system-id allocated kind=%q -> %q", kind, id)
	_ = json.NewEncoder(w).Encode(map[string]string{"systemId": id})
}
